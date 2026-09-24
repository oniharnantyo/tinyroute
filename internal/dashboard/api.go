package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/auth"
	"github.com/oniharnantyo/tinyroute/internal/clients"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	"github.com/oniharnantyo/tinyroute/internal/history"
	"github.com/oniharnantyo/tinyroute/internal/preset"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func parseJSONBody(r *http.Request, target any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func isProviderConfigured(name string, prov config.Provider, credPath string) bool {
	if prov.APIKey != "" {
		return true
	}
	store, err := credential.NewStore(credPath)
	if err != nil {
		return false
	}
	for _, rec := range store.ListMasked() {
		if strings.EqualFold(rec.Provider, name) {
			return true
		}
	}
	return false
}

func (h *DashboardHandler) handleAPIAuthStatus(w http.ResponseWriter, r *http.Request) {
	isDefault := false
	if h.deps.PasswordStore != nil {
		isDefault = h.deps.PasswordStore.IsDefaultPassword()
	}

	cookie, err := r.Cookie(SessionCookieName)
	authed := err == nil && cookie != nil && h.deps.SessionStore != nil && h.deps.SessionStore.ValidateSession(cookie.Value)

	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":       authed,
		"is_default_password": isDefault,
	})
}

func (h *DashboardHandler) handleAPIAuthLogin(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	if h.deps.LoginLimiter != nil {
		if allowed, retryAfter := h.deps.LoginLimiter.Allow(clientIP); !allowed {
			writeError(w, http.StatusTooManyRequests, fmt.Sprintf("Too many login attempts. Retry in %d seconds.", int(retryAfter.Seconds())))
			return
		}
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if h.deps.PasswordStore == nil || !h.deps.PasswordStore.VerifyPassword(req.Password) {
		writeError(w, http.StatusUnauthorized, "Invalid password")
		return
	}

	token := ""
	if h.deps.SessionStore != nil {
		token = h.deps.SessionStore.CreateSession(24 * time.Hour)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/dashboard",
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
	})
}

