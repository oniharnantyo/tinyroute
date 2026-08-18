package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/auth"
	"github.com/oniharnantyo/tinyroute/internal/clients"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/dashboard/assets"
	"github.com/oniharnantyo/tinyroute/internal/dashboard/components"
	"github.com/oniharnantyo/tinyroute/internal/history"
	"github.com/oniharnantyo/tinyroute/internal/oauth"
	"github.com/oniharnantyo/tinyroute/internal/preset"
	"github.com/oniharnantyo/tinyroute/internal/route"
)

type Deps struct {
	Service         config.Service
	PasswordStore   *PasswordStore
	SessionStore    *SessionStore
	LoginLimiter    *LoginLimiter
	TopologyWatcher *config.Watcher[config.Topology]
	KeyWatcher      *config.Watcher[auth.KeyStore]
	HealthStore     *route.HealthStore
	HistoryQuerier  history.Querier
	// RunProbe executes a model probe through the gateway's real in-process
	// request path (route → translate → failover → upstream), bypassing API-key
	// auth. Wired by serve.go.
	RunProbe func(ctx context.Context, provName, dialectName, model string, timeout time.Duration) (int, time.Duration, error)
}

type OAuthStateSession struct {
	Provider    string
	Verifier    string
	RedirectURI string
	CreatedAt   time.Time
}

type DashboardHandler struct {
	deps            *Deps
	oauthStateStore map[string]OAuthStateSession
	mu              sync.RWMutex
	keysMu          sync.Mutex
}

func NewHandler(deps *Deps) *DashboardHandler {
	return &DashboardHandler{
		deps:            deps,
		oauthStateStore: make(map[string]OAuthStateSession),
	}
}

func RegisterRoutes(mux *http.ServeMux, deps *Deps) {
	h := NewHandler(deps)

	// Static assets handler
	assetsFS := http.FileServer(assets.FS())
	mux.Handle("GET /dashboard/assets/", http.StripPrefix("/dashboard/assets/", assetsFS))

	// shadcn-templ component JS bundle (inputgroup, etc.)
	mux.Handle("GET /components/", components.ScriptsHandler())

	// Auth routes
	mux.HandleFunc("GET /dashboard/login", h.handleLoginView)
	mux.HandleFunc("POST /dashboard/login", h.handleLoginSubmit)
	mux.HandleFunc("GET /dashboard/logout", h.handleLogout)

	// Protected dashboard routes
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("GET /dashboard", h.handleOverviewView)
	protectedMux.HandleFunc("GET /dashboard/", h.handleOverviewView)
	protectedMux.HandleFunc("GET /dashboard/overview", h.handleOverviewView)

	protectedMux.HandleFunc("GET /dashboard/providers", h.handleProvidersView)
	protectedMux.HandleFunc("POST /dashboard/providers/add", h.handleProviderAdd)
	protectedMux.HandleFunc("POST /dashboard/providers/add-custom", h.handleCustomProviderAdd)
	protectedMux.HandleFunc("POST /dashboard/providers/delete", h.handleProviderDelete)
	protectedMux.HandleFunc("POST /dashboard/providers/credential", h.handleProviderCredential)
	protectedMux.HandleFunc("POST /dashboard/providers/credential/delete", h.handleProviderCredentialDelete)
	protectedMux.HandleFunc("GET /dashboard/providers/", h.handleProviderDetailView)

	// OAuth routes
	protectedMux.HandleFunc("GET /dashboard/oauth/callback", h.handleOAuthCallback)
	protectedMux.HandleFunc("GET /dashboard/providers/{name}/oauth/start", h.handleOAuthStart)
	protectedMux.HandleFunc("POST /dashboard/providers/{name}/oauth/device/poll", h.handleOAuthDevicePoll)

	protectedMux.HandleFunc("POST /dashboard/models/add", h.handleModelAdd)
	protectedMux.HandleFunc("POST /dashboard/models/remove", h.handleModelRemove)
	protectedMux.HandleFunc("POST /dashboard/models/test", h.handleModelTest)

	protectedMux.HandleFunc("GET /dashboard/routes", h.handleRoutesView)
	protectedMux.HandleFunc("GET /dashboard/history", h.handleHistoryView)
	protectedMux.HandleFunc("GET /dashboard/history/{id}", h.handleHistoryDetailView)
	protectedMux.HandleFunc("GET /dashboard/keys", h.handleKeysView)
	protectedMux.HandleFunc("POST /dashboard/keys/create", h.handleKeyCreate)
	protectedMux.HandleFunc("POST /dashboard/keys/{id}/update", h.handleKeyUpdate)
	protectedMux.HandleFunc("POST /dashboard/keys/{id}/revoke", h.handleKeyRevoke)

	protectedMux.HandleFunc("GET /dashboard/settings", h.handleSettingsView)
	protectedMux.HandleFunc("POST /dashboard/settings/password", h.handlePasswordChange)

	protectedMux.HandleFunc("GET /dashboard/clients", h.handleClientsView)
	protectedMux.HandleFunc("GET /dashboard/clients/{id}", h.handleClientDetailView)
	protectedMux.HandleFunc("POST /dashboard/clients/{id}/plan", h.handleClientPlan)
	protectedMux.HandleFunc("POST /dashboard/clients/{id}/apply", h.handleClientApply)
	protectedMux.HandleFunc("POST /dashboard/clients/{id}/reset", h.handleClientReset)

	// Wrap protected routes with Auth + Host/Origin guard middleware
	guarded := HostGuardMiddleware(h.authMiddleware(protectedMux))
	mux.Handle("/dashboard/", guarded)
}

func (h *DashboardHandler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || cookie == nil || !h.deps.SessionStore.ValidateSession(cookie.Value) {
			http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *DashboardHandler) handleLoginView(w http.ResponseWriter, r *http.Request) {
	errStr := r.URL.Query().Get("error")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	Login(errStr).Render(r.Context(), w)
}

func (h *DashboardHandler) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	if allowed, retryAfter := h.deps.LoginLimiter.Allow(clientIP); !allowed {
		msg := fmt.Sprintf("Too many login attempts. Retry in %d seconds.", int(retryAfter.Seconds()))
		http.Redirect(w, r, "/dashboard/login?error="+url.QueryEscape(msg), http.StatusSeeOther)
		return
	}

	password := r.FormValue("password")
	if !h.deps.PasswordStore.VerifyPassword(password) {
		http.Redirect(w, r, "/dashboard/login?error="+url.QueryEscape("Invalid password"), http.StatusSeeOther)
		return
	}

	token := h.deps.SessionStore.CreateSession(24 * time.Hour)
	// SameSite=Lax is required so the session cookie is sent on the top-level
	// GET redirect from an OAuth provider (e.g. Antigravity PKCE flow) back to
	// /dashboard/oauth/callback. Strict would be withheld by the browser on
	// that cross-site navigation and the callback would bounce to /dashboard/login.
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/dashboard",
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/dashboard/overview", http.StatusSeeOther)
}

