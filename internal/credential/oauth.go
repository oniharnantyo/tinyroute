package credential

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultLeadTime = 5 * time.Minute

// OAuthRefreshable implements Credential for refreshable OAuth tokens.
type OAuthRefreshable struct {
	mu                  sync.RWMutex
	provider            string
	refreshToken        string
	clientID            string
	clientSecret        string
	tokenEndpoint       string
	profile             RefreshProfile
	accessToken         string
	expiresAt           time.Time
	leadTime            time.Duration
	deviceID            string
	deviceHeaderProfile string
	refreshManager      *RefreshManager
	store               *Store
	httpClient          *http.Client
}

// OAuthRefreshableConfig holds initialization fields for OAuthRefreshable.
type OAuthRefreshableConfig struct {
	Provider            string
	RefreshToken        string
	ClientID            string
	ClientSecret        string
	TokenEndpoint       string
	Profile             RefreshProfile
	AccessToken         string
	ExpiresAt           time.Time
	LeadTime            time.Duration
	DeviceID            string
	DeviceHeaderProfile string
	RefreshManager      *RefreshManager
	Store               *Store
	HTTPClient          *http.Client
}

// NewOAuthRefreshable constructs an OAuthRefreshable credential.
func NewOAuthRefreshable(cfg OAuthRefreshableConfig) *OAuthRefreshable {
	leadTime := cfg.LeadTime
	if leadTime <= 0 {
		leadTime = defaultLeadTime
	}
	rm := cfg.RefreshManager
	if rm == nil {
		rm = DefaultRefreshManager
	}
	return &OAuthRefreshable{
		provider:            cfg.Provider,
		refreshToken:        cfg.RefreshToken,
		clientID:            cfg.ClientID,
		clientSecret:        cfg.ClientSecret,
		tokenEndpoint:       cfg.TokenEndpoint,
		profile:             cfg.Profile,
		accessToken:         cfg.AccessToken,
		expiresAt:           cfg.ExpiresAt,
		leadTime:            leadTime,
		deviceID:            cfg.DeviceID,
		deviceHeaderProfile: cfg.DeviceHeaderProfile,
		refreshManager:      rm,
		store:               cfg.Store,
		httpClient:          cfg.HTTPClient,
	}
}

// Token returns a cached access token if valid, or proactive refresh if within lead time / expired.
func (o *OAuthRefreshable) Token(ctx context.Context) (TokenResult, error) {
	o.mu.RLock()
	at := o.accessToken
	exp := o.expiresAt
	lead := o.leadTime
	o.mu.RUnlock()

	if (o.provider == "cline" || o.provider == "clinepass") && at != "" && !strings.HasPrefix(at, "workos:") {
		at = "workos:" + at
	}

	if at != "" && time.Until(exp) > lead {
		return TokenResult{
			Value: at,
			Kind:  KindOAuthBearer,
		}, nil
	}

	return o.Refresh(ctx)
}

// Refresh performs a token refresh and updates cached state & custodian store.
func (o *OAuthRefreshable) Refresh(ctx context.Context) (TokenResult, error) {
	o.mu.RLock()
	req := RefreshRequest{
		Provider:            o.provider,
		RefreshToken:        o.refreshToken,
		ClientID:            o.clientID,
		ClientSecret:        o.clientSecret,
		TokenEndpoint:       o.tokenEndpoint,
		Profile:             o.profile,
		DeviceID:            o.deviceID,
		DeviceHeaderProfile: o.deviceHeaderProfile,
		HTTPClient:          o.httpClient,
	}
	o.mu.RUnlock()

	res, exp, newRT, err := o.refreshManager.RefreshToken(ctx, req)
	if err != nil {
		return TokenResult{}, fmt.Errorf("oauth refresh for provider %q failed: %w", o.provider, err)
	}

	o.mu.Lock()
	o.accessToken = res.Value
	o.expiresAt = exp
	if newRT != "" {
		o.refreshToken = newRT
	}
	record := OAuthRecord{
		Provider:            o.provider,
		RefreshToken:        o.refreshToken,
		AccessToken:         o.accessToken,
		ExpiresAt:           o.expiresAt,
		ClientID:            o.clientID,
		ClientSecret:        o.clientSecret,
		TokenEndpoint:       o.tokenEndpoint,
		Profile:             o.profile,
		DeviceID:            o.deviceID,
		DeviceHeaderProfile: o.deviceHeaderProfile,
		UpdatedAt:           time.Now().UTC(),
	}
	store := o.store
	o.mu.Unlock()

	if store != nil {
		_ = store.Save(record)
	}

	return res, nil
}

// MaskedStatus returns connectivity status without revealing plaintext credentials.
func (o *OAuthRefreshable) MaskedStatus() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.refreshToken == "" && o.accessToken == "" {
		return "not connected"
	}
	if !o.expiresAt.IsZero() {
		return fmt.Sprintf("connected (expires %s)", o.expiresAt.Format(time.RFC3339))
	}
	return "connected"
}
