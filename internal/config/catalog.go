package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultFallbackURL = "https://models.dev/api.json"
	CatalogTTL         = 12 * time.Hour
)

// Catalog maps provider names to lists of model IDs.
type Catalog struct {
	Providers map[string][]string `json:"providers"`
}

// LoadOrRefreshCatalog loads the catalog from cache if valid (exists, < 12h old, matching sha256 checksum).
// Otherwise, it fetches from fallbackURL (defaulting to DefaultFallbackURL if empty) and atomically updates the cache.
func LoadOrRefreshCatalog(cacheDir string, fallbackURL string) (*Catalog, error) {
	if fallbackURL == "" {
		fallbackURL = DefaultFallbackURL
	}
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("user home dir: %w", err)
		}
		cacheDir = filepath.Join(home, ".tinyroute", "cache")
	}

	cacheFile := filepath.Join(cacheDir, "api.json")
	checksumFile := filepath.Join(cacheDir, "api.json.sha256")

	// Try reading valid cache first
	if cat, err := readValidCache(cacheFile, checksumFile); err == nil && cat != nil {
		return cat, nil
	}

	// Fetch fresh catalog
	cat, rawData, err := fetchCatalog(fallbackURL)
	if err != nil {
		// If fetch fails, try reading cached file regardless of TTL/checksum as fallback
		if catFallback, errFallback := readCacheFileOnly(cacheFile); errFallback == nil && catFallback != nil {
			return catFallback, nil
		}
		return nil, fmt.Errorf("fetch catalog: %w", err)
	}

	// Save to cache atomically
	if err := saveCacheAtomic(cacheDir, cacheFile, checksumFile, rawData); err != nil {
		// Non-fatal logging or ignore write error if catalog parsed fine
		fmt.Fprintf(os.Stderr, "warning: failed to save catalog cache: %v\n", err)
	}

	return cat, nil
}

func readValidCache(cacheFile, checksumFile string) (*Catalog, error) {
	info, err := os.Stat(cacheFile)
	if err != nil {
		return nil, err
	}
	if time.Since(info.ModTime()) > CatalogTTL {
		return nil, fmt.Errorf("cache expired")
	}

	rawData, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}

	checksumData, err := os.ReadFile(checksumFile)
	if err != nil {
		return nil, err
	}

	actualHash := sha256.Sum256(rawData)
	expectedHashStr := strings.TrimSpace(string(checksumData))
	if hex.EncodeToString(actualHash[:]) != expectedHashStr {
		return nil, fmt.Errorf("checksum mismatch")
	}

	return ParseCatalog(rawData)
}

func readCacheFileOnly(cacheFile string) (*Catalog, error) {
	rawData, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}
	return ParseCatalog(rawData)
}

func fetchCatalog(url string) (*Catalog, []byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	cat, err := ParseCatalog(body)
	if err != nil {
		return nil, nil, err
	}

	return cat, body, nil
}

func saveCacheAtomic(cacheDir, cacheFile, checksumFile string, rawData []byte) error {
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return err
	}

	hash := sha256.Sum256(rawData)
	hashStr := hex.EncodeToString(hash[:])

	tmpData := cacheFile + ".tmp"
	tmpSum := checksumFile + ".tmp"

	if err := os.WriteFile(tmpData, rawData, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(tmpSum, []byte(hashStr+"\n"), 0600); err != nil {
		os.Remove(tmpData)
		return err
	}

	if err := os.Rename(tmpData, cacheFile); err != nil {
		os.Remove(tmpData)
		os.Remove(tmpSum)
		return err
	}

	if err := os.Rename(tmpSum, checksumFile); err != nil {
		os.Remove(tmpSum)
		return err
	}

	return nil
}

// ParseCatalog flexibly parses model catalog JSON payloads.
func ParseCatalog(data []byte) (*Catalog, error) {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal catalog json: %w", err)
	}

	result := make(map[string][]string)

	switch v := raw.(type) {
	case map[string]interface{}:
		// Format A: {"providers": {"openai": ["gpt-4o", ...], ...}} or {"openai": {"models": ...}}
		if provsRaw, ok := v["providers"]; ok {
			if provsMap, ok := provsRaw.(map[string]interface{}); ok {
				v = provsMap
			}
		}

		for provName, provData := range v {
			if provName == "providers" {
				continue
			}
			models := extractModelsFromValue(provData)
			if len(models) > 0 {
				result[strings.ToLower(provName)] = models
			}
		}
	case []interface{}:
		// Format B: [{"id": "gpt-4o", "provider": "openai"}, ...]
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				prov := ""
				if p, ok := m["provider"].(string); ok {
					prov = p
				} else if p, ok := m["owned_by"].(string); ok {
					prov = p
				}
				id := ""
				if i, ok := m["id"].(string); ok {
					id = i
				} else if i, ok := m["name"].(string); ok {
					id = i
				}
				if prov != "" && id != "" {
					provKey := strings.ToLower(prov)
					result[provKey] = append(result[provKey], id)
				}
			}
		}
	}

	for prov, models := range result {
		result[prov] = uniqueSortedStrings(models)
	}

	return &Catalog{Providers: result}, nil
}