func (h *DashboardHandler) handleAPIAuthLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie != nil {
		if h.deps.SessionStore != nil {
			h.deps.SessionStore.RevokeSession(cookie.Value)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/dashboard",
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPIOverview(w http.ResponseWriter, r *http.Request) {
	windowKey := r.URL.Query().Get("window")
	spec, ok := windowDurations[windowKey]
	if !ok {
		windowKey = "24h"
		spec = windowDurations["24h"]
	}

	now := time.Now().UTC()
	from := now.Add(-spec.duration)
	to := now

	var totalReqs, successReqs, promptTokens, compTokens, avgLatency int64
	var successRate float64 = 100.0
	pStatsMap := make(map[string]history.ProviderStats)
	var topModels []history.ModelStats
	var buckets []history.Bucket

	if h.deps.HistoryAggregator != nil {
		ctx := r.Context()
		if ws, err := h.deps.HistoryAggregator.Stats(ctx, from, to); err == nil {
			totalReqs = ws.TotalRequests
			successReqs = ws.SuccessRequests
			promptTokens = ws.InputTokens
			compTokens = ws.OutputTokens
			avgLatency = ws.AvgLatencyMs
			if totalReqs > 0 {
				successRate = float64(successReqs) / float64(totalReqs) * 100.0
			}
		}
		if pStatsList, err := h.deps.HistoryAggregator.StatsByProvider(ctx, from, to); err == nil {
			for _, ps := range pStatsList {
				pStatsMap[ps.Provider] = ps
			}
		}
		if mStats, err := h.deps.HistoryAggregator.StatsByModel(ctx, from, to); err == nil {
			topModels = mStats
		}
		if b, err := h.deps.HistoryAggregator.RequestBuckets(ctx, from, to, spec.bucketMs); err == nil {
			buckets = b
		}
	}

	var topo *config.Topology
	if h.deps.TopologyWatcher != nil {
		topo = h.deps.TopologyWatcher.Get()
	}

	providerTrafficList := make([]map[string]any, 0)
	if topo != nil {
		for name, prov := range topo.Providers {
			ps := pStatsMap[name]
			inCooldown := false
			cooldownUntil := ""
			if h.deps.HealthStore != nil {
				if until := h.deps.HealthStore.CooldownEnd(name); !until.IsZero() {
					inCooldown = true
					cooldownUntil = until.Format("15:04:05")
				}
			}
			configured := isProviderConfigured(name, prov, h.deps.Service.CredentialsPath)

			var provSuccessRate float64 = 100.0
			if ps.TotalRequests > 0 {
				provSuccessRate = float64(ps.SuccessRequests) / float64(ps.TotalRequests) * 100.0
			}

			status := "healthy"
			if inCooldown || !configured {
				status = "cooldown"
			}

			providerTrafficList = append(providerTrafficList, map[string]any{
				"name":           name,
				"dialect":        prov.Dialect,
				"status":         status,
				"cooldown_until": cooldownUntil,
				"total_requests": ps.TotalRequests,
				"success_rate":   provSuccessRate,
			})
		}
	}
	sort.Slice(providerTrafficList, func(i, j int) bool {
		return providerTrafficList[i]["name"].(string) < providerTrafficList[j]["name"].(string)
	})

	recentSessions := make([]map[string]any, 0)
	if h.deps.HistoryQuerier != nil {
		filter := history.Filter{Limit: 10}
		if recs, _, err := h.deps.HistoryQuerier.List(r.Context(), filter); err == nil {
			for _, rec := range recs {
				atts := decodeAttempts(rec.Attempts)
				status := deriveStatusCode(rec.Outcome, atts)
				variant := "neutral"
				if status >= 200 && status < 300 {
					variant = "success"
				} else if status >= 400 && status < 500 {
					variant = "warning"
				} else {
					variant = "error"
				}

				recentSessions = append(recentSessions, map[string]any{
					"id":                rec.ID,
					"timestamp":         rec.Timestamp.UnixMilli(),
					"time_formatted":    rec.Timestamp.Format("15:04:05"),
					"provider":          rec.Provider,
					"endpoint":          rec.Endpoint,
					"model_requested":   rec.ModelReq,
					"model_served":      rec.ModelServed,
					"outcome":           rec.Outcome,
					"status_code":       status,
					"status_variant":    variant,
					"latency_ms":        rec.Latency.Milliseconds(),
					"input_tokens":      rec.InputTokens,
					"output_tokens":     rec.OutputTokens,
					"key_id":            rec.KeyID,
					"has_request_body":  rec.RequestBody != "",
					"has_response_body": rec.ResponseBody != "",
				})
			}
		}
	}

	bucketList := make([]map[string]any, 0, len(buckets))
	for _, b := range buckets {
		bucketList = append(bucketList, map[string]any{
			"timestamp":  b.Timestamp.UnixMilli(),
			"count":      b.Count,
			"time_label": formatBucketLabel(b.Timestamp, windowKey),
		})
	}

	modelStatsList := make([]map[string]any, 0, len(topModels))
	for _, m := range topModels {
		modelStatsList = append(modelStatsList, map[string]any{
			"model":         m.Model,
			"input_tokens":  m.InputTokens,
			"output_tokens": m.OutputTokens,
			"total_tokens":  m.TotalTokens,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"active_window":           windowKey,
		"total_requests":          totalReqs,
		"success_requests":        successReqs,
		"success_rate":            successRate,
		"total_prompt_tokens":     promptTokens,
		"total_completion_tokens": compTokens,
		"avg_latency_ms":          avgLatency,
		"top_models":              modelStatsList,
		"provider_traffic_list":   providerTrafficList,
		"buckets":                 bucketList,
		"recent_sessions":         recentSessions,
	})
}

// providerSection classifies a provider into the fixed card-group order:
// Free Tier first (any tiered preset, regardless of auth type), then OAuth,
// then API Key (spec: management-dashboard / providers list).
func providerSection(oauthCapable bool, tier string) string {
	if tier == "free" || tier == "freemium" {
		return "Free Tier"
	}
	if oauthCapable {
		return "OAuth"
	}
	return "API Key"
}

// mergedProviderAccounts merges the credential store's OAuth records with the
// topology's Accounts[] entries (the home of static API-key accounts), keyed
// by account name so the same account renders once.
func mergedProviderAccounts(provName string, prov config.Provider, credStore *credential.Store) []map[string]any {
	accMap := make(map[string]map[string]any)
	if credStore != nil {
		for _, rec := range credStore.ListMasked() {
			if !strings.EqualFold(rec.Provider, provName) {
				continue
			}
			acc := rec.Account
			if acc == "" {
				acc = "default"
			}
			maskedToken := rec.RefreshToken
			if maskedToken == "" {
				maskedToken = rec.AccessToken
			}
			expires := "Never"
			if !rec.ExpiresAt.IsZero() {
				expires = rec.ExpiresAt.Format("2006-01-02 15:04")
			}
			accMap[acc] = map[string]any{
				"name":         acc,
				"type":         "oauth",
				"status":       "active",
				"masked_token": maskedToken,
				"expires_at":   expires,
			}
		}
	}
	for _, acc := range prov.Accounts {
		if acc.Name == "" {
			continue
		}
		if _, ok := accMap[acc.Name]; ok {
			// A stored credential record means OAuth; the topology entry is
			// just router linkage for the same account.
			continue
		}
		credType := acc.Type
		if credType == "oauth_refresh" {
			credType = "oauth"
		} else if credType == "" {
			credType = "static"
		}
		tok := credential.MaskSecretToken(acc.APIKey)
		if tok == "" {
			tok = "configured"
		}
		accMap[acc.Name] = map[string]any{
			"name":         acc.Name,
			"type":         credType,
			"status":       "active",
			"masked_token": tok,
			"expires_at":   "Never",
		}
	}
	names := make([]string, 0, len(accMap))
	for k := range accMap {
		names = append(names, k)
	}
	sort.Strings(names)
	accounts := make([]map[string]any, 0, len(names))
	for _, n := range names {
		accounts = append(accounts, accMap[n])
	}
	return accounts
}

// providerStatus derives the detail-header status ladder: connected,
// cooldown, awaiting_credentials, or not_connected.
func (h *DashboardHandler) providerStatus(provName string, prov config.Provider, configured bool, connectionCount int) string {
	if !configured {
		return "not_connected"
	}
	if h.deps.HealthStore != nil {
		if until := h.deps.HealthStore.CooldownEnd(provName); !until.IsZero() {
			return "cooldown"
		}
	}
	if prov.APIKey != "" || connectionCount > 0 {
		return "connected"
	}
	return "awaiting_credentials"
}

func (h *DashboardHandler) handleAPIProviders(w http.ResponseWriter, r *http.Request) {
	var topo *config.Topology
	if h.deps.TopologyWatcher != nil {
		topo = h.deps.TopologyWatcher.Get()
	}
	credStore, _ := credential.NewStore(h.deps.Service.CredentialsPath)

	providersList := make([]map[string]any, 0)
	if topo != nil {
		for name, prov := range topo.Providers {
			var remaining time.Duration
			if h.deps.HealthStore != nil {
				if until := h.deps.HealthStore.CooldownEnd(name); !until.IsZero() {
					remaining = time.Until(until)
					if remaining < 0 {
						remaining = 0
					}
				}
			}
			configured := isProviderConfigured(name, prov, h.deps.Service.CredentialsPath)
			accounts := mergedProviderAccounts(name, prov, credStore)

			connectionCount := len(prov.Accounts)
			if connectionCount == 0 && prov.APIKey != "" {
				connectionCount = 1
			}

			// Check preset metadata for display name & logo
			displayName := titleCase(name)
			logoName := name
			tier := ""
			freeNote := ""
			oauthCapable := false

			if pre := preset.Get(name); pre != nil {
				if pre.DisplayName != "" {
					displayName = pre.DisplayName
				}
				if pre.Logo != "" {
					logoName = pre.Logo
				}
				tier = pre.Tier
				freeNote = pre.FreeNote
				oauthCapable = pre.OAuthCapable
			}

			providersList = append(providersList, map[string]any{
				"name":                   name,
				"display_name":           displayName,
				"logo":                   logoName,
				"dialect":                prov.Dialect,
				"base_url":               prov.BaseURL,
				"configured":             configured,
				"in_topology":            true,
				"models":                 prov.Models,
				"accounts":               accounts,
				"connection_count":       connectionCount,
				"status":                 h.providerStatus(name, prov, true, connectionCount),
				"in_cooldown":            remaining > 0,
				"cooldown_remaining_sec": int64(remaining.Seconds()),
				"oauth_capable":          oauthCapable,
				"tier":                   tier,
				"free_note":              freeNote,
				"section":                providerSection(oauthCapable, tier),
			})
		}
	}
	sort.Slice(providersList, func(i, j int) bool {
		return providersList[i]["name"].(string) < providersList[j]["name"].(string)
	})

	presetsList := make([]map[string]any, 0)
	for _, pre := range preset.All() {
		presetsList = append(presetsList, map[string]any{
			"name":             pre.Name,
			"display_name":     pre.DisplayName,
			"dialect":          pre.Dialect,
			"default_base_url": pre.BaseURL,
			"models":           pre.Models,
			"logo":             pre.Logo,
			"tier":             pre.Tier,
			"free_note":        pre.FreeNote,
			"oauth_capable":    pre.OAuthCapable,
			"description":      pre.Tier,
			"section":          providerSection(pre.OAuthCapable, pre.Tier),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"providers": providersList,
		"presets":   presetsList,
	})
}

// handleAPIProviderDetail serves the shared provider-detail view model as
// JSON for the SPA: status ladder, merged masked connections, and the full
// model catalog split into whitelisted and available groups.
func (h *DashboardHandler) handleAPIProviderDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		name = strings.TrimPrefix(r.URL.Path, "/dashboard/api/providers/")
		name = strings.Trim(name, "/")
	}

	data, found, errRedirect := h.buildProviderDetailData(name)
	if errRedirect != "" {
		writeError(w, http.StatusInternalServerError, errRedirect)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "Provider not found")
		return
	}

	connList := make([]map[string]any, 0, len(data.Connections))
	for _, conn := range data.Connections {
		connList = append(connList, map[string]any{
			"name":                 conn.Account,
			"type":                 conn.Type,
			"status":               "active",
			"masked_token":         conn.RefreshToken,
			"expires_at":           conn.ExpiresAt,
			"affected_combo_count": conn.AffectedComboCount,
		})
	}

	toModelList := func(models []CatalogModelItem) []map[string]any {
		out := make([]map[string]any, 0, len(models))
		for _, m := range models {
			out = append(out, map[string]any{
				"id":          m.ID,
				"name":        m.Name,
				"whitelisted": m.Whitelisted,
			})
		}
		return out
	}

	connectionCount := len(data.Connections)
	if connectionCount == 0 && data.APIKey != "" {
		connectionCount = 1
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":               data.Name,
		"display_name":       data.DisplayName,
		"logo":               data.Logo,
		"dialect":            data.Dialect,
		"base_url":           data.BaseURL,
		"configured":         data.Configured,
		"in_topology":        data.Configured,
		"oauth_capable":      data.OAuthCapable,
		"health_status":      data.HealthStatus,
		"status":             data.Status,
		"connection_count":   connectionCount,
		"connections":        connList,
		"models":             toModelList(data.Models),
		"whitelisted_models": toModelList(data.WhitelistedModels),
		"available_models":   toModelList(data.AvailableModels),
	})
}