func (h *DashboardHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie != nil {
		h.deps.SessionStore.RevokeSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/dashboard",
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
}

func (h *DashboardHandler) handleOverviewView(w http.ResponseWriter, r *http.Request) {
	data := OverviewData{}
	topo := h.deps.TopologyWatcher.Get()

	// Gather provider health status
	if topo != nil {
		for name, p := range topo.Providers {
			status := "healthy"
			cooldownUntil := ""
			if h.deps.HealthStore != nil {
				if until := h.deps.HealthStore.CooldownEnd(name); !until.IsZero() {
					status = "cooldown"
					cooldownUntil = until.Format("15:04:05")
				}
			}
			data.ProviderHealthList = append(data.ProviderHealthList, ProviderHealthItem{
				Name:          name,
				Dialect:       p.Dialect,
				Status:        status,
				CooldownUntil: cooldownUntil,
			})
		}
		sort.Slice(data.ProviderHealthList, func(i, j int) bool {
			return data.ProviderHealthList[i].Name < data.ProviderHealthList[j].Name
		})
	}

	// Gather stats from SQLite history store
	if h.deps.HistoryQuerier != nil {
		if recs, _, err := h.deps.HistoryQuerier.List(r.Context(), history.Filter{Limit: 100}); err == nil {
			data.TotalRequests = int64(len(recs))
			var successCount int64
			for _, rec := range recs {
				if rec.Outcome == "success" {
					successCount++
				} else {
					if len(data.RecentFailures) < 10 {
						data.RecentFailures = append(data.RecentFailures, RecentFailureItem{
							ID:         rec.ID,
							Time:       rec.Timestamp.Format("15:04:05"),
							Provider:   rec.Provider,
							Model:      rec.ModelReq,
							StatusCode: 500,
							ErrorMsg:   rec.Outcome,
						})
					}
				}
				data.TotalPromptTokens += rec.InputTokens
				data.TotalCompletionTokens += rec.OutputTokens
			}
			if data.TotalRequests > 0 {
				data.SuccessRate = float64(successCount) / float64(data.TotalRequests) * 100.0
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	Layout("Overview", "overview", h.deps.PasswordStore.IsDefaultPassword(), OverviewPage(data)).Render(r.Context(), w)
}

func titleCase(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func (h *DashboardHandler) handleProvidersView(w http.ResponseWriter, r *http.Request) {
	data := ProvidersPageData{}

	topo := h.deps.TopologyWatcher.Get()
	credStore, _ := credential.NewStore(h.deps.Service.CredentialsPath)
	hasCredMap := make(map[string]bool)
	connectionsMap := make(map[string][]MaskedConnectionItem)
	if credStore != nil {
		for _, rec := range credStore.ListMasked() {
			hasCredMap[rec.Provider] = true
			account := rec.Account
			if account == "" {
				account = "default"
			}
			exp := "Never"
			if !rec.ExpiresAt.IsZero() {
				exp = rec.ExpiresAt.Format("2006-01-02 15:04:05")
			}
			connectionsMap[rec.Provider] = append(connectionsMap[rec.Provider], MaskedConnectionItem{
				Account:      account,
				Provider:     rec.Provider,
				RefreshToken: rec.RefreshToken,
				ExpiresAt:    exp,
			})
		}
	}

	configuredProviders := make(map[string]config.Provider)
	if topo != nil && topo.Providers != nil {
		configuredProviders = topo.Providers
	}

	matchedConfiguredKeys := make(map[string]bool)
	var allCards []ProviderCardData

	// 1. Process all preset templates
	for _, pre := range preset.All() {
		authType := "—"
		if pre.OAuthCapable {
			authType = "oauth"
		} else if pre.CredentialVar != "" {
			authType = "api key"
		}

		section := "API Key"
		if pre.Tier != "" {
			section = "Free Tier"
		} else if pre.OAuthCapable {
			section = "OAuth"
		}

		var matchedKey string
		for name := range configuredProviders {
			if strings.EqualFold(name, pre.Name) || (preset.Get(name) != nil && preset.Get(name).Name == pre.Name) {
				matchedKey = name
				break
			}
		}

		var card ProviderCardData
		if matchedKey != "" {
			matchedConfiguredKeys[matchedKey] = true
			p := configuredProviders[matchedKey]

			dispName := titleCase(matchedKey)
			if pre.DisplayName != "" {
				dispName = pre.DisplayName
			}

			connCount := len(connectionsMap[matchedKey])
			if connCount == 0 && p.APIKey != "" {
				connCount = 1
			}

			healthStatus := "healthy"
			if h.deps.HealthStore != nil {
				if until := h.deps.HealthStore.CooldownEnd(matchedKey); !until.IsZero() {
					healthStatus = "cooldown"
				}
			}

			card = ProviderCardData{
				Name:             matchedKey,
				DisplayName:      dispName,
				Logo:             pre.Logo,
				Dialect:          p.Dialect,
				BaseURL:          p.BaseURL,
				HasAPIKey:        p.APIKey != "",
				HasCredential:    hasCredMap[matchedKey],
				HealthStatus:     healthStatus,
				ConnectionCount:  connCount,
				WhitelistedCount: len(p.Models),
				Configured:       true,
				AuthType:         authType,
				Tier:             pre.Tier,
				FreeNote:         pre.FreeNote,
				Section:          section,
			}
		} else {
			dispName := titleCase(pre.Name)
			if pre.DisplayName != "" {
				dispName = pre.DisplayName
			}
			card = ProviderCardData{
				Name:             pre.Name,
				DisplayName:      dispName,
				Logo:             pre.Logo,
				Dialect:          pre.Dialect,
				BaseURL:          pre.BaseURL,
				HasAPIKey:        false,
				HasCredential:    false,
				HealthStatus:     "",
				ConnectionCount:  0,
				WhitelistedCount: 0,
				Configured:       false,
				AuthType:         authType,
				Tier:             pre.Tier,
				FreeNote:         pre.FreeNote,
				Section:          section,
			}
		}
		allCards = append(allCards, card)
	} // 2. Custom configured providers not in preset.All()
	for name, p := range configuredProviders {
		if matchedConfiguredKeys[name] {
			continue
		}

		connCount := len(connectionsMap[name])
		if connCount == 0 && p.APIKey != "" {
			connCount = 1
		}

		healthStatus := "healthy"
		if h.deps.HealthStore != nil {
			if until := h.deps.HealthStore.CooldownEnd(name); !until.IsZero() {
				healthStatus = "cooldown"
			}
		}

		card := ProviderCardData{
			Name:             name,
			DisplayName:      titleCase(name),
			Logo:             "",
			Dialect:          p.Dialect,
			BaseURL:          p.BaseURL,
			HasAPIKey:        p.APIKey != "",
			HasCredential:    hasCredMap[name],
			HealthStatus:     healthStatus,
			ConnectionCount:  connCount,
			WhitelistedCount: len(p.Models),
			Configured:       true,
			AuthType:         "api key",
			Tier:             "",
			FreeNote:         "",
			Section:          "API Key",
		}
		allCards = append(allCards, card)
	}

	// 3. Group into non-empty sections in order: Free Tier -> OAuth -> API Key
	sectionTitles := []string{"Free Tier", "OAuth", "API Key"}
	for _, title := range sectionTitles {
		var sectionCards []ProviderCardData
		for _, card := range allCards {
			if card.Section == title {
				sectionCards = append(sectionCards, card)
			}
		}
		if len(sectionCards) > 0 {
			sort.Slice(sectionCards, func(i, j int) bool {
				nameI := sectionCards[i].DisplayName
				if nameI == "" {
					nameI = sectionCards[i].Name
				}
				nameJ := sectionCards[j].DisplayName
				if nameJ == "" {
					nameJ = sectionCards[j].Name
				}
				return strings.ToLower(nameI) < strings.ToLower(nameJ)
			})
			data.Sections = append(data.Sections, ProviderSection{
				Title: title,
				Cards: sectionCards,
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	Layout("Providers", "providers", h.deps.PasswordStore.IsDefaultPassword(), ProvidersPage(data)).Render(r.Context(), w)
}

func (h *DashboardHandler) handleProviderDetailView(w http.ResponseWriter, r *http.Request) {
	provName := strings.TrimPrefix(r.URL.Path, "/dashboard/providers/")
	provName = strings.TrimSuffix(provName, "/")
	if provName == "" {
		http.Redirect(w, r, "/dashboard/providers", http.StatusSeeOther)
		return
	}

	topo := h.deps.TopologyWatcher.Get()
	if topo == nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Topology unavailable"), http.StatusSeeOther)
		return
	}

	p, configured := topo.Providers[provName]
	pre := preset.Get(provName)
	if !configured && pre == nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Provider not found"), http.StatusSeeOther)
		return
	}

	dialect := ""
	baseURL := ""
	apiKey := ""
	oauthCapable := false

	if configured {
		dialect = p.Dialect
		baseURL = p.BaseURL
		apiKey = p.APIKey
	} else if pre != nil {
		dialect = pre.Dialect
		baseURL = pre.BaseURL
	}
	if pre != nil {
		oauthCapable = pre.OAuthCapable
	}

	credStore, _ := credential.NewStore(h.deps.Service.CredentialsPath)
	var connections []MaskedConnectionItem
	if credStore != nil {
		for _, rec := range credStore.ListMasked() {
			if strings.EqualFold(rec.Provider, provName) {
				acc := rec.Account
				if acc == "" {
					acc = "default"
				}
				exp := "Never"
				if !rec.ExpiresAt.IsZero() {
					exp = rec.ExpiresAt.Format("2006-01-02 15:04:05")
				}
				connections = append(connections, MaskedConnectionItem{
					Account:      acc,
					Provider:     rec.Provider,
					RefreshToken: rec.RefreshToken,
					ExpiresAt:    exp,
				})
			}
		}
	}

	whitelistedSet := make(map[string]bool)
	if configured {
		for _, m := range p.Models {
			whitelistedSet[m] = true
		}
	}

	healthStatus := "healthy"
	if h.deps.HealthStore != nil {
		if until := h.deps.HealthStore.CooldownEnd(provName); !until.IsZero() {
			healthStatus = "cooldown"
		}
	}

	dispName := titleCase(provName)
	logo := ""
	if pre != nil {
		if pre.DisplayName != "" {
			dispName = pre.DisplayName
		}
		logo = pre.Logo
	}

	var allModels []CatalogModelItem
	seenModels := make(map[string]bool)
	addModels := func(models []string, defaultWhitelisted bool) {
		for _, m := range models {
			if m == "" || seenModels[m] {
				continue
			}
			seenModels[m] = true
			allModels = append(allModels, CatalogModelItem{
				ID:          m,
				Name:        m,
				Whitelisted: defaultWhitelisted || whitelistedSet[m],
			})
		}
	}

	if pre != nil && len(pre.Models) > 0 {
		// Curated provider: the preset's model list is authoritative. These IDs
		// (e.g. Antigravity's tiered names) are provider-specific and must not be
		// mixed with a generic live listing, so they take precedence.
		addModels(pre.Models, false)
	} else {
		// Discover the model list: live-fetch from the provider using the stored
		// credential (mirrors the CLI), then fall back to the remote catalog.
		// OAuth providers store a "${VAR}" placeholder in Provider.APIKey, so
		// prefer the credential store's access token and only fall back to a
		// non-placeholder static key.
		liveKey := ""
		if credStore != nil {
			if rec, ok := credStore.Get(provName); ok && rec.AccessToken != "" {
				liveKey = rec.AccessToken
			}
		}
		if liveKey == "" && apiKey != "" && !strings.HasPrefix(apiKey, "${") {
			liveKey = apiKey
		}
		if liveKey != "" && baseURL != "" {
			if fetched, err := config.FetchProviderModels(baseURL, liveKey, dialect); err == nil && len(fetched) > 0 {
				addModels(fetched, false)
			}
		}

		if len(allModels) == 0 {
			if cat, err := config.LoadOrRefreshCatalog("", ""); err == nil && cat != nil {
				if models, ok := cat.Providers[strings.ToLower(provName)]; ok {
					addModels(models, false)
				} else if pre != nil {
					if models, ok := cat.Providers[strings.ToLower(pre.Name)]; ok {
						addModels(models, false)
					}
				}
				if len(allModels) == 0 && dialect != "" && dialect != "openai" {
					if models, ok := cat.Providers[strings.ToLower(dialect)]; ok {
						addModels(models, false)
					}
				}
			}
		}
	}

	// Always surface any whitelisted models not already listed.
	if configured {
		addModels(p.Models, true)
	}

	// Split for the UI: whitelisted models render in a group above the
	// available-but-unwhitelisted catalog models.
	whitelistedModels, availableModels := splitCatalogModels(allModels)

	status := "not_connected"
	if configured {
		if healthStatus == "cooldown" {
			status = "cooldown"
		} else if apiKey != "" || len(connections) > 0 {
			status = "connected"
		} else {
			status = "awaiting_credentials"
		}
	}

	data := ProviderDetailPageData{
		Name:              provName,
		DisplayName:       dispName,
		Logo:              logo,
		Dialect:           dialect,
		BaseURL:           baseURL,
		APIKey:            apiKey,
		Configured:        configured,
		OAuthCapable:      oauthCapable,
		HealthStatus:      healthStatus,
		Status:            status,
		Connections:       connections,
		Models:            allModels,
		WhitelistedModels: whitelistedModels,
		AvailableModels:   availableModels,
		DeviceCode:        r.URL.Query().Get("device_code"),
		UserCode:          r.URL.Query().Get("user_code"),
		VerificationURI:   r.URL.Query().Get("verification_uri"),
		DeviceID:          r.URL.Query().Get("device_id"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	Layout("Provider Detail", "providers", h.deps.PasswordStore.IsDefaultPassword(), ProviderDetailPage(data)).Render(r.Context(), w)
}

// splitCatalogModels partitions a provider's catalog listing into the models
// already on its whitelist and the remaining available-but-unwhitelisted ones,
// each preserving the original catalog order.
func splitCatalogModels(models []CatalogModelItem) (whitelisted, available []CatalogModelItem) {
	for _, m := range models {
		if m.Whitelisted {
			whitelisted = append(whitelisted, m)
		} else {
			available = append(available, m)
		}
	}
	return whitelisted, available
}

func (h *DashboardHandler) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	provName := r.PathValue("name")
	if provName == "" {
		provName = strings.TrimPrefix(r.URL.Path, "/dashboard/providers/")
		provName = strings.TrimSuffix(provName, "/oauth/start")
	}

	pre := preset.Get(provName)
	if pre == nil || !pre.OAuthCapable {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Provider is not OAuth capable"), http.StatusSeeOther)
		return
	}

	if pre.FlowType == "device_code" {
		sess, err := oauth.StartDeviceFlow(r.Context(), pre, nil)
		if err != nil {
			http.Redirect(w, r, fmt.Sprintf("/dashboard/providers/%s?error=%s", provName, url.QueryEscape("Device flow start failed: "+err.Error())), http.StatusSeeOther)
			return
		}
		vURL := url.QueryEscape(sess.VerificationURI)
		dCode := url.QueryEscape(sess.DeviceCode)
		uCode := url.QueryEscape(sess.UserCode)
		dID := url.QueryEscape(sess.DeviceID)
		redir := fmt.Sprintf("/dashboard/providers/%s?device_code=%s&user_code=%s&verification_uri=%s&device_id=%s&interval=%d", provName, dCode, uCode, vURL, dID, sess.Interval)
		http.Redirect(w, r, redir, http.StatusSeeOther)
		return
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1:8787"
	}
	redirectURI := fmt.Sprintf("%s://%s/dashboard/oauth/callback", scheme, host)

	pkceSess, err := oauth.StartPKCE(pre, redirectURI)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/dashboard/providers/%s?error=%s", provName, url.QueryEscape("PKCE start error: "+err.Error())), http.StatusSeeOther)
		return
	}

	h.mu.Lock()
	if h.oauthStateStore == nil {
		h.oauthStateStore = make(map[string]OAuthStateSession)
	}
	now := time.Now()
	for k, v := range h.oauthStateStore {
		if now.Sub(v.CreatedAt) > 10*time.Minute {
			delete(h.oauthStateStore, k)
		}
	}
	h.oauthStateStore[pkceSess.State] = OAuthStateSession{
		Provider:    provName,
		Verifier:    pkceSess.Verifier,
		RedirectURI: redirectURI,
		CreatedAt:   now,
	}
	h.mu.Unlock()

	http.Redirect(w, r, pkceSess.AuthorizeURL, http.StatusSeeOther)
}

func (h *DashboardHandler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	errParam := r.URL.Query().Get("error")

	if state == "" {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Missing OAuth state parameter"), http.StatusSeeOther)
		return
	}

	h.mu.Lock()
	sess, ok := h.oauthStateStore[state]
	if ok {
		delete(h.oauthStateStore, state)
	}
	h.mu.Unlock()

	if !ok || time.Since(sess.CreatedAt) > 10*time.Minute {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Invalid or expired OAuth state"), http.StatusSeeOther)
		return
	}

	if errParam != "" {
		desc := r.URL.Query().Get("error_description")
		msg := fmt.Sprintf("OAuth error: %s (%s)", errParam, desc)
		http.Redirect(w, r, fmt.Sprintf("/dashboard/providers/%s?error=%s", sess.Provider, url.QueryEscape(msg)), http.StatusSeeOther)
		return
	}

	if code == "" {
		http.Redirect(w, r, fmt.Sprintf("/dashboard/providers/%s?error=%s", sess.Provider, url.QueryEscape("Missing authorization code")), http.StatusSeeOther)
		return
	}

	pre := preset.Get(sess.Provider)
	if pre == nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Preset not found"), http.StatusSeeOther)
		return
	}

	rec, err := oauth.ExchangePKCE(r.Context(), pre, nil, code, sess.Verifier, sess.RedirectURI)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/dashboard/providers/%s?error=%s", sess.Provider, url.QueryEscape("OAuth token exchange failed: "+err.Error())), http.StatusSeeOther)
		return
	}

	if data, err := os.ReadFile(h.deps.Service.ConfigPath); err == nil {
		if rawTopo, err := config.ParseRawTopology(data); err == nil {
			_, _ = ensureMaterialized(h.deps.Service.ConfigPath, &rawTopo, sess.Provider)
		}
	}

	credStore, err := credential.NewStore(h.deps.Service.CredentialsPath)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/dashboard/providers/%s?error=%s", sess.Provider, url.QueryEscape("Init credential store failed: "+err.Error())), http.StatusSeeOther)
		return
	}

	if err := credStore.Save(*rec); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/dashboard/providers/%s?error=%s", sess.Provider, url.QueryEscape("Save OAuth credential failed: "+err.Error())), http.StatusSeeOther)
		return
	}

	dispName := pre.DisplayName
	if dispName == "" {
		dispName = titleCase(pre.Name)
	}
	flash := fmt.Sprintf("Successfully connected OAuth for %s!", dispName)
	http.Redirect(w, r, fmt.Sprintf("/dashboard/providers/%s?flash=%s", sess.Provider, url.QueryEscape(flash)), http.StatusSeeOther)
}

func (h *DashboardHandler) handleOAuthDevicePoll(w http.ResponseWriter, r *http.Request) {
	provName := r.PathValue("name")
	if provName == "" {
		provName = strings.TrimPrefix(r.URL.Path, "/dashboard/providers/")
		provName = strings.TrimSuffix(provName, "/oauth/device/poll")
	}

	pre := preset.Get(provName)
	if pre == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "error", "message": "Preset not found"})
		return
	}

	deviceCode := r.FormValue("device_code")
	deviceID := r.FormValue("device_id")
	if deviceCode == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "error", "message": "Missing device_code"})
		return
	}

	rec, pending, err := oauth.PollDeviceFlow(r.Context(), pre, nil, deviceCode, deviceID)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "error", "message": err.Error()})
		return
	}
	if pending {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "pending"})
		return
	}
	if rec != nil {
		if data, err := os.ReadFile(h.deps.Service.ConfigPath); err == nil {
			if rawTopo, err := config.ParseRawTopology(data); err == nil {
				_, _ = ensureMaterialized(h.deps.Service.ConfigPath, &rawTopo, provName)
			}
		}
		credStore, err := credential.NewStore(h.deps.Service.CredentialsPath)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "error", "message": "Init credential store failed: " + err.Error()})
			return
		}
		if err := credStore.Save(*rec); err != nil {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "error", "message": "Save credential failed: " + err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "pending"})
}

