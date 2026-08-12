package credential

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// RefreshRequest encapsulates parameters for an OAuth token refresh call.
type RefreshRequest struct {
	Provider            string
	RefreshToken        string
	ClientID            string
	ClientSecret        string
	TokenEndpoint       string
	Profile             RefreshProfile
	DeviceID            string
	DeviceHeaderProfile string
	HTTPClient          *http.Client
}

// Key returns the singleflight and cache key in the format `provider:tokenSuffix`.
func (r RefreshRequest) Key() string {
	suffix := r.RefreshToken
	if len(suffix) > 6 {
		suffix = suffix[len(suffix)-6:]
	}
	return r.Provider + ":" + suffix
}

type callResult struct {
	tokenRes        TokenResult
	tokenExpiresAt  time.Time
	newRefreshToken string
	err             error
}

type call struct {
	wg  sync.WaitGroup
	res callResult
}

type singleflightGroup struct {
	mu sync.Mutex
	m  map[string]*call
}

func (g *singleflightGroup) Do(key string, fn func() (TokenResult, time.Time, string, error)) (TokenResult, time.Time, string, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.res.tokenRes, c.res.tokenExpiresAt, c.res.newRefreshToken, c.res.err
	}
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	res, exp, newRT, err := fn()
	c.res = callResult{
		tokenRes:        res,
		tokenExpiresAt:  exp,
		newRefreshToken: newRT,
		err:             err,
	}
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return res, exp, newRT, err
}

type cacheEntry struct {
	res            TokenResult
	tokenExpiresAt time.Time
	cacheExpiresAt time.Time
}

// RefreshManager manages singleflight deduplication and a 10s result cache for token refreshes.
type RefreshManager struct {
	mu         sync.Mutex
	cache      map[string]cacheEntry
	sf         singleflightGroup
	httpClient *http.Client
}

// DefaultRefreshManager is a package-level default RefreshManager.
var DefaultRefreshManager = NewRefreshManager(nil)

// NewRefreshManager constructs a RefreshManager with an optional HTTP client.
func NewRefreshManager(httpClient *http.Client) *RefreshManager {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &RefreshManager{
		cache:      make(map[string]cacheEntry),
		httpClient: httpClient,
	}
}

// ClearCache clears all cached refresh results.
func (rm *RefreshManager) ClearCache() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.cache = make(map[string]cacheEntry)
}