func (h *DashboardHandler) handleAPIProviderAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Preset string `json:"preset"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	pre := preset.Get(req.Preset)
	if pre == nil {
		pre = preset.Get(req.Name)
	}
	if pre == nil {
		writeError(w, http.StatusBadRequest, "Preset not found")
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read config error: "+err.Error())
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Parse topology error: "+err.Error())
		return
	}

	provName := strings.ToLower(req.Name)
	if provName == "" {
		provName = strings.ToLower(pre.Name)
	}
	if rawTopo.Providers == nil {
		rawTopo.Providers = make(map[string]config.Provider)
	}

	apiKey := ""
	if pre.CredentialVar != "" {
		apiKey = "${" + pre.CredentialVar + "}"
	}

	rawTopo.Providers[provName] = config.Provider{
		Dialect: pre.Dialect,
		BaseURL: pre.BaseURL,
		APIKey:  apiKey,
		Models:  pre.Models,
	}

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		writeError(w, http.StatusInternalServerError, "Write topology error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPICustomProviderAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string   `json:"name"`
		Dialect string   `json:"dialect"`
		BaseURL string   `json:"base_url"`
		APIKey  string   `json:"api_key"`
		Models  []string `json:"models"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	name := strings.ToLower(strings.TrimSpace(req.Name))
	if name == "" || req.Dialect == "" || req.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "Name, dialect, and base_url are required")
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read config error: "+err.Error())
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Parse topology error: "+err.Error())
		return
	}

	if rawTopo.Providers == nil {
		rawTopo.Providers = make(map[string]config.Provider)
	}

	rawTopo.Providers[name] = config.Provider{
		Dialect: req.Dialect,
		BaseURL: req.BaseURL,
		APIKey:  req.APIKey,
		Models:  req.Models,
	}

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		writeError(w, http.StatusInternalServerError, "Write topology error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPIProviderDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	name := strings.ToLower(strings.TrimSpace(req.Name))
	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read config error: "+err.Error())
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Parse topology error: "+err.Error())
		return
	}

	delete(rawTopo.Providers, name)

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		writeError(w, http.StatusInternalServerError, "Write topology error: "+err.Error())
		return
	}

	if credStore, err := credential.NewStore(h.deps.Service.CredentialsPath); err == nil && credStore != nil {
		_ = credStore.DeleteProvider(name)
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPIProviderCredential(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Account  string `json:"account"`
		APIKey   string `json:"api_key"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	provName := strings.ToLower(strings.TrimSpace(req.Provider))
	account := strings.TrimSpace(req.Account)
	if account == "" {
		account = "default"
	}

	acc := config.Account{
		Name:   account,
		Type:   "static",
		APIKey: req.APIKey,
	}

	if err := h.saveAccountGesture(provName, acc, nil); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPIProviderCredentialDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Account  string `json:"account"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if req.Provider == "" || req.Account == "" {
		writeError(w, http.StatusBadRequest, "Provider and account are required")
		return
	}

	// 1. Delete from the credential store (slash keys; the bare provider key
	// is the legacy home of the default account).
	if credStore, err := credential.NewStore(h.deps.Service.CredentialsPath); err == nil && credStore != nil {
		_ = credStore.Delete(req.Provider + "/" + req.Account)
		if req.Account == "default" {
			_ = credStore.Delete(req.Provider)
		}
	}

	// 2. Remove the account from the topology and downgrade any combo member
	// pinned to it (spec: connection removal keeps combos consistent).
	modifiedCombos := make([]string, 0)
	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read config error: "+err.Error())
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Parse topology error: "+err.Error())
		return
	}

	provKey := req.Provider
	prov, ok := rawTopo.Providers[provKey]
	if !ok {
		provKey = strings.ToLower(req.Provider)
		prov, ok = rawTopo.Providers[provKey]
	}
	if ok {
		remaining := make([]config.Account, 0, len(prov.Accounts))
		for _, acc := range prov.Accounts {
			if acc.Name != req.Account {
				remaining = append(remaining, acc)
			}
		}
		prov.Accounts = remaining
		rawTopo.Providers[provKey] = prov
		rawTopo.Combos, modifiedCombos = config.DowngradeComboAccount(rawTopo.Combos, req.Provider, req.Account)
		if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
			writeError(w, http.StatusInternalServerError, "Write topology error: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"modified_combos": modifiedCombos,
	})
}