func ensureMaterialized(configPath string, rawTopo *config.Topology, provName string) (materialized bool, err error) {
	provNameLower := strings.ToLower(provName)
	if rawTopo.Providers != nil {
		if _, ok := rawTopo.Providers[provNameLower]; ok {
			return false, nil
		}
		if _, ok := rawTopo.Providers[provName]; ok {
			return false, nil
		}
	}
	pre := preset.Get(provName)
	if pre == nil {
		return false, nil
	}
	apiKey := ""
	if pre.CredentialVar != "" {
		apiKey = "${" + pre.CredentialVar + "}"
	}
	if rawTopo.Providers == nil {
		rawTopo.Providers = make(map[string]config.Provider)
	}
	rawTopo.Providers[provNameLower] = config.Provider{
		Dialect: pre.Dialect,
		BaseURL: pre.BaseURL,
		APIKey:  apiKey,
	}
	if err := config.WriteTopology(configPath, *rawTopo); err != nil {
		return false, err
	}
	return true, nil
}

func (h *DashboardHandler) handleCustomProviderAdd(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.FormValue("name")))
	dialect := strings.TrimSpace(r.FormValue("dialect"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))

	if name == "" || dialect == "" || baseURL == "" {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("All fields are required for custom provider"), http.StatusSeeOther)
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Read config error: "+err.Error()), http.StatusSeeOther)
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Parse topology error: "+err.Error()), http.StatusSeeOther)
		return
	}

	if rawTopo.Providers == nil {
		rawTopo.Providers = make(map[string]config.Provider)
	}

	rawTopo.Providers[name] = config.Provider{
		Dialect: dialect,
		BaseURL: baseURL,
	}

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Write topology error: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard/providers/"+name+"?flash="+url.QueryEscape("Custom provider '"+name+"' created"), http.StatusSeeOther)
}

