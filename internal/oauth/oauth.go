package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/preset"
)

// GeneratePKCE creates a random code verifier and S256 code challenge.
func GeneratePKCE() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate random verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)

	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// GenerateState creates a random hex state token for OAuth CSRF protection.
func GenerateState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// GenerateDeviceID generates a UUID v4 formatted device ID string.
func GenerateDeviceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// PKCESession contains state and URLs required to drive a PKCE flow.
type PKCESession struct {
	State        string `json:"state"`
	Verifier     string `json:"verifier"`
	Challenge    string `json:"challenge"`
	RedirectURI  string `json:"redirect_uri"`
	AuthorizeURL string `json:"authorize_url"`
}

// BuildAuthorizeURL builds the OAuth authorization URL for a given preset, state, challenge, and redirectURI.
func BuildAuthorizeURL(p *preset.Preset, redirectURI, state, challenge string) (string, error) {
	u, err := url.Parse(p.AuthorizeEndpoint)
	if err != nil {
		return "", fmt.Errorf("parse authorize endpoint: %w", err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	if p.ClientID != "" {
		q.Set("client_id", p.ClientID)
	}
	q.Set("redirect_uri", redirectURI)
	if p.Name == "cline" || p.Name == "clinepass" {
		callbackURL := redirectURI
		if strings.Contains(callbackURL, "?") {
			callbackURL += "&state=" + url.QueryEscape(state)
		} else {
			callbackURL += "?state=" + url.QueryEscape(state)
		}
		q.Set("callback_url", callbackURL)
	}
	if len(p.Scopes) > 0 {
		q.Set("scope", strings.Join(p.Scopes, " "))
	}
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	for k, v := range p.ExtraParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// StartPKCE initiates a PKCE session for a preset and redirect URI.
func StartPKCE(p *preset.Preset, redirectURI string) (PKCESession, error) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return PKCESession{}, err
	}
	state := GenerateState()
	authURL, err := BuildAuthorizeURL(p, redirectURI, state, challenge)
	if err != nil {
		return PKCESession{}, err
	}

	return PKCESession{
		State:        state,
		Verifier:     verifier,
		Challenge:    challenge,
		RedirectURI:  redirectURI,
		AuthorizeURL: authURL,
	}, nil
}

// ExchangePKCE exchanges an authorization code for an OAuthRecord.
func ExchangePKCE(ctx context.Context, p *preset.Preset, client *http.Client, code, verifier, redirectURI string) (*credential.OAuthRecord, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var req *http.Request
	if p.Name == "cline" || p.Name == "clinepass" {
		bodyMap := map[string]interface{}{
			"grant_type":   "authorization_code",
			"code":         code,
			"client_type":  "extension",
			"redirect_uri": redirectURI,
			"provider":     p.Name,
		}
		bodyBytes, err := json.Marshal(bodyMap)
		if err != nil {
			return nil, fmt.Errorf("marshal token request: %w", err)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, p.TokenEndpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("create token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		form := url.Values{}
		form.Set("grant_type", "authorization_code")
		form.Set("code", code)
		form.Set("redirect_uri", redirectURI)
		form.Set("client_id", p.ClientID)
		if p.ClientSecret != "" {
			form.Set("client_secret", p.ClientSecret)
		}
		form.Set("code_verifier", verifier)
		for k, v := range p.ExtraParams {
			form.Set(k, v)
		}

		var err error
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, p.TokenEndpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, fmt.Errorf("create token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	req.Header.Set("Accept", "application/json")
	if p.RefreshProfile.UseBasicAuth {
		req.SetBasicAuth(p.ClientID, p.ClientSecret)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token request failed (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var res struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
		Data         struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			IDToken      string `json:"idToken"`
			TokenType    string `json:"tokenType"`
			ExpiresAt    string `json:"expiresAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	if res.Error != "" {
		return nil, fmt.Errorf("token exchange error: %s (%s)", res.Error, res.ErrorDesc)
	}

	accessToken := res.AccessToken
	if accessToken == "" {
		accessToken = res.Data.AccessToken
	}
	if (p.Name == "cline" || p.Name == "clinepass") && accessToken != "" && !strings.HasPrefix(accessToken, "workos:") {
		accessToken = "workos:" + accessToken
	}
	refreshToken := res.RefreshToken
	if refreshToken == "" {
		refreshToken = res.Data.RefreshToken
	}
	idToken := res.IDToken
	if idToken == "" {
		idToken = res.Data.IDToken
	}

	var exp time.Time
	if res.ExpiresIn > 0 {
		exp = time.Now().Add(time.Duration(res.ExpiresIn) * time.Second)
	} else if res.Data.ExpiresAt != "" {
		if parsedExp, err := time.Parse(time.RFC3339, res.Data.ExpiresAt); err == nil {
			exp = parsedExp
		}
	}

	identityHint := ExtractIdentityHint(idToken, accessToken)

	rec := &credential.OAuthRecord{
		Provider:      p.Name,
		RefreshToken:  refreshToken,
		AccessToken:   accessToken,
		ExpiresAt:     exp,
		ClientID:      p.ClientID,
		ClientSecret:  p.ClientSecret,
		TokenEndpoint: p.TokenEndpoint,
		Profile:       p.RefreshProfile,
		Scopes:        p.Scopes,
		IdentityHint:  identityHint,
	}
	return rec, nil
}

// DeviceSession contains the details returned by a device authorization request.
type DeviceSession struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
	DeviceID        string `json:"device_id,omitempty"`
}

type deviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	Code                    string `json:"code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURL         string `json:"verificationUrl"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	ExpiresInCamel          int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
	Error                   string `json:"error"`
	ErrorDescription        string `json:"error_description"`
}

// StartDeviceFlow sends a device authorization request to the provider.
func StartDeviceFlow(ctx context.Context, p *preset.Preset, client *http.Client) (DeviceSession, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var deviceID string
	if p.DeviceHeaderProfile != "" {
		deviceID = GenerateDeviceID()
	}

	form := url.Values{}
	if p.ClientID != "" {
		form.Set("client_id", p.ClientID)
	}
	if len(p.Scopes) > 0 {
		form.Set("scope", strings.Join(p.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.DeviceEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return DeviceSession{}, fmt.Errorf("create device request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if p.DeviceHeaderProfile != "" {
		credential.ApplyDeviceHeaders(req, p.DeviceHeaderProfile, deviceID)
	}

	resp, err := client.Do(req)
	if err != nil {
		return DeviceSession{}, fmt.Errorf("device request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DeviceSession{}, fmt.Errorf("device request failed (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var dResp deviceAuthResponse
	if err := json.Unmarshal(bodyBytes, &dResp); err != nil {
		return DeviceSession{}, fmt.Errorf("decode device response: %w", err)
	}

	if dResp.Error != "" {
		return DeviceSession{}, fmt.Errorf("device authorization failed: %s (%s)", dResp.Error, dResp.ErrorDescription)
	}

	if dResp.DeviceCode == "" && dResp.Code != "" {
		dResp.DeviceCode = dResp.Code
	}
	if dResp.UserCode == "" && dResp.Code != "" {
		dResp.UserCode = dResp.Code
	}
	if dResp.VerificationURI == "" && dResp.VerificationURL != "" {
		dResp.VerificationURI = dResp.VerificationURL
	}
	if dResp.VerificationURI == "" {
		dResp.VerificationURI = dResp.VerificationURIComplete
	}
	if dResp.ExpiresIn <= 0 && dResp.ExpiresInCamel > 0 {
		dResp.ExpiresIn = dResp.ExpiresInCamel
	}

	interval := dResp.Interval
	if interval <= 0 {
		interval = 5
	}
	expiresIn := dResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 600
	}

	return DeviceSession{
		DeviceCode:      dResp.DeviceCode,
		UserCode:        dResp.UserCode,
		VerificationURI: dResp.VerificationURI,
		Interval:        interval,
		ExpiresIn:       expiresIn,
		DeviceID:        deviceID,
	}, nil
}

// PollDeviceFlow performs a single token poll request for a device flow.
// Returns (record, pending, error). If pending is true, caller should sleep interval and poll again.
func PollDeviceFlow(ctx context.Context, p *preset.Preset, client *http.Client, deviceCode, deviceID string) (*credential.OAuthRecord, bool, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var pollReq *http.Request
	var err error

	if strings.Contains(p.TokenEndpoint, "{code}") {
		pollURL := strings.ReplaceAll(p.TokenEndpoint, "{code}", deviceCode)
		pollReq, err = http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
		if err != nil {
			return nil, false, fmt.Errorf("create token poll request: %w", err)
		}
	} else {
		pollForm := url.Values{}
		pollForm.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		pollForm.Set("device_code", deviceCode)
		pollForm.Set("code", deviceCode)
		if p.ClientID != "" {
			pollForm.Set("client_id", p.ClientID)
		}

		pollReq, err = http.NewRequestWithContext(ctx, http.MethodPost, p.TokenEndpoint, strings.NewReader(pollForm.Encode()))
		if err != nil {
			return nil, false, fmt.Errorf("create token poll request: %w", err)
		}
		pollReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	pollReq.Header.Set("Accept", "application/json")
	if p.DeviceHeaderProfile != "" && deviceID != "" {
		credential.ApplyDeviceHeaders(pollReq, p.DeviceHeaderProfile, deviceID)
	}

	pollResp, err := client.Do(pollReq)
	if err != nil {
		return nil, true, nil // Network error, retry
	}
	defer pollResp.Body.Close()

	var tokenRes struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		Token        string `json:"token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Status       string `json:"status"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.NewDecoder(pollResp.Body).Decode(&tokenRes); err != nil {
		return nil, true, nil // Decode error while pending, retry
	}

	if tokenRes.Token != "" && tokenRes.AccessToken == "" {
		tokenRes.AccessToken = tokenRes.Token
		tokenRes.RefreshToken = tokenRes.Token
	}

	if tokenRes.AccessToken != "" || tokenRes.RefreshToken != "" {
		var exp time.Time
		if tokenRes.ExpiresIn > 0 {
			exp = time.Now().Add(time.Duration(tokenRes.ExpiresIn) * time.Second)
		}
		identityHint := ExtractIdentityHint(tokenRes.IDToken, tokenRes.AccessToken)
		rec := &credential.OAuthRecord{
			Provider:            p.Name,
			RefreshToken:        tokenRes.RefreshToken,
			AccessToken:         tokenRes.AccessToken,
			ExpiresAt:           exp,
			ClientID:            p.ClientID,
			ClientSecret:        p.ClientSecret,
			TokenEndpoint:       p.TokenEndpoint,
			Profile:             p.RefreshProfile,
			Scopes:              p.Scopes,
			DeviceID:            deviceID,
			DeviceHeaderProfile: p.DeviceHeaderProfile,
			IdentityHint:        identityHint,
		}
		return rec, false, nil
	}

	switch tokenRes.Status {
	case "pending":
		return nil, true, nil
	case "expired":
		return nil, false, errors.New("device code expired")
	}

	switch tokenRes.Error {
	case "authorization_pending", "slow_down", "pending":
		return nil, true, nil
	case "access_denied":
		return nil, false, errors.New("access denied by user")
	case "expired_token", "expired":
		return nil, false, errors.New("device code expired")
	default:
		if tokenRes.Error != "" {
			return nil, false, fmt.Errorf("oauth error: %s (%s)", tokenRes.Error, tokenRes.ErrorDesc)
		}
	}

	return nil, true, nil
}