// RefreshToken executes an OAuth refresh request, deduplicating concurrent calls
// for the same provider:tokenSuffix and caching successful results for 10 seconds.
// Failed refreshes are not cached.
func (rm *RefreshManager) RefreshToken(ctx context.Context, req RefreshRequest) (TokenResult, time.Time, string, error) {
	key := req.Key()

	// 1. Check 10s cache
	rm.mu.Lock()
	now := time.Now()
	if entry, ok := rm.cache[key]; ok {
		if now.Before(entry.cacheExpiresAt) {
			rm.mu.Unlock()
			return entry.res, entry.tokenExpiresAt, "", nil
		}
		delete(rm.cache, key)
	}
	rm.mu.Unlock()

	// 2. Singleflight execution
	client := req.HTTPClient
	if client == nil {
		client = rm.httpClient
	}

	tokenRes, tokenExp, newRT, err := rm.sf.Do(key, func() (TokenResult, time.Time, string, error) {
		httpReq, err := BuildRefreshRequest(ctx, req)
		if err != nil {
			return TokenResult{}, time.Time{}, "", fmt.Errorf("oauth: build refresh request: %w", err)
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			return TokenResult{}, time.Time{}, "", fmt.Errorf("oauth: refresh request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return TokenResult{}, time.Time{}, "", fmt.Errorf("oauth: read refresh response: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return TokenResult{}, time.Time{}, "", fmt.Errorf("oauth: refresh returned status %d: %s", resp.StatusCode, string(body))
		}

		var tokenData struct {
			AccessToken  string `json:"access_token"`
			TokenType    string `json:"token_type"`
			ExpiresIn    int64  `json:"expires_in"`
			RefreshToken string `json:"refresh_token"`
			Data         struct {
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
				TokenType    string `json:"tokenType"`
				ExpiresAt    string `json:"expiresAt"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &tokenData); err != nil {
			return TokenResult{}, time.Time{}, "", fmt.Errorf("oauth: parse refresh response: %w", err)
		}

		accessToken := tokenData.AccessToken
		if accessToken == "" {
			accessToken = tokenData.Data.AccessToken
		}
		if (req.Provider == "cline" || req.Provider == "clinepass") && accessToken != "" && !strings.HasPrefix(accessToken, "workos:") {
			accessToken = "workos:" + accessToken
		}
		if accessToken == "" {
			return TokenResult{}, time.Time{}, "", fmt.Errorf("oauth: refresh response missing access_token")
		}

		refreshToken := tokenData.RefreshToken
		if refreshToken == "" {
			refreshToken = tokenData.Data.RefreshToken
		}

		var exp time.Time
		if tokenData.ExpiresIn > 0 {
			exp = time.Now().Add(time.Duration(tokenData.ExpiresIn) * time.Second)
		} else if tokenData.Data.ExpiresAt != "" {
			if parsedExp, err := time.Parse(time.RFC3339, tokenData.Data.ExpiresAt); err == nil {
				exp = parsedExp
			} else {
				exp = time.Now().Add(1 * time.Hour)
			}
		} else {
			exp = time.Now().Add(1 * time.Hour)
		}

		res := TokenResult{
			Value: accessToken,
			Kind:  KindOAuthBearer,
		}
		return res, exp, refreshToken, nil
	})

	if err != nil {
		return TokenResult{}, time.Time{}, "", err
	}

	// 3. Cache successful result for 10s
	rm.mu.Lock()
	rm.cache[key] = cacheEntry{
		res:            tokenRes,
		tokenExpiresAt: tokenExp,
		cacheExpiresAt: time.Now().Add(10 * time.Second),
	}
	rm.mu.Unlock()

	return tokenRes, tokenExp, newRT, nil
}

// ApplyDeviceHeaders sets provider-specific device identity headers on an HTTP request.
func ApplyDeviceHeaders(req *http.Request, profile, deviceID string) {
	if profile == "kimi" {
		req.Header.Set("X-Msh-Platform", "desktop")
		req.Header.Set("X-Msh-Version", "1.0.0")
		hostname, err := os.Hostname()
		if err != nil || hostname == "" {
			hostname = "tinyroute"
		}
		req.Header.Set("X-Msh-Device-Name", hostname)
		req.Header.Set("X-Msh-Device-Model", "tinyroute")
		if deviceID != "" {
			req.Header.Set("X-Msh-Device-Id", deviceID)
		}
	}
}

// BuildRefreshRequest creates an *http.Request matching the RefreshProfile specifications.
func BuildRefreshRequest(ctx context.Context, req RefreshRequest) (*http.Request, error) {
	grantType := req.Profile.GrantType
	if grantType == "" {
		grantType = "refresh_token"
	}

	format := req.Profile.BodyFormat
	if format == "" {
		format = FormatJSON
	}

	targetURL := req.TokenEndpoint
	if (req.Provider == "cline" || req.Provider == "clinepass") && strings.Contains(targetURL, "/auth/token") {
		targetURL = strings.Replace(targetURL, "/auth/token", "/auth/refresh", 1)
	}

	var httpReq *http.Request
	var err error

	switch format {
	case FormatForm:
		form := url.Values{}
		form.Set("grant_type", grantType)
		form.Set("refresh_token", req.RefreshToken)
		if !req.Profile.UseBasicAuth && req.ClientID != "" {
			form.Set("client_id", req.ClientID)
		}
		if req.Profile.IncludeClientSecret && req.ClientSecret != "" {
			form.Set("client_secret", req.ClientSecret)
		}
		bodyStr := form.Encode()
		httpReq, err = http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(bodyStr))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	case FormatJSON:
		var bodyBytes []byte
		if req.Provider == "cline" || req.Provider == "clinepass" {
			bodyMap := map[string]string{
				"grantType":    grantType,
				"refreshToken": req.RefreshToken,
			}
			bodyBytes, err = json.Marshal(bodyMap)
		} else {
			bodyMap := map[string]string{
				"grant_type":    grantType,
				"refresh_token": req.RefreshToken,
			}
			if !req.Profile.UseBasicAuth && req.ClientID != "" {
				bodyMap["client_id"] = req.ClientID
			}
			if req.Profile.IncludeClientSecret && req.ClientSecret != "" {
				bodyMap["client_secret"] = req.ClientSecret
			}
			bodyBytes, err = json.Marshal(bodyMap)
		}
		if err != nil {
			return nil, fmt.Errorf("oauth: marshal json body: %w", err)
		}
		httpReq, err = http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")

	default:
		return nil, fmt.Errorf("oauth: unknown body format %q", format)
	}

	if req.Profile.UseBasicAuth {
		auth := req.ClientID + ":" + req.ClientSecret
		encoded := base64.StdEncoding.EncodeToString([]byte(auth))
		httpReq.Header.Set("Authorization", "Basic "+encoded)
	}

	if req.DeviceHeaderProfile != "" {
		ApplyDeviceHeaders(httpReq, req.DeviceHeaderProfile, req.DeviceID)
	}

	return httpReq, nil
}