func (h *DashboardHandler) handleAPIProviderAccountRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider   string `json:"provider"`
		OldAccount string `json:"old_name"`
		NewAccount string `json:"new_name"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	provName := strings.TrimSpace(req.Provider)
	oldAccount := strings.TrimSpace(req.OldAccount)
	newAccount := strings.TrimSpace(req.NewAccount)
	if provName == "" || oldAccount == "" || newAccount == "" {
		writeError(w, http.StatusBadRequest, "Provider and account names are required")
		return
	}

	if err := credential.ValidateAccountName(newAccount); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid account name: "+err.Error())
		return
	}

	if oldAccount == newAccount {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Account name unchanged"})
		return
	}

	credStore, err := credential.NewStore(h.deps.Service.CredentialsPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Open credential store error: "+err.Error())
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read config error: "+err.Error())
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Parse topology error: "+err.Error())
		return
	}

	for _, acc := range getDashboardExistingAccounts(provName, credStore, config.Topology{Providers: rawTopo.Providers}) {
		if strings.EqualFold(acc, newAccount) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Account '%s' already exists for %s", newAccount, provName))
			return
		}
	}

	// 1. Re-key in the credential store (store-first).
	oldKey := provName + "/" + oldAccount
	if rec, ok := credStore.Get(oldKey); ok {
		rec.Account = newAccount
		if err := credStore.Save(rec); err != nil {
			writeError(w, http.StatusInternalServerError, "Save renamed credential failed: "+err.Error())
			return
		}
		_ = credStore.Delete(oldKey)
		if oldAccount == "default" {
			_ = credStore.Delete(provName)
		}
	}

	// 2. Rename in the topology and rewrite pinned combo members in the same
	// write (spec: rename re-keys credential and topology together).
	provKey := provName
	prov, ok := rawTopo.Providers[provKey]
	if !ok {
		provKey = strings.ToLower(provName)
		prov, ok = rawTopo.Providers[provKey]
	}
	if ok {
		renamed := false
		for i, acc := range prov.Accounts {
			if acc.Name == oldAccount {
				prov.Accounts[i].Name = newAccount
				renamed = true
				break
			}
		}
		if !renamed {
			prov = prov.UpsertAccount(config.Account{
				Name: newAccount,
				Type: "oauth_refresh",
			})
		}
		rawTopo.Providers[provKey] = prov
		rawTopo.Combos = config.RenameComboAccount(rawTopo.Combos, provName, oldAccount, newAccount)
		if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
			writeError(w, http.StatusInternalServerError, "Write topology error: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("Renamed account '%s' to '%s'", oldAccount, newAccount),
	})
}

func (h *DashboardHandler) handleAPIModelAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	provName := strings.ToLower(strings.TrimSpace(req.Provider))
	model := strings.TrimSpace(req.Model)

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read config error: "+err.Error())
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Parse topology error: "+err.Error())
		return
	}

	// Whitelisting a model on an available preset materializes it first
	// (spec: connection and whitelist actions lazily materialize presets).
	if _, err := ensureMaterialized(h.deps.Service.ConfigPath, &rawTopo, provName); err != nil {
		writeError(w, http.StatusInternalServerError, "Materialize error: "+err.Error())
		return
	}

	prov, ok := rawTopo.Providers[provName]
	if !ok {
		writeError(w, http.StatusNotFound, "Provider not found")
		return
	}

	for _, m := range prov.Models {
		if m == model {
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
			return
		}
	}

	prov.Models = append(prov.Models, model)
	rawTopo.Providers[provName] = prov

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		writeError(w, http.StatusInternalServerError, "Write topology error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPIModelRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	provName := strings.ToLower(strings.TrimSpace(req.Provider))
	model := strings.TrimSpace(req.Model)

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read config error: "+err.Error())
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Parse topology error: "+err.Error())
		return
	}

	prov, ok := rawTopo.Providers[provName]
	if !ok {
		writeError(w, http.StatusNotFound, "Provider not found")
		return
	}

	filtered := make([]string, 0, len(prov.Models))
	for _, m := range prov.Models {
		if m != model {
			filtered = append(filtered, m)
		}
	}
	prov.Models = filtered
	rawTopo.Providers[provName] = prov

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		writeError(w, http.StatusInternalServerError, "Write topology error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPIModelTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Dialect  string `json:"dialect"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if h.deps.RunProbe == nil {
		writeError(w, http.StatusServiceUnavailable, "Probe runner unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	statusCode, duration, err := h.deps.RunProbe(ctx, req.Provider, req.Dialect, req.Model, 15*time.Second)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status_code": statusCode,
		"duration_ms": duration.Milliseconds(),
		"error":       errMsg,
		"success":     err == nil && statusCode < 400,
	})
}

func (h *DashboardHandler) handleAPICombos(w http.ResponseWriter, r *http.Request) {
	var topo *config.Topology
	if h.deps.TopologyWatcher != nil {
		topo = h.deps.TopologyWatcher.Get()
	}
	combosList := make([]map[string]any, 0)
	availableModels := make([]map[string]string, 0)

	if topo != nil {
		for _, c := range topo.Combos {
			mode := c.Mode
			if mode == "" {
				mode = "ordered"
			}
			combosList = append(combosList, map[string]any{
				"name":         c.Name,
				"mode":         mode,
				"members":      c.Members,
				"capabilities": c.Capabilities,
				"enabled":      c.IsEnabled(),
			})
		}

		for provName, prov := range topo.Providers {
			for _, m := range prov.Models {
				availableModels = append(availableModels, map[string]string{
					"provider": provName,
					"model":    m,
				})
			}
		}
	}

	sort.Slice(combosList, func(i, j int) bool {
		return combosList[i]["name"].(string) < combosList[j]["name"].(string)
	})

	// Per-provider account names for the wizard's Connection dropdown — each
	// model lists once, accounts carry the pin (spec: member selection
	// conveys priority order).
	providerAccounts := make(map[string][]string)
	if topo != nil {
		seen := make(map[string]map[string]bool)
		addAccount := func(provider, account string) {
			if account == "" {
				return
			}
			if seen[provider] == nil {
				seen[provider] = make(map[string]bool)
			}
			if !seen[provider][account] {
				seen[provider][account] = true
				providerAccounts[provider] = append(providerAccounts[provider], account)
			}
		}
		for provName, prov := range topo.Providers {
			for _, acc := range prov.Accounts {
				addAccount(provName, acc.Name)
			}
		}
		if credStore, err := credential.NewStore(h.deps.Service.CredentialsPath); err == nil {
			for _, rec := range credStore.List() {
				acc := rec.Account
				if acc == "" {
					acc = "default"
				}
				addAccount(rec.Provider, acc)
			}
		}
		for prov := range providerAccounts {
			sort.Strings(providerAccounts[prov])
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"combos":            combosList,
		"available_models":  availableModels,
		"provider_accounts": providerAccounts,
	})
}