// handleProviderAdd retains the POST /dashboard/providers/add route as an explicit activation escape hatch.
func (h *DashboardHandler) handleProviderAdd(w http.ResponseWriter, r *http.Request) {
	presetName := r.FormValue("preset_name")
	pre := preset.Get(presetName)
	if pre == nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Unknown preset"), http.StatusSeeOther)
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Read config error: "+err.Error()), http.StatusSeeOther)
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Parse topology error: "+err.Error()), http.StatusSeeOther)
		return
	}

	provName := strings.ToLower(pre.Name)
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
	}

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Write topology error: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard/providers?flash="+url.QueryEscape("Provider '"+provName+"' added successfully"), http.StatusSeeOther)
}

func (h *DashboardHandler) handleProviderDelete(w http.ResponseWriter, r *http.Request) {
	provName := r.FormValue("name")
	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Read config error: "+err.Error()), http.StatusSeeOther)
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Parse topology error: "+err.Error()), http.StatusSeeOther)
		return
	}

	delete(rawTopo.Providers, provName)
	delete(rawTopo.Providers, strings.ToLower(provName))

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		http.Redirect(w, r, "/dashboard/providers?error="+url.QueryEscape("Write topology error: "+err.Error()), http.StatusSeeOther)
		return
	}

	if credStore, err := credential.NewStore(h.deps.Service.CredentialsPath); err == nil && credStore != nil {
		_ = credStore.DeleteProvider(provName)
	}

	http.Redirect(w, r, "/dashboard/providers?flash="+url.QueryEscape("Provider '"+provName+"' removed"), http.StatusSeeOther)
}

func detailRedirect(provName, flash, errStr string) string {
	base := "/dashboard/providers"
	if provName != "" {
		base = "/dashboard/providers/" + url.PathEscape(provName)
	}
	if flash != "" {
		return base + "?flash=" + url.QueryEscape(flash)
	}
	if errStr != "" {
		return base + "?error=" + url.QueryEscape(errStr)
	}
	return base
}

func (h *DashboardHandler) handleProviderCredential(w http.ResponseWriter, r *http.Request) {
	provName := r.FormValue("name")
	apiKey := strings.TrimSpace(r.FormValue("api_key"))

	if provName == "" {
		http.Redirect(w, r, detailRedirect(provName, "", "Provider name required"), http.StatusSeeOther)
		return
	}
	if apiKey == "" {
		http.Redirect(w, r, detailRedirect(provName, "", "API key secret cannot be empty"), http.StatusSeeOther)
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Read config error"), http.StatusSeeOther)
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Parse topology error"), http.StatusSeeOther)
		return
	}

	if _, err := ensureMaterialized(h.deps.Service.ConfigPath, &rawTopo, provName); err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Materialize error: "+err.Error()), http.StatusSeeOther)
		return
	}

	provKey := provName
	prov, ok := rawTopo.Providers[provKey]
	if !ok {
		provKey = strings.ToLower(provName)
		prov, ok = rawTopo.Providers[provKey]
	}
	if !ok {
		http.Redirect(w, r, detailRedirect(provName, "", "Provider not found"), http.StatusSeeOther)
		return
	}

	prov.APIKey = apiKey
	rawTopo.Providers[provKey] = prov

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Write topology error: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, detailRedirect(provName, "API key saved for '"+provName+"'", ""), http.StatusSeeOther)
}