func extractModelsFromValue(val interface{}) []string {
	var models []string
	switch v := val.(type) {
	case []interface{}:
		for _, item := range v {
			switch elem := item.(type) {
			case string:
				models = append(models, elem)
			case map[string]interface{}:
				if id, ok := elem["id"].(string); ok {
					models = append(models, id)
				} else if name, ok := elem["name"].(string); ok {
					models = append(models, name)
				}
			}
		}
	case map[string]interface{}:
		if modelsVal, ok := v["models"]; ok {
			return extractModelsFromValue(modelsVal)
		}
		for key, item := range v {
			switch elem := item.(type) {
			case string:
				models = append(models, elem)
			case map[string]interface{}:
				if id, ok := elem["id"].(string); ok {
					models = append(models, id)
				} else {
					models = append(models, key)
				}
			default:
				models = append(models, key)
			}
		}
	}
	return models
}

func uniqueSortedStrings(slice []string) []string {
	seen := make(map[string]bool)
	var list []string
	for _, s := range slice {
		if s != "" && !seen[s] {
			seen[s] = true
			list = append(list, s)
		}
	}
	sort.Strings(list)
	return list
}

// FetchProviderModels queries a provider's direct models endpoint (e.g. /v1/models or /models).
func FetchProviderModels(baseURL string, apiKey string, dialect string) ([]string, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	// Pick candidate model-list endpoints. The Gemini (Generative Language) API
	// exposes its catalog at /v1beta/models, not the OpenAI-style /v1/models.
	var endpoints []string
	switch {
	case strings.Contains(baseURL, "cline.bot"):
		endpoints = []string{
			"https://api.cline.bot/api/v1/ai/cline/recommended-models",
			baseURL + "/models",
			baseURL + "/ai/cline/recommended-models",
		}
	case strings.HasSuffix(baseURL, "/v1"):
		endpoints = []string{
			baseURL + "/models",
			strings.TrimSuffix(baseURL, "/v1") + "/models",
		}
	case dialect == "gemini":
		endpoints = []string{
			baseURL + "/v1beta/models",
			baseURL + "/v1/models",
		}
	default:
		endpoints = []string{
			baseURL + "/v1/models",
			baseURL + "/models",
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}

	var lastErr error
	for _, url := range endpoints {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			continue
		}

		if apiKey != "" {
			if dialect == "anthropic" {
				req.Header.Set("x-api-key", apiKey)
				req.Header.Set("anthropic-version", "2023-06-01")
			} else {
				authVal := apiKey
				if strings.Contains(baseURL, "cline.bot") && !strings.HasPrefix(authVal, "workos:") {
					authVal = "workos:" + authVal
				}
				req.Header.Set("Authorization", "Bearer "+authVal)
			}
		}
		if strings.Contains(baseURL, "cline.bot") {
			req.Header.Set("HTTP-Referer", "https://cline.bot")
			req.Header.Set("X-Title", "Cline")
		}
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, string(bodyBytes))
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}

		models := parseModelsJSON(body)
		if len(models) > 0 {
			return models, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("could not parse models from provider response")
}

// supportsGenerateContent reports whether a Gemini model advertises the
// generateContent method (i.e. it is usable for chat, not just embeddings).
// An absent methods list is treated as supported to avoid over-filtering.
func supportsGenerateContent(methods []string) bool {
	if len(methods) == 0 {
		return true
	}
	for _, m := range methods {
		if m == "generateContent" {
			return true
		}
	}
	return false
}

func parseModelsJSON(body []byte) []string {
	// Gemini /v1beta/models returns {"models":[{"name":"models/<id>",
	// "supportedGenerationMethods":[...]}]}. Strip the "models/" prefix and
	// keep only chat-capable models (those supporting generateContent).
	var geminiResp struct {
		Models []struct {
			Name                       string   `json:"name"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &geminiResp); err == nil && len(geminiResp.Models) > 0 {
		var models []string
		for _, m := range geminiResp.Models {
			if !supportsGenerateContent(m.SupportedGenerationMethods) {
				continue
			}
			if id := strings.TrimPrefix(m.Name, "models/"); id != "" {
				models = append(models, id)
			}
		}
		if len(models) > 0 {
			return uniqueSortedStrings(models)
		}
	}

	var clineResp struct {
		Recommended []struct {
			ID string `json:"id"`
		} `json:"recommended"`
		Free []struct {
			ID string `json:"id"`
		} `json:"free"`
		ClinePass []struct {
			ID string `json:"id"`
		} `json:"clinePass"`
	}
	if err := json.Unmarshal(body, &clineResp); err == nil && (len(clineResp.Recommended) > 0 || len(clineResp.Free) > 0 || len(clineResp.ClinePass) > 0) {
		var models []string
		for _, m := range clineResp.Recommended {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
		for _, m := range clineResp.Free {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
		for _, m := range clineResp.ClinePass {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
		if len(models) > 0 {
			return uniqueSortedStrings(models)
		}
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && len(payload.Data) > 0 {
		var models []string
		for _, m := range payload.Data {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
		if len(models) > 0 {
			return uniqueSortedStrings(models)
		}
	}

	var arr []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &arr); err == nil && len(arr) > 0 {
		var models []string
		for _, m := range arr {
			if m.ID != "" {
				models = append(models, m.ID)
			} else if m.Name != "" {
				models = append(models, m.Name)
			}
		}
		if len(models) > 0 {
			return uniqueSortedStrings(models)
		}
	}

	cat, err := ParseCatalog(body)
	if err == nil {
		for _, models := range cat.Providers {
			if len(models) > 0 {
				return models
			}
		}
	}

	return nil
}