func (h *DashboardHandler) handleAPICombosWizard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string   `json:"name"`
		Mode         string   `json:"mode"`
		Capabilities []string `json:"capabilities"`
		Routes       []struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		} `json:"routes"`
		Members []string `json:"members"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Combo name is required")
		return
	}

	members := req.Members
	if len(members) == 0 && len(req.Routes) > 0 {
		for _, rt := range req.Routes {
			members = append(members, fmt.Sprintf("%s:%s", rt.Provider, rt.Model))
		}
	}

	if len(members) == 0 {
		writeError(w, http.StatusBadRequest, "Combo must have at least one member")
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read config error: "+err.Error())
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Parse topology error: "+err.Error())
		return
	}

	mode := req.Mode
	if mode == "" {
		mode = "ordered"
	}

	newCombo := config.Combo{
		Name:         name,
		Mode:         mode,
		Members:      members,
		Capabilities: req.Capabilities,
	}

	found := false
	for i, c := range rawTopo.Combos {
		if c.Name == name {
			newCombo.Disabled = c.Disabled
			rawTopo.Combos[i] = newCombo
			found = true
			break
		}
	}
	if !found {
		rawTopo.Combos = append(rawTopo.Combos, newCombo)
	}

	validTopo := config.Topology{
		Providers: rawTopo.Providers,
		Combos:    rawTopo.Combos,
	}
	if errs := config.ValidateTopology(validTopo, dialect.Names()); len(errs) > 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid topology: %v", errs[0]))
		return
	}

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		writeError(w, http.StatusInternalServerError, "Write topology error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPICombosToggle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read config error: "+err.Error())
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Parse topology error: "+err.Error())
		return
	}

	found := false
	for i, c := range rawTopo.Combos {
		if c.Name == req.Name {
			rawTopo.Combos[i].Disabled = !req.Enabled
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "Combo not found")
		return
	}

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		writeError(w, http.StatusInternalServerError, "Write topology error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPICombosDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Read config error: "+err.Error())
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Parse topology error: "+err.Error())
		return
	}

	filtered := make([]config.Combo, 0, len(rawTopo.Combos))
	for _, c := range rawTopo.Combos {
		if c.Name != req.Name {
			filtered = append(filtered, c)
		}
	}
	rawTopo.Combos = filtered

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		writeError(w, http.StatusInternalServerError, "Write topology error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPIHistory(w http.ResponseWriter, r *http.Request) {
	if h.deps.HistoryQuerier == nil {
		writeJSON(w, http.StatusOK, map[string]any{"records": []any{}, "total": 0})
		return
	}

	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 && l <= history.MaxListLimit {
			limit = l
		}
	}

	filter := history.Filter{
		Limit:    limit,
		Provider: r.URL.Query().Get("provider"),
		KeyID:    r.URL.Query().Get("key"),
		Session:  r.URL.Query().Get("search"),
	}

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", fromStr, time.Local); err == nil {
			filter.From = t
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", toStr, time.Local); err == nil {
			filter.To = t.Add(24*time.Hour - time.Nanosecond)
		}
	}

	records, _, err := h.deps.HistoryQuerier.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	recordList := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		atts := decodeAttempts(rec.Attempts)
		statusCode := deriveStatusCode(rec.Outcome, atts)
		statusVariant := "neutral"
		if statusCode >= 200 && statusCode < 300 {
			statusVariant = "success"
		} else if statusCode >= 400 && statusCode < 500 {
			statusVariant = "warning"
		} else {
			statusVariant = "error"
		}

		attemptList := make([]map[string]any, 0, len(atts))
		for _, a := range atts {
			attemptList = append(attemptList, map[string]any{
				"provider":   a.Provider,
				"model":      a.Model,
				"status":     a.Status,
				"elapsed_ms": a.Elapsed.Milliseconds(),
			})
		}

		totalToks := rec.InputTokens + rec.OutputTokens

		recordList = append(recordList, map[string]any{
			"id":                    rec.ID,
			"timestamp":             rec.Timestamp.UnixMilli(),
			"time_formatted":        rec.Timestamp.Format("2006-01-02 15:04:05"),
			"session_id":            rec.Session,
			"key_id":                rec.KeyID,
			"provider":              rec.Provider,
			"endpoint":              rec.Endpoint,
			"model_requested":       rec.ModelReq,
			"model_served":          rec.ModelServed,
			"outcome":               rec.Outcome,
			"status_code":           statusCode,
			"status_variant":        statusVariant,
			"latency_ms":            rec.Latency.Milliseconds(),
			"input_tokens":          rec.InputTokens,
			"output_tokens":         rec.OutputTokens,
			"cache_read_tokens":     rec.CacheReadTokens,
			"cache_creation_tokens": rec.CacheCreationTokens,
			"total_tokens":          totalToks,
			"has_request_body":      rec.RequestBody != "",
			"has_response_body":     rec.ResponseBody != "",
			"attempts":              attemptList,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"records": recordList,
		"total":   len(recordList),
	})
}

func (h *DashboardHandler) handleAPIHistoryDetail(w http.ResponseWriter, r *http.Request) {
	if h.deps.HistoryQuerier == nil {
		writeError(w, http.StatusNotFound, "History querier not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/dashboard/api/history/")
	}

	rec, found, err := h.deps.HistoryQuerier.Get(r.Context(), id)
	if err != nil || !found {
		writeError(w, http.StatusNotFound, "Record not found")
		return
	}

	atts := decodeAttempts(rec.Attempts)
	statusCode := deriveStatusCode(rec.Outcome, atts)
	statusVariant := "neutral"
	if statusCode >= 200 && statusCode < 300 {
		statusVariant = "success"
	} else if statusCode >= 400 && statusCode < 500 {
		statusVariant = "warning"
	} else {
		statusVariant = "error"
	}

	attemptList := make([]map[string]any, 0, len(atts))
	for _, a := range atts {
		attemptList = append(attemptList, map[string]any{
			"provider":   a.Provider,
			"model":      a.Model,
			"status":     a.Status,
			"elapsed_ms": a.Elapsed.Milliseconds(),
		})
	}

	formattedReq := formatBodyPane(rec.RequestBody)
	formattedTransReq := formatBodyPane(rec.TranslatedRequestBody)
	formattedRawResp := formatBodyPane(rec.RawResponseBody)
	formattedResp := formatBodyPane(rec.ResponseBody)

	writeJSON(w, http.StatusOK, map[string]any{
		"id":                       rec.ID,
		"timestamp":                rec.Timestamp.UnixMilli(),
		"time_formatted":           rec.Timestamp.Format("2006-01-02 15:04:05"),
		"session_id":               rec.Session,
		"key_id":                   rec.KeyID,
		"provider":                 rec.Provider,
		"endpoint":                 rec.Endpoint,
		"model_requested":          rec.ModelReq,
		"model_served":             rec.ModelServed,
		"outcome":                  rec.Outcome,
		"status_code":              statusCode,
		"status_variant":           statusVariant,
		"latency_ms":               rec.Latency.Milliseconds(),
		"input_tokens":             rec.InputTokens,
		"output_tokens":            rec.OutputTokens,
		"cache_read_tokens":        rec.CacheReadTokens,
		"cache_creation_tokens":    rec.CacheCreationTokens,
		"total_tokens":             rec.InputTokens + rec.OutputTokens,
		"has_request_body":         rec.RequestBody != "",
		"has_translated_request":   rec.TranslatedRequestBody != "",
		"has_raw_response":         rec.RawResponseBody != "",
		"has_response_body":        rec.ResponseBody != "",
		"request_body":             formattedReq.Content,
		"translated_request_body":  formattedTransReq.Content,
		"raw_response_body":        formattedRawResp.Content,
		"response_body":            formattedResp.Content,
		"request_body_truncated":   formattedReq.IsTruncated,
		"translated_req_truncated": formattedTransReq.IsTruncated,
		"raw_resp_truncated":       formattedRawResp.IsTruncated,
		"response_body_truncated":  formattedResp.IsTruncated,
		"attempts":                 attemptList,
	})
}

func (h *DashboardHandler) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	keysList := make([]map[string]any, 0)
	var lastUsedMap map[string]time.Time
	if h.deps.HistoryQuerier != nil {
		lastUsedMap, _ = h.deps.HistoryQuerier.LastUseByKey(r.Context())
	}

	if h.deps.KeyWatcher != nil {
		if ks := h.deps.KeyWatcher.Get(); ks != nil {
			for _, k := range ks.Keys() {
				// Revoked keys MUST NOT render (spec: API keys are managed
				// from the dashboard).
				if k.Disabled {
					continue
				}

				var rpm, rpd *int
				rateSpec := "Unlimited"
				if k.Rate != nil && k.Rate.Requests > 0 {
					val := k.Rate.Requests
					if k.Rate.Interval == "1d" || k.Rate.Interval == "24h" {
						rpd = &val
					} else {
						rpm = &val
					}
					rateSpec = fmt.Sprintf("%d / %s", k.Rate.Requests, k.Rate.Interval)
				}

				var lastUsedAt *int64
				lastUsedFormatted := "Never"
				if lastUsedMap != nil {
					if t, ok := lastUsedMap[k.ID]; ok && !t.IsZero() {
						ms := t.UnixMilli()
						lastUsedAt = &ms
						lastUsedFormatted = t.Format("2006-01-02 15:04")
					}
				}

				var expiresAt *int64
				expiresFormatted := "Never"
				expired := false
				if k.Expires != nil {
					ms := k.Expires.UnixMilli()
					expiresAt = &ms
					expiresFormatted = k.Expires.Format("2006-01-02 15:04")
					expired = k.Expires.Before(time.Now())
				}

				masked := k.Prefix + "••••••••••••••••"

				keysList = append(keysList, map[string]any{
					"id":                  k.ID,
					"name":                k.Name,
					"prefix":              k.Prefix,
					"masked":              masked,
					"secret":              k.Secret,
					"rate_spec":           rateSpec,
					"rate_limit_rpm":      rpm,
					"rate_limit_rpd":      rpd,
					"created_at":          k.Created.UnixMilli(),
					"expires":             expiresAt,
					"expires_formatted":   expiresFormatted,
					"last_used_at":        lastUsedAt,
					"last_used_formatted": lastUsedFormatted,
					"is_active":           !expired,
					"is_expired":          expired,
					"is_revoked":          false,
				})
			}
		}
	}

	sort.Slice(keysList, func(i, j int) bool {
		return keysList[i]["name"].(string) < keysList[j]["name"].(string)
	})

	writeJSON(w, http.StatusOK, map[string]any{"keys": keysList})
}

// parseKeyExpiry accepts an absolute expiry as RFC 3339 or a plain date and
// returns the parsed time. An empty string clears the expiry.
func parseKeyExpiry(v string) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("invalid expiry date: %s", v)
}

// parseKeyRate builds a RateSpec from a request count plus interval. A
// non-positive count with a set interval clears the rate.
func parseKeyRate(count *int, interval string) *auth.RateSpec {
	if count == nil {
		return nil
	}
	if *count <= 0 {
		return nil
	}
	if interval == "" {
		interval = "1m"
	}
	return &auth.RateSpec{Requests: *count, Interval: interval}
}

func (h *DashboardHandler) handleAPIKeyCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		RateLimitRPM *int   `json:"rate_limit_rpm"`
		RateLimitRPD *int   `json:"rate_limit_rpd"`
		ExpiresAt    string `json:"expires_at"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Key name is required")
		return
	}

	var opts []auth.KeyOpt
	if req.RateLimitRPM != nil && *req.RateLimitRPM > 0 {
		opts = append(opts, auth.WithRate(&auth.RateSpec{Requests: *req.RateLimitRPM, Interval: "1m"}))
	}
	if req.RateLimitRPD != nil && *req.RateLimitRPD > 0 {
		opts = append(opts, auth.WithRate(&auth.RateSpec{Requests: *req.RateLimitRPD, Interval: "1d"}))
	}
	if req.ExpiresAt != "" && req.ExpiresAt != "never" {
		exp, err := parseKeyExpiry(req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if exp != nil {
			opts = append(opts, auth.WithExpires(exp))
		}
	}

	h.keysMu.Lock()
	defer h.keysMu.Unlock()

	keysPath := h.getKeysPath()
	kf := auth.KeyFile{}
	if data, err := os.ReadFile(keysPath); err == nil {
		_ = json.Unmarshal(data, &kf)
	}

	secret, key, err := auth.GenerateKey(name, opts...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Generate key failed: "+err.Error())
		return
	}

	kf.Keys = append(kf.Keys, key)
	if err := auth.WriteKeyFile(keysPath, kf); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to write keys file: "+err.Error())
		return
	}

	var expiresAt *int64
	if key.Expires != nil {
		ms := key.Expires.UnixMilli()
		expiresAt = &ms
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"key": secret,
		"item": map[string]any{
			"id":         key.ID,
			"name":       key.Name,
			"prefix":     key.Prefix,
			"created_at": key.Created.UnixMilli(),
			"expires":    expiresAt,
		},
	})
}