func (h *DashboardHandler) handleProviderCredentialDelete(w http.ResponseWriter, r *http.Request) {
	provName := r.FormValue("name")
	credType := r.FormValue("type")
	account := r.FormValue("account")

	if provName == "" {
		http.Redirect(w, r, detailRedirect(provName, "", "Provider name required"), http.StatusSeeOther)
		return
	}

	if credType == "api_key" {
		data, err := os.ReadFile(h.deps.Service.ConfigPath)
		if err != nil {
			http.Redirect(w, r, detailRedirect(provName, "", "Read config error"), http.StatusSeeOther)
			return
		}
		rawTopo, err := config.ParseRawTopology(data)
		if err != nil {
			http.Redirect(w, r, detailRedirect(provName, "", "Parse topology error"), http.StatusSeeOther)
			return
		}
		prov, ok := rawTopo.Providers[provName]
		if ok {
			prov.APIKey = ""
			rawTopo.Providers[provName] = prov
			if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
				http.Redirect(w, r, detailRedirect(provName, "", "Write topology error: "+err.Error()), http.StatusSeeOther)
				return
			}
		}
		http.Redirect(w, r, detailRedirect(provName, "API key removed for '"+provName+"'", ""), http.StatusSeeOther)
		return
	}

	// Delete OAuth / custodian credential
	credStore, err := credential.NewStore(h.deps.Service.CredentialsPath)
	if err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Init credential store error"), http.StatusSeeOther)
		return
	}

	keyToDelete := provName
	if account != "" && account != "default" {
		keyToDelete = provName + "/" + account
	}

	if err := credStore.Delete(keyToDelete); err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Delete credential error: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, detailRedirect(provName, "OAuth connection removed", ""), http.StatusSeeOther)
}

func (h *DashboardHandler) handleModelAdd(w http.ResponseWriter, r *http.Request) {
	provName := r.FormValue("provider")
	modelName := strings.TrimSpace(r.FormValue("model"))

	if modelName == "" {
		http.Redirect(w, r, detailRedirect(provName, "", "Model name cannot be empty"), http.StatusSeeOther)
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Read config error"), http.StatusSeeOther)
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Parse topology error"), http.StatusSeeOther)
		return
	}

	if _, err := ensureMaterialized(h.deps.Service.ConfigPath, &rawTopo, provName); err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Materialize error: "+err.Error()), http.StatusSeeOther)
		return
	}

	provKey := provName
	prov, ok := rawTopo.Providers[provKey]
	if !ok {
		provKey = strings.ToLower(provName)
		prov, ok = rawTopo.Providers[provKey]
	}
	if !ok {
		http.Redirect(w, r, detailRedirect(provName, "", "Provider not found"), http.StatusSeeOther)
		return
	}

	for _, m := range prov.Models {
		if m == modelName {
			http.Redirect(w, r, detailRedirect(provName, "Model '"+modelName+"' already whitelisted", ""), http.StatusSeeOther)
			return
		}
	}

	prov.Models = append(prov.Models, modelName)
	rawTopo.Providers[provKey] = prov

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Write topology error: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, detailRedirect(provName, "Model '"+modelName+"' added to "+provName, ""), http.StatusSeeOther)
}