// handleAPIKeyUpdate edits a key's name, expiry, and rate without rotating
// its secret (spec: the Edit action never rotates the credential).
func (h *DashboardHandler) handleAPIKeyUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 2 {
			id = parts[len(parts)-2]
		}
	}

	var req struct {
		Name         string `json:"name"`
		ExpiresAt    string `json:"expires_at"`
		RateLimitRPM *int   `json:"rate_limit_rpm"`
		RateLimitRPD *int   `json:"rate_limit_rpd"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Key name is required")
		return
	}

	upd := auth.KeyUpdate{Name: &name}

	switch strings.TrimSpace(req.ExpiresAt) {
	case "":
		// untouched
	case "never":
		upd.ClearExpires = true
	default:
		exp, err := parseKeyExpiry(req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if exp != nil {
			upd.Expires = exp
		}
	}

	if req.RateLimitRPM != nil {
		if rate := parseKeyRate(req.RateLimitRPM, "1m"); rate != nil {
			upd.Rate = rate
		} else {
			upd.ClearRate = true
		}
	}
	if req.RateLimitRPD != nil {
		if rate := parseKeyRate(req.RateLimitRPD, "1d"); rate != nil {
			upd.Rate = rate
		} else {
			upd.ClearRate = true
		}
	}

	h.keysMu.Lock()
	defer h.keysMu.Unlock()

	if err := auth.UpdateKey(h.getKeysPath(), id, upd); err != nil {
		writeError(w, http.StatusInternalServerError, "Update key failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPIKeyRevoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 2 {
			id = parts[len(parts)-2]
		}
	}

	h.keysMu.Lock()
	defer h.keysMu.Unlock()

	keysPath := h.getKeysPath()
	if err := auth.RevokeKey(keysPath, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Revoke key failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *DashboardHandler) handleAPIClients(w http.ResponseWriter, r *http.Request) {
	all := clients.All()
	clientList := make([]map[string]any, 0, len(all))

	for _, c := range all {
		card := buildClientCard(c)
		st, _ := c.Detect()
		clientList = append(clientList, map[string]any{
			"id":           c.ID(),
			"name":         c.Name(),
			"category":     "Coding Assistant",
			"dialect":      c.Dialect(),
			"logo":         c.ID(),
			"status_state": card.StatusState,
			"status_label": card.StatusLabel,
			"installed":    st.Installed,
			"configured":   st.PointedAtTinyRoute,
			"config_path":  st.ConfigPath,
		})
	}

	sort.Slice(clientList, func(i, j int) bool {
		return clientList[i]["name"].(string) < clientList[j]["name"].(string)
	})

	writeJSON(w, http.StatusOK, map[string]any{"clients": clientList})
}

func (h *DashboardHandler) handleAPIClientDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/dashboard/api/clients/")
	}

	c, ok := clients.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "Client not found")
		return
	}

	st, _ := c.Detect()
	card := buildClientCard(c)

	listen := h.deps.Service.Listen
	if listen == "" {
		listen = "127.0.0.1:8080"
	}

	// Dialect-mapped endpoints, defaulting to the endpoint derived from the
	// client's own dialect (spec: management-dashboard / client detail view).
	defaultBaseURL := clients.DialectBaseURL(listen, c.Dialect())
	endpointURLs := []string{defaultBaseURL}
	for _, d := range []string{"anthropic", "openai", "gemini"} {
		if ep := clients.DialectBaseURL(listen, d); ep != defaultBaseURL {
			endpointURLs = append(endpointURLs, ep)
		}
	}
	endpoints := make([]map[string]any, 0, len(endpointURLs))
	for i, ep := range endpointURLs {
		endpoints = append(endpoints, map[string]any{
			"url":        ep,
			"is_default": i == 0,
			"is_current": st.CurrentBaseURL == ep,
		})
	}

	routableModels := clients.DiscoverModelsForDialect(c.Dialect())

	modelSlots := make([]map[string]any, 0)
	for _, s := range c.ModelSlots() {
		modelSlots = append(modelSlots, map[string]any{
			"id":       s.ID,
			"name":     s.Name,
			"kind":     int(s.Kind),
			"required": s.Required,
		})
	}

	// Existing keys offered for reuse: only active keys with a recoverable
	// secret qualify (disabled or secret-less keys cannot be embedded).
	existingKeys := make([]map[string]any, 0)
	selectedKeyID := ""
	if h.deps.KeyWatcher != nil {
		if ks := h.deps.KeyWatcher.Get(); ks != nil {
			if st.RawKey != "" {
				if matched, ok := ks.LookupByToken(st.RawKey); ok {
					selectedKeyID = matched.ID
				}
			}
			for _, k := range ks.Keys() {
				if k.Disabled || k.Secret == "" {
					continue
				}
				existingKeys = append(existingKeys, map[string]any{
					"id":     k.ID,
					"name":   k.Name,
					"prefix": k.Prefix,
				})
			}
		}
	}

	var envVars, envToken string
	if c.Dialect() == "anthropic" {
		envVars, envToken = "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"
	} else {
		envVars, envToken = "OPENAI_BASE_URL", "OPENAI_API_KEY"
	}
	snippet := fmt.Sprintf("export %s=%s\nexport %s=<YOUR_TINYROUTE_KEY>", envVars, defaultBaseURL, envToken)

	writeJSON(w, http.StatusOK, map[string]any{
		"id":               c.ID(),
		"name":             c.Name(),
		"dialect":          c.Dialect(),
		"category":         "Coding Assistant",
		"logo":             c.ID(),
		"status_state":     card.StatusState,
		"status_label":     card.StatusLabel,
		"installed":        st.Installed,
		"configured":       st.PointedAtTinyRoute,
		"config_path":      st.ConfigPath,
		"current_base_url": st.CurrentBaseURL,
		"default_base_url": defaultBaseURL,
		"endpoints":        endpoints,
		"routable_models":  routableModels,
		"model_slots":      modelSlots,
		"slot_values":      st.SlotValues,
		"masked_key":       st.MaskedKey,
		"existing_keys":    existingKeys,
		"selected_key_id":  selectedKeyID,
		"manual_snippet":   snippet,
	})
}

// clientInstallRequest is the JSON payload shared by the client plan and
// apply endpoints.
type clientInstallRequest struct {
	BaseURL       string            `json:"base_url"`
	APIKey        string            `json:"api_key"`
	KeyStrategy   string            `json:"key_strategy"`
	KeyName       string            `json:"key_name"`
	Model         string            `json:"model"`
	Models        []string          `json:"models"`
	ModelSlots    map[string]string `json:"model_slots"`
	Slots         map[string]string `json:"slots"`
	ExistingKeyID string            `json:"key_id"`
	ContextWindow string            `json:"context_window"`
}

// resolveClientPlan turns a JSON client request into an installer plan,
// applying the same key-strategy defaults as the CLI-driven flow.
func (h *DashboardHandler) resolveClientPlan(c clients.Client, req clientInstallRequest) (*clients.Plan, error) {
	apiKey := req.APIKey
	if apiKey == "" && req.ExistingKeyID != "" {
		if secret, ok := h.resolveExistingKey(req.ExistingKeyID); ok {
			apiKey = secret
		}
	}
	if apiKey == "" && (req.KeyStrategy == "reuse" || req.KeyStrategy == "") {
		if st, _ := c.Detect(); st.RawKey != "" {
			apiKey = st.RawKey
		}
	}

	keyStrategy := clients.KeyStrategy(req.KeyStrategy)
	if keyStrategy == "" {
		if apiKey != "" {
			keyStrategy = clients.KeyStrategyReuse
		} else {
			keyStrategy = clients.KeyStrategyMint
		}
	}

	slotValues := make(map[string]string, len(req.ModelSlots)+len(req.Slots))
	for k, v := range req.ModelSlots {
		if v != "" {
			slotValues[k] = v
		}
	}
	for k, v := range req.Slots {
		if v != "" {
			slotValues[k] = v
		}
	}

	primaryModel := req.Model
	if primaryModel == "" && len(req.Models) > 0 {
		primaryModel = req.Models[0]
	}

	installer := clients.NewInstaller(h.deps.Service.Listen, h.deps.Service.KeysPath)
	plan, err := installer.Plan(clients.InstallRequest{
		ClientID:      c.ID(),
		BaseURL:       req.BaseURL,
		APIKey:        apiKey,
		KeyStrategy:   keyStrategy,
		KeyName:       req.KeyName,
		Model:         primaryModel,
		Models:        req.Models,
		ModelSlots:    slotValues,
		ContextWindow: req.ContextWindow,
	})
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func (h *DashboardHandler) handleAPIClientPlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, ok := clients.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "Client not found")
		return
	}

	var req clientInstallRequest
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	plan, err := h.resolveClientPlan(c, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"plan":    plan,
		"preview": formatClientPlanPreview(plan),
	})
}

func (h *DashboardHandler) handleAPIClientApply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, ok := clients.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "Client not found")
		return
	}

	var req clientInstallRequest
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	plan, err := h.resolveClientPlan(c, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	installer := clients.NewInstaller(h.deps.Service.Listen, h.deps.Service.KeysPath)
	res, err := installer.Apply(plan)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := map[string]any{
		"success": true,
		"message": "Configuration applied to " + c.Name() + ".",
		"files":   res.Files,
	}
	// A minted key's plaintext is revealed exactly once, in this response.
	if plan.KeyStrategy == clients.KeyStrategyMint && res.Key != "" {
		resp["minted_key"] = res.Key
		resp["message"] = "Configuration applied to " + c.Name() + ". A new key was minted for it."
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *DashboardHandler) handleAPIClientReset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, ok := clients.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "Client not found")
		return
	}

	if err := c.Reset(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Client reset to defaults.",
	})
}

// formatClientPlanPreview renders a short human-readable summary of an
// install plan for the SPA's plan-preview pane.
func formatClientPlanPreview(p *clients.Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Client:      %s (%s)\n", p.ClientName, p.Dialect)
	fmt.Fprintf(&b, "Endpoint:    %s\n", p.BaseURL)
	if p.KeyStrategy == clients.KeyStrategyMint {
		b.WriteString("API key:     mint a new key\n")
	} else {
		b.WriteString("API key:     reuse the current key\n")
	}
	if len(p.ModelSlots) > 0 {
		b.WriteString("Model slots:\n")
		keys := make([]string, 0, len(p.ModelSlots))
		for k := range p.ModelSlots {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  %-12s %s\n", k, p.ModelSlots[k])
		}
	}
	fmt.Fprintf(&b, "Config path: %s\n", p.ConfigPath)
	if p.HasBackup {
		fmt.Fprintf(&b, "Backup:      %s\n", p.BackupPath)
	}
	return b.String()
}

func (h *DashboardHandler) handleAPISettings(w http.ResponseWriter, r *http.Request) {
	isDefault := false
	if h.deps.PasswordStore != nil {
		isDefault = h.deps.PasswordStore.IsDefaultPassword()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"listen":              h.deps.Service.Listen,
		"config_path":         h.deps.Service.ConfigPath,
		"keys_path":           h.deps.Service.KeysPath,
		"history_db_path":     h.deps.Service.HistoryDBPath,
		"log_level":           h.deps.Service.LogLevel,
		"capture_mode":        string(h.deps.Service.Capture),
		"is_default_password": isDefault,
		"tls_enabled":         h.deps.Service.TLSCert != "",
		"version":             "0.1.0",
	})
}

func (h *DashboardHandler) handleAPIPasswordChange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := parseJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if h.deps.PasswordStore == nil || !h.deps.PasswordStore.VerifyPassword(req.OldPassword) {
		writeError(w, http.StatusBadRequest, "Current password is incorrect")
		return
	}

	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "New password must be at least 8 characters")
		return
	}

	if err := h.deps.PasswordStore.SetPassword(req.NewPassword); err != nil {
		writeError(w, http.StatusInternalServerError, "Save password error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