func (h *DashboardHandler) handleModelRemove(w http.ResponseWriter, r *http.Request) {
	provName := r.FormValue("provider")
	modelName := r.FormValue("model")

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Read config error"), http.StatusSeeOther)
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Parse topology error"), http.StatusSeeOther)
		return
	}

	prov, ok := rawTopo.Providers[provName]
	if !ok {
		http.Redirect(w, r, detailRedirect(provName, "", "Provider not found"), http.StatusSeeOther)
		return
	}

	filtered := make([]string, 0, len(prov.Models))
	for _, m := range prov.Models {
		if m != modelName {
			filtered = append(filtered, m)
		}
	}
	prov.Models = filtered
	rawTopo.Providers[provName] = prov

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		http.Redirect(w, r, detailRedirect(provName, "", "Write topology error: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, detailRedirect(provName, "Model '"+modelName+"' removed from "+provName, ""), http.StatusSeeOther)
}

func (h *DashboardHandler) handleModelTest(w http.ResponseWriter, r *http.Request) {
	provName := r.FormValue("provider")
	targetModel := r.FormValue("model")
	wantsJSON := strings.Contains(r.Header.Get("Accept"), "application/json")

	respondErr := func(msg string, code int) {
		if wantsJSON {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
			return
		}
		http.Redirect(w, r, detailRedirect(provName, "", msg), http.StatusSeeOther)
	}

	topo := h.deps.TopologyWatcher.Get()
	if topo == nil {
		respondErr("Topology unavailable", http.StatusServiceUnavailable)
		return
	}
	prov, ok := topo.Providers[provName]
	if !ok {
		respondErr("Provider not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	statusCode, elapsed, err := h.deps.RunProbe(ctx, provName, prov.Dialect, targetModel, 10*time.Second)
	if err != nil {
		respondErr(fmt.Sprintf("Probe failed: %v", err), http.StatusInternalServerError)
		return
	}

	if statusCode >= 200 && statusCode < 300 {
		msg := fmt.Sprintf("Model '%s' test OK (%d) in %v", targetModel, statusCode, elapsed.Round(time.Millisecond))
		if wantsJSON {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":         true,
				"message":    msg,
				"statusCode": statusCode,
				"elapsedMs":  elapsed.Milliseconds(),
			})
			return
		}
		http.Redirect(w, r, detailRedirect(provName, msg, ""), http.StatusSeeOther)
		return
	}

	respondErr(fmt.Sprintf("Model test returned HTTP %d", statusCode), http.StatusBadRequest)
}

func (h *DashboardHandler) handleRoutesView(w http.ResponseWriter, r *http.Request) {
	data := RoutesPageData{}
	topo := h.deps.TopologyWatcher.Get()
	if topo != nil {
		for _, r := range topo.Routes {
			data.Routes = append(data.Routes, RouteItem{
				From:  r.From,
				Match: r.Match,
				Chain: r.Chain,
			})
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	Layout("Routes", "routes", h.deps.PasswordStore.IsDefaultPassword(), RoutesPage(data)).Render(r.Context(), w)
}

func (h *DashboardHandler) handleHistoryView(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	provider := q.Get("provider")
	fromStr := q.Get("from")
	toStr := q.Get("to")

	limit := 50
	if lStr := q.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > history.MaxListLimit {
		limit = history.MaxListLimit
	}

	filter := history.Filter{
		Provider: provider,
		Limit:    limit,
	}

	if fromStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", fromStr, time.Local); err == nil {
			filter.From = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		}
	}
	if toStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", toStr, time.Local); err == nil {
			filter.To = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.Local)
		}
	}

	data := HistoryPageData{
		FilterProvider: provider,
		FilterFrom:     fromStr,
		FilterTo:       toStr,
		Limit:          limit,
	}

	if h.deps.TopologyWatcher != nil {
		if topo := h.deps.TopologyWatcher.Get(); topo != nil {
			for pName := range topo.Providers {
				data.AvailableProviders = append(data.AvailableProviders, pName)
			}
			sort.Strings(data.AvailableProviders)
		}
	}

	if h.deps.HistoryQuerier != nil {
		if recs, nextCursor, err := h.deps.HistoryQuerier.List(r.Context(), filter); err == nil {
			data.HasMore = nextCursor != ""
			for _, rec := range recs {
				attempts := decodeAttempts(rec.Attempts)
				statusCode := deriveStatusCode(rec.Outcome, attempts)
				data.Rows = append(data.Rows, HistoryRowItem{
					ID:               rec.ID,
					Timestamp:        rec.Timestamp.Local().Format("Jan 2 15:04:05"),
					Dialect:          rec.Endpoint,
					Model:            rec.ModelReq,
					KeyID:            rec.KeyID,
					Outcome:          rec.Outcome,
					StatusCode:       statusCode,
					StatusVariant:    historyStatusBadgeVariant(statusCode),
					PromptTokens:     rec.InputTokens,
					CompletionTokens: rec.OutputTokens,
					LatencyMs:        rec.Latency.Milliseconds(),
					Attempts:         attempts,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	Layout("History", "history", h.deps.PasswordStore.IsDefaultPassword(), HistoryPage(data)).Render(r.Context(), w)
}

func (h *DashboardHandler) handleHistoryDetailView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Redirect(w, r, "/dashboard/history", http.StatusSeeOther)
		return
	}

	if h.deps.HistoryQuerier == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		Layout("History Details", "history", h.deps.PasswordStore.IsDefaultPassword(), HistoryDetailNotFoundPage(id)).Render(r.Context(), w)
		return
	}

	rec, found, err := h.deps.HistoryQuerier.Get(r.Context(), id)
	if err != nil || !found {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		Layout("History Details", "history", h.deps.PasswordStore.IsDefaultPassword(), HistoryDetailNotFoundPage(id)).Render(r.Context(), w)
		return
	}

	attempts := decodeAttempts(rec.Attempts)
	statusCode := deriveStatusCode(rec.Outcome, attempts)
	statusVariant := historyStatusBadgeVariant(statusCode)

	data := HistoryDetailPageData{
		ID:                    rec.ID,
		Timestamp:             rec.Timestamp.Local().Format("Jan 2, 2006 15:04:05"),
		TimestampUTC:          rec.Timestamp.UTC().Format("2006-01-02 15:04:05 MST"),
		Provider:              rec.Provider,
		ModelReq:              rec.ModelReq,
		ModelServed:           rec.ModelServed,
		KeyID:                 rec.KeyID,
		Session:               rec.Session,
		Endpoint:              rec.Endpoint,
		Stream:                rec.Stream,
		Outcome:               rec.Outcome,
		StatusCode:            statusCode,
		StatusVariant:         statusVariant,
		InputTokens:           rec.InputTokens,
		OutputTokens:          rec.OutputTokens,
		CacheReadTokens:       rec.CacheReadTokens,
		CacheCreationTokens:   rec.CacheCreationTokens,
		TotalTokens:           rec.InputTokens + rec.OutputTokens + rec.CacheReadTokens + rec.CacheCreationTokens,
		LatencyMs:             rec.Latency.Milliseconds(),
		Attempts:              attempts,
		RequestBody:           formatBodyPane(rec.RequestBody),
		TranslatedRequestBody: formatBodyPane(rec.TranslatedRequestBody),
		RawResponseBody:       formatBodyPane(rec.RawResponseBody),
		ResponseBody:          formatBodyPane(rec.ResponseBody),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	Layout("History Details", "history", h.deps.PasswordStore.IsDefaultPassword(), HistoryDetailPage(data)).Render(r.Context(), w)
}

func (h *DashboardHandler) handleKeysView(w http.ResponseWriter, r *http.Request) {
	data := KeysPageData{
		Error:      r.URL.Query().Get("error"),
		Flash:      r.URL.Query().Get("flash"),
		ListenAddr: h.deps.Service.Listen,
	}
	if data.ListenAddr == "" {
		data.ListenAddr = "127.0.0.1:8787"
	}
	var lastUsedMap map[string]time.Time
	if h.deps.HistoryQuerier != nil {
		lastUsedMap, _ = h.deps.HistoryQuerier.LastUseByKey(r.Context())
	}

	if h.deps.KeyWatcher != nil {
		ks := h.deps.KeyWatcher.Get()
		if ks != nil {
			now := time.Now()
			for _, k := range ks.Keys() {
				if k.Disabled {
					continue
				}
				masked := k.Prefix
				if masked == "" {
					masked = "tr_live_..."
				} else {
					masked = "tr_live_" + masked + "..."
				}
				rateStr := "Unlimited"
				var rateReqs int
				var rateIntv string
				rateIntvVal := 1
				rateIntvUnit := "m"
				if k.Rate != nil {
					rateStr = fmt.Sprintf("%d / %s", k.Rate.Requests, k.Rate.Interval)
					rateReqs = k.Rate.Requests
					rateIntv = k.Rate.Interval
					if len(rateIntv) >= 2 {
						unit := rateIntv[len(rateIntv)-1:]
						valStr := rateIntv[:len(rateIntv)-1]
						if v, err := strconv.Atoi(valStr); err == nil && v > 0 {
							rateIntvVal = v
							rateIntvUnit = unit
						}
					}
				}
				expiresStr := "Never"
				if k.Expires != nil {
					expiresStr = k.Expires.Format("2006-01-02 15:04")
				}
				status := "active"
				if k.Expires != nil && now.After(*k.Expires) {
					status = "inactive"
				}
				lastUsed := "Never"
				if lastUsedMap != nil {
					if t, ok := lastUsedMap[k.ID]; ok && !t.IsZero() {
						lastUsed = t.Format("15:04:05")
					}
				}
				data.Keys = append(data.Keys, KeyItem{
					ID:           k.ID,
					Name:         k.Name,
					Prefix:       k.Prefix,
					Masked:       masked,
					Secret:       k.Secret,
					RateSpec:     rateStr,
					RateReqs:     rateReqs,
					RateIntv:     rateIntv,
					RateIntvVal:  rateIntvVal,
					RateIntvUnit: rateIntvUnit,
					Expires:      expiresStr,
					ExpiresAt:    k.Expires,
					Status:       status,
					LastUsed:     lastUsed,
				})
			}
			sort.Slice(data.Keys, func(i, j int) bool {
				return data.Keys[i].ID < data.Keys[j].ID
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	Layout("API Keys", "keys", h.deps.PasswordStore.IsDefaultPassword(), KeysPage(data)).Render(r.Context(), w)
}

func (h *DashboardHandler) getKeysPath() string {
	if h.deps.Service.KeysPath != "" {
		return h.deps.Service.KeysPath
	}
	if svc, err := config.LoadService(); err == nil && svc.KeysPath != "" {
		return svc.KeysPath
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tinyroute", "keys.json")
}

func parseExpiryFromForm(r *http.Request) (*time.Time, bool, error) {
	choice := r.FormValue("expiry_choice")
	switch choice {
	case "never":
		return nil, true, nil
	case "7d":
		exp := time.Now().Add(7 * 24 * time.Hour).UTC()
		return &exp, false, nil
	case "30d":
		exp := time.Now().Add(30 * 24 * time.Hour).UTC()
		return &exp, false, nil
	case "custom":
		daysStr := strings.TrimSpace(r.FormValue("expiry_days"))
		if daysStr != "" {
			days, err := strconv.Atoi(daysStr)
			if err != nil || days <= 0 {
				return nil, false, fmt.Errorf("Expiry days must be a positive integer")
			}
			exp := time.Now().Add(time.Duration(days) * 24 * time.Hour).UTC()
			return &exp, false, nil
		}
		customVal := strings.TrimSpace(r.FormValue("expiry_custom"))
		if customVal != "" {
			var exp time.Time
			var err error
			if exp, err = time.ParseInLocation("2006-01-02T15:04", customVal, time.Local); err != nil {
				if exp, err = time.Parse(time.RFC3339, customVal); err != nil {
					return nil, false, fmt.Errorf("Invalid expiry date format")
				}
			}
			expUTC := exp.UTC()
			if !expUTC.After(time.Now()) {
				return nil, false, fmt.Errorf("Expiry must be in the future")
			}
			return &expUTC, false, nil
		}
		return nil, false, fmt.Errorf("Custom expiry days or date must be provided")
	default:
		return nil, false, nil
	}
}

func parseRateSpecFromForm(r *http.Request) (*auth.RateSpec, bool, error) {
	hasRateSection := r.Form.Has("rate_section_present") || r.Form.Has("enable_rate")
	enableRate := r.FormValue("enable_rate") == "true"
	clearRate := r.FormValue("clear_rate") == "true" || (hasRateSection && !enableRate)

	if clearRate {
		return nil, true, nil
	}

	reqsStr := strings.TrimSpace(r.FormValue("rate_requests"))
	intvValStr := strings.TrimSpace(r.FormValue("rate_interval_val"))
	intvUnitStr := strings.TrimSpace(r.FormValue("rate_interval_unit"))
	intvStr := strings.TrimSpace(r.FormValue("rate_interval"))

	if reqsStr == "" && intvStr == "" && intvValStr == "" {
		if hasRateSection && enableRate {
			return nil, false, fmt.Errorf("Rate limit requests must be specified")
		}
		return nil, false, nil
	}

	reqs, err := strconv.Atoi(reqsStr)
	if err != nil || reqs <= 0 {
		return nil, false, fmt.Errorf("Rate limit requests must be a positive integer")
	}

	finalIntv := ""
	if intvValStr != "" && intvUnitStr != "" {
		v, err := strconv.Atoi(intvValStr)
		if err != nil || v <= 0 {
			return nil, false, fmt.Errorf("Rate limit interval value must be a positive integer")
		}
		switch intvUnitStr {
		case "s", "m", "h":
			finalIntv = fmt.Sprintf("%d%s", v, intvUnitStr)
		case "d":
			finalIntv = fmt.Sprintf("%dh", v*24)
		default:
			return nil, false, fmt.Errorf("Invalid rate limit interval unit")
		}
	} else if intvStr != "" {
		finalIntv = intvStr
	} else {
		finalIntv = "1m"
	}

	if _, err := time.ParseDuration(finalIntv); err != nil {
		return nil, false, fmt.Errorf("Invalid rate limit interval format (e.g. 1m, 1h)")
	}

	return &auth.RateSpec{
		Requests: reqs,
		Interval: finalIntv,
	}, false, nil
}

func (h *DashboardHandler) handleKeyCreate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/dashboard/keys?error="+url.QueryEscape("Key name is required"), http.StatusSeeOther)
		return
	}

	var opts []auth.KeyOpt
	exp, _, err := parseExpiryFromForm(r)
	if err != nil {
		http.Redirect(w, r, "/dashboard/keys?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if exp != nil {
		opts = append(opts, auth.WithExpires(exp))
	}

	rate, _, err := parseRateSpecFromForm(r)
	if err != nil {
		http.Redirect(w, r, "/dashboard/keys?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if rate != nil {
		opts = append(opts, auth.WithRate(rate))
	}

	h.keysMu.Lock()
	defer h.keysMu.Unlock()

	keysPath := h.getKeysPath()
	kf := auth.KeyFile{}
	if data, err := os.ReadFile(keysPath); err == nil {
		_ = json.Unmarshal(data, &kf)
	}

	_, key, err := auth.GenerateKey(name, opts...)
	if err != nil {
		http.Redirect(w, r, "/dashboard/keys?error="+url.QueryEscape("Generate key failed: "+err.Error()), http.StatusSeeOther)
		return
	}

	kf.Keys = append(kf.Keys, key)
	if err := auth.WriteKeyFile(keysPath, kf); err != nil {
		http.Redirect(w, r, "/dashboard/keys?error="+url.QueryEscape("Failed to write keys file: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard/keys?flash="+url.QueryEscape("Key '"+key.Name+"' created successfully"), http.StatusSeeOther)
}

func (h *DashboardHandler) handleKeyUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/dashboard/keys/")
		id = strings.TrimSuffix(id, "/update")
		id = strings.Trim(id, "/")
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/dashboard/keys?error="+url.QueryEscape("Key name is required"), http.StatusSeeOther)
		return
	}

	upd := auth.KeyUpdate{
		Name: &name,
	}

	exp, clearExp, err := parseExpiryFromForm(r)
	if err != nil {
		http.Redirect(w, r, "/dashboard/keys?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if clearExp {
		upd.ClearExpires = true
	} else if exp != nil {
		upd.Expires = exp
	}

	rate, clearRate, err := parseRateSpecFromForm(r)
	if err != nil {
		http.Redirect(w, r, "/dashboard/keys?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if clearRate {
		upd.ClearRate = true
	} else if rate != nil {
		upd.Rate = rate
	}

	h.keysMu.Lock()
	defer h.keysMu.Unlock()

	keysPath := h.getKeysPath()
	if err := auth.UpdateKey(keysPath, id, upd); err != nil {
		http.Redirect(w, r, "/dashboard/keys?error="+url.QueryEscape("Update key failed: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard/keys?flash="+url.QueryEscape("Key updated successfully"), http.StatusSeeOther)
}

func (h *DashboardHandler) handleKeyRevoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/dashboard/keys/")
		id = strings.TrimSuffix(id, "/revoke")
		id = strings.Trim(id, "/")
	}

	h.keysMu.Lock()
	defer h.keysMu.Unlock()

	keysPath := h.getKeysPath()
	if err := auth.RevokeKey(keysPath, id); err != nil {
		http.Redirect(w, r, "/dashboard/keys?error="+url.QueryEscape("Revoke key failed: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard/keys?flash="+url.QueryEscape("Key revoked successfully"), http.StatusSeeOther)
}

func (h *DashboardHandler) handleSettingsView(w http.ResponseWriter, r *http.Request) {
	data := SettingsPageData{}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	Layout("Settings", "settings", h.deps.PasswordStore.IsDefaultPassword(), SettingsPage(data)).Render(r.Context(), w)
}

func (h *DashboardHandler) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	curr := r.FormValue("current_password")
	newP := r.FormValue("new_password")
	confP := r.FormValue("confirm_password")

	if !h.deps.PasswordStore.VerifyPassword(curr) {
		http.Redirect(w, r, "/dashboard/settings?error="+url.QueryEscape("Current password incorrect"), http.StatusSeeOther)
		return
	}

	if newP != confP {
		http.Redirect(w, r, "/dashboard/settings?error="+url.QueryEscape("New passwords do not match"), http.StatusSeeOther)
		return
	}

	if len(newP) < 6 {
		http.Redirect(w, r, "/dashboard/settings?error="+url.QueryEscape("Password must be at least 6 characters"), http.StatusSeeOther)
		return
	}

	if err := h.deps.PasswordStore.SetPassword(newP); err != nil {
		http.Redirect(w, r, "/dashboard/settings?error="+url.QueryEscape("Failed to save password: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard/settings?flash="+url.QueryEscape("Dashboard password updated successfully!"), http.StatusSeeOther)
}

func (h *DashboardHandler) handleClientsView(w http.ResponseWriter, r *http.Request) {
	all := clients.All()
	cards := make([]ClientCard, len(all))
	for i, c := range all {
		cards[i] = buildClientCard(c)
	}

	data := ClientsPageData{
		Clients: cards,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = Layout("Clients", "clients", h.deps.PasswordStore.IsDefaultPassword(), viewClients(data)).Render(r.Context(), w)
}

func (h *DashboardHandler) handleClientDetailView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/dashboard/clients/")
		id = strings.Trim(id, "/")
	}

	c, ok := clients.Get(id)
	if !ok {
		http.Redirect(w, r, "/dashboard/clients", http.StatusSeeOther)
		return
	}

	st, _ := c.Detect()

	listen := h.deps.Service.Listen
	if listen == "" {
		listen = "127.0.0.1:8080"
	}

	defaultBaseURL := clients.DialectBaseURL(listen, c.Dialect())
	endpoints := []ClientEndpointOption{
		{URL: defaultBaseURL, IsDefault: true},
		{URL: clients.DialectBaseURL(listen, "anthropic")},
		{URL: clients.DialectBaseURL(listen, "openai")},
		{URL: clients.DialectBaseURL(listen, "gemini")},
	}
	seenEP := map[string]bool{}
	var uniqueEPs []ClientEndpointOption
	for _, ep := range endpoints {
		if !seenEP[ep.URL] {
			seenEP[ep.URL] = true
			if st.CurrentBaseURL == ep.URL {
				ep.IsCurrent = true
			}
			uniqueEPs = append(uniqueEPs, ep)
		}
	}

	routableModels := clients.DiscoverModelsForDialect(c.Dialect())
	if len(routableModels) == 0 {
		routableModels = []string{"gpt-4o", "gpt-4o-mini", "claude-3-5-sonnet", "claude-3-7-sonnet", "gemini-2.5-flash"}
	}

	card := buildClientCard(c)

	snippet := fmt.Sprintf("export ANTHROPIC_BASE_URL=%s\nexport ANTHROPIC_AUTH_TOKEN=tr_live_YOUR_KEY", defaultBaseURL)

	// Existing keys are offered in the "Provide Key" dropdown. Only keys with a
	// recoverable secret are listed, since selecting one embeds that secret into
	// the client config.
	existingKeys := []ExistingKeyOption{}
	legacyKeyCount := 0
	selectedKeyID := ""
	if h.deps.KeyWatcher != nil {
		if ks := h.deps.KeyWatcher.Get(); ks != nil {
			if st.RawKey != "" {
				if matchedKey, ok := ks.LookupByToken(st.RawKey); ok {
					selectedKeyID = matchedKey.ID
				}
			}
			for _, k := range ks.Keys() {
				if k.Disabled {
					continue
				}
				if k.Secret == "" {
					// Minted before secrets were persisted — cannot be embedded.
					legacyKeyCount++
					continue
				}
				masked := k.Prefix
				if masked == "" {
					masked = "tr_live_..."
				} else {
					masked = "tr_live_" + masked + "..."
				}
				name := k.Name
				if name == "" {
					name = k.ID
				}
				existingKeys = append(existingKeys, ExistingKeyOption{
					ID:    k.ID,
					Label: name + " (" + masked + ")",
				})
			}
			sort.Slice(existingKeys, func(i, j int) bool {
				return existingKeys[i].Label < existingKeys[j].Label
			})
		}
	}

	data := ClientDetailPageData{
		Client:              c,
		ClientID:            c.ID(),
		ClientName:          c.Name(),
		Dialect:             c.Dialect(),
		Status:              st,
		StatusBadgeClass:    card.StatusBadgeClass,
		StatusLabel:         card.StatusLabel,
		CurrentEndpoint:     st.CurrentBaseURL,
		DefaultEndpoint:     defaultBaseURL,
		Endpoints:           uniqueEPs,
		MaskedKey:           st.MaskedKey,
		RoutableModels:      routableModels,
		ProviderModelGroups: groupModelsByProvider(routableModels),
		SlotValues:          st.SlotValues,
		ModelSlots:          c.ModelSlots(),
		ExistingKeys:        existingKeys,
		SelectedKeyID:       selectedKeyID,
		LegacyKeyCount:      legacyKeyCount,
		ManualSnippet:       snippet,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = Layout(c.Name()+" - Clients", "clients", h.deps.PasswordStore.IsDefaultPassword(), viewClientDetail(data)).Render(r.Context(), w)
}

// resolveExistingKey looks up a key by id and returns its recoverable secret,
// so the dashboard's "Provide Key" dropdown can embed an existing key into a
// client config. Returns ok=false when the store is unavailable, the id is
// unknown, or the key has no persisted secret (e.g. created before recovery).
func (h *DashboardHandler) resolveExistingKey(keyID string) (string, bool) {
	if keyID == "" || h.deps.KeyWatcher == nil {
		return "", false
	}
	ks := h.deps.KeyWatcher.Get()
	if ks == nil {
		return "", false
	}
	k, ok := ks.Lookup(keyID)
	if !ok || k.Disabled || k.Secret == "" {
		return "", false
	}
	return k.Secret, true
}

func (h *DashboardHandler) handleClientPlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/dashboard/clients/")
		id = strings.TrimSuffix(id, "/plan")
		id = strings.Trim(id, "/")
	}

	c, ok := clients.Get(id)
	if !ok {
		http.Error(w, "unknown client", http.StatusNotFound)
		return
	}

	baseURL := r.FormValue("base_url")
	keyStrategyStr := r.FormValue("key_strategy")
	apiKey := r.FormValue("api_key")
	keyName := r.FormValue("key_name")

	// "Provide Key" via the existing-key dropdown: resolve the selected key's
	// recoverable secret so the installer can embed it as a reused key.
	existingKey := r.FormValue("existing_key")
	if apiKey == "" && existingKey != "" {
		if secret, ok := h.resolveExistingKey(existingKey); ok {
			apiKey = secret
		}
	}
	if apiKey == "" && (keyStrategyStr == "reuse" || keyStrategyStr == "") {
		st, _ := c.Detect()
		if st.RawKey != "" {
			apiKey = st.RawKey
		}
	}

	keyStrategy := clients.KeyStrategy(keyStrategyStr)
	if keyStrategy == "" {
		if apiKey != "" {
			keyStrategy = clients.KeyStrategyReuse
		} else {
			keyStrategy = clients.KeyStrategyMint
		}
	}

	slotValues := make(map[string]string)
	for _, slot := range c.ModelSlots() {
		val := r.FormValue("slot_" + slot.ID)
		if val != "" {
			slotValues[slot.ID] = val
		}
	}
	if slotValues["model"] == "" {
		if val := r.FormValue("slot_models"); val != "" {
			slotValues["model"] = val
		} else if val := r.FormValue("model"); val != "" {
			slotValues["model"] = val
		}
	}
	if slotValues["models"] == "" {
		if val := r.FormValue("slot_model"); val != "" {
			slotValues["models"] = val
		}
	}

	listen := h.deps.Service.Listen
	installer := clients.NewInstaller(listen, h.deps.Service.KeysPath)

	primaryModelPlan := slotValues["model"]
	if primaryModelPlan == "" {
		primaryModelPlan = slotValues["models"]
	}

	plan, err := installer.Plan(clients.InstallRequest{
		ClientID:    id,
		BaseURL:     baseURL,
		APIKey:      apiKey,
		KeyStrategy: keyStrategy,
		KeyName:     keyName,
		Model:       primaryModelPlan,
		ModelSlots:  slotValues,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(plan)
}

func (h *DashboardHandler) handleClientApply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/dashboard/clients/")
		id = strings.TrimSuffix(id, "/apply")
		id = strings.Trim(id, "/")
	}

	c, ok := clients.Get(id)
	if !ok {
		http.Redirect(w, r, "/dashboard/clients", http.StatusSeeOther)
		return
	}

	baseURL := r.FormValue("base_url")
	keyStrategyStr := r.FormValue("key_strategy")
	apiKey := r.FormValue("api_key")
	keyName := r.FormValue("key_name")

	// "Provide Key" via the existing-key dropdown: resolve the selected key's
	// recoverable secret so the installer can embed it as a reused key.
	existingKey := r.FormValue("existing_key")
	if apiKey == "" && existingKey != "" {
		if secret, ok := h.resolveExistingKey(existingKey); ok {
			apiKey = secret
		}
	}
	if apiKey == "" && (keyStrategyStr == "reuse" || keyStrategyStr == "") {
		st, _ := c.Detect()
		if st.RawKey != "" {
			apiKey = st.RawKey
		}
	}

	keyStrategy := clients.KeyStrategy(keyStrategyStr)
	if keyStrategy == "" {
		if apiKey != "" {
			keyStrategy = clients.KeyStrategyReuse
		} else {
			keyStrategy = clients.KeyStrategyMint
		}
	}

	slotValues := make(map[string]string)
	for _, slot := range c.ModelSlots() {
		val := r.FormValue("slot_" + slot.ID)
		if val != "" {
			slotValues[slot.ID] = val
		}
	}
	if slotValues["model"] == "" {
		if val := r.FormValue("slot_models"); val != "" {
			slotValues["model"] = val
		} else if val := r.FormValue("model"); val != "" {
			slotValues["model"] = val
		}
	}
	if slotValues["models"] == "" {
		if val := r.FormValue("slot_model"); val != "" {
			slotValues["models"] = val
		}
	}

	listen := h.deps.Service.Listen
	installer := clients.NewInstaller(listen, h.deps.Service.KeysPath)

	primaryModelApply := slotValues["model"]
	if primaryModelApply == "" {
		primaryModelApply = slotValues["models"]
	}

	plan, err := installer.Plan(clients.InstallRequest{
		ClientID:    id,
		BaseURL:     baseURL,
		APIKey:      apiKey,
		KeyStrategy: keyStrategy,
		KeyName:     keyName,
		Model:       primaryModelApply,
		ModelSlots:  slotValues,
	})
	if err != nil {
		http.Redirect(w, r, "/dashboard/clients/"+id+"?error="+url.QueryEscape("Failed to plan install: "+err.Error()), http.StatusSeeOther)
		return
	}

	res, err := installer.Apply(plan)
	if err != nil {
		http.Redirect(w, r, "/dashboard/clients/"+id+"?error="+url.QueryEscape("Failed to apply configuration: "+err.Error()), http.StatusSeeOther)
		return
	}

	if plan.KeyStrategy == clients.KeyStrategyMint && res.Key != "" {
		st, _ := c.Detect()
		revealData := KeyRevealPageData{
			ClientName: c.Name(),
			ClientID:   c.ID(),
			MintedKey:  res.Key,
			BaseURL:    plan.BaseURL,
			ConfigPath: st.ConfigPath,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = Layout("Key Minted - "+c.Name(), "clients", h.deps.PasswordStore.IsDefaultPassword(), viewKeyReveal(revealData)).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/dashboard/clients/"+id+"?flash="+url.QueryEscape("Successfully configured "+c.Name()+"!"), http.StatusSeeOther)
}

func (h *DashboardHandler) handleClientReset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/dashboard/clients/")
		id = strings.TrimSuffix(id, "/reset")
		id = strings.Trim(id, "/")
	}

	c, ok := clients.Get(id)
	if !ok {
		http.Redirect(w, r, "/dashboard/clients", http.StatusSeeOther)
		return
	}

	if err := c.Reset(); err != nil {
		http.Redirect(w, r, "/dashboard/clients/"+id+"?error="+url.QueryEscape("Failed to reset client config: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard/clients/"+id+"?flash="+url.QueryEscape("Successfully reset "+c.Name()+" configuration!"), http.StatusSeeOther)
}
