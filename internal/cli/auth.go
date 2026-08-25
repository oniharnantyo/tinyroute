package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/cli/interactive"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/oauth"
	"github.com/oniharnantyo/tinyroute/internal/preset"
	"github.com/urfave/cli/v3"
)

type importOptions struct {
	account       string
	refreshToken  string
	accessToken   string
	clientID      string
	tokenEndpoint string
	filePath      string
}

func cmdAuth() *cli.Command {
	return &cli.Command{
		Name:  "auth",
		Usage: "Manage provider credentials and OAuth authentication",
		Commands: []*cli.Command{
			{
				Name:      "login",
				Usage:     "Log in to an OAuth provider (device_code or pkce flow)",
				ArgsUsage: "[provider]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "account",
						Aliases: []string{"a"},
						Usage:   "account name to update under provider",
					},
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive prompt if provider is omitted",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive prompts",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					isInteractive := cmd.Bool("interactive") && !cmd.Bool("no-interactive")
					return cmdAuthLoginWithAccount(ctx, cmd.Args().Slice(), cmd.String("account"), isInteractive)
				},
			},
			{
				Name:      "import",
				Usage:     "Import OAuth tokens or credential file into custodian store",
				ArgsUsage: "[provider]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "account",
						Aliases: []string{"a"},
						Usage:   "account name to update under provider",
					},
					&cli.StringFlag{
						Name:    "refresh-token",
						Aliases: []string{"r"},
						Usage:   "refresh token value",
					},
					&cli.StringFlag{
						Name:    "access-token",
						Aliases: []string{"a-tok"},
						Usage:   "access token value",
					},
					&cli.StringFlag{
						Name:    "client-id",
						Aliases: []string{"c"},
						Usage:   "OAuth client ID",
					},
					&cli.StringFlag{
						Name:    "token-endpoint",
						Aliases: []string{"e"},
						Usage:   "OAuth token endpoint URL",
					},
					&cli.StringFlag{
						Name:    "file",
						Aliases: []string{"f"},
						Usage:   "path to native credential JSON file",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					opts := importOptions{
						account:       cmd.String("account"),
						refreshToken:  cmd.String("refresh-token"),
						accessToken:   cmd.String("access-token"),
						clientID:      cmd.String("client-id"),
						tokenEndpoint: cmd.String("token-endpoint"),
						filePath:      cmd.String("file"),
					}
					return cmdAuthImport(cmd.Args().Slice(), opts)
				},
			},
			{
				Name:  "status",
				Usage: "Show status for OAuth providers",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return cmdAuthStatus(cmd.Args().Slice())
				},
			},
			{
				Name:      "set",
				Usage:     "Set API key provider credential",
				ArgsUsage: "<provider>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "prompt for masked credential input",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable masked credential prompt",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "read credential from stdin without prompt",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() < 1 {
						return fmt.Errorf("usage: tinyroute auth set <provider>")
					}
					args := []string{cmd.Args().First()}
					if cmd.IsSet("interactive") && !cmd.Bool("interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("no-interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("force") {
						args = append(args, "--force")
					}
					return cmdAuthSet(args)
				},
			},
			{
				Name:  "list",
				Usage: "List providers and credential status",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return cmdAuthList([]string{})
				},
			},
		},
	}
}

func cmdAuthLogin(ctx context.Context, args []string, isInteractive bool) error {
	return cmdAuthLoginWithAccount(ctx, args, "", isInteractive)
}

func cmdAuthLoginWithAccount(ctx context.Context, args []string, accountName string, isInteractive bool) error {
	signalCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var providerName string
	if len(args) > 0 {
		providerName = args[0]
	}

	if providerName == "" {
		if !isInteractive || !interactive.CanPrompt() {
			return errors.New("provider argument required in non-interactive mode")
		}

		var oauthPresets []string
		for _, p := range preset.All() {
			if p.OAuthCapable {
				oauthPresets = append(oauthPresets, p.Name)
			}
		}
		sort.Strings(oauthPresets)
		if len(oauthPresets) == 0 {
			return errors.New("no OAuth-capable presets available")
		}
		if len(oauthPresets) == 1 {
			providerName = oauthPresets[0]
			fmt.Printf("Auto-selected provider %q\n", providerName)
		} else {
			selected, err := interactive.Select("Select OAuth provider:", oauthPresets)
			if err != nil {
				return fmt.Errorf("select provider: %w", err)
			}
			providerName = selected
		}
	}

	p := preset.Get(providerName)
	if p == nil || !p.OAuthCapable {
		return fmt.Errorf("provider %q does not support OAuth authentication", providerName)
	}

	if p.RiskNotice != "" {
		fmt.Printf("Notice: %s\n", p.RiskNotice)
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	store, err := credential.NewStore(svc.CredentialsPath)
	if err != nil {
		return fmt.Errorf("init custodian store: %w", err)
	}

	if p.FlowType == "pkce" && p.ClientID == "" {
		if !interactive.CanPrompt() {
			return fmt.Errorf("provider %q requires an OAuth client_id; run interactively in a TTY or import credentials with --client-id", p.Name)
		}
		cid, err := interactive.Input(fmt.Sprintf("Enter OAuth client_id for provider %q: ", p.Name), "", nil)
		if err != nil {
			return fmt.Errorf("read client_id: %w", err)
		}
		cid = strings.TrimSpace(cid)
		if cid == "" {
			return fmt.Errorf("client_id for provider %q cannot be empty", p.Name)
		}
		pCopy := *p
		pCopy.ClientID = cid
		if p.RefreshProfile.IncludeClientSecret {
			cs, err := interactive.Password(fmt.Sprintf("Enter OAuth client_secret for provider %q: ", p.Name))
			if err != nil {
				return fmt.Errorf("read client_secret: %w", err)
			}
			pCopy.ClientSecret = strings.TrimSpace(cs)
		}
		p = &pCopy
	}

	if accountName != "" {
		if err := credential.ValidateAccountName(accountName); err != nil {
			return fmt.Errorf("invalid account name: %w", err)
		}
	}

	var topo config.Topology
	if data, err := os.ReadFile(svc.ConfigPath); err == nil {
		topo, _ = config.ParseRawTopology(data)
	}
	existing := getExistingAccounts(providerName, store, topo)

	client := &http.Client{Timeout: 30 * time.Second}

	switch p.FlowType {
	case "device_code":
		return runDeviceCodeFlow(signalCtx, p, client, store, accountName, existing, os.Stdout)
	case "pkce":
		return runPKCEFlow(signalCtx, p, client, store, accountName, existing, os.Stdout)
	case "qoder":
		return runQoderFlow(signalCtx, p, client, store, accountName, existing, os.Stdout)
	case "trae":
		return runTraeFlow(signalCtx, p, client, store, accountName, existing, os.Stdout)
	default:
		return fmt.Errorf("unsupported OAuth flow type %q for provider %q", p.FlowType, providerName)
	}
}

func getExistingAccounts(providerName string, store *credential.Store, topo config.Topology) []string {
	seen := make(map[string]bool)
	if store != nil {
		for _, rec := range store.List() {
			if strings.EqualFold(rec.Provider, providerName) {
				acc := rec.Account
				if acc == "" {
					acc = "default"
				}
				seen[acc] = true
			}
		}
	}
	if p, ok := topo.Providers[providerName]; ok {
		for _, acc := range p.Accounts {
			if acc.Name != "" {
				seen[acc.Name] = true
			}
		}
	} else if p, ok := topo.Providers[strings.ToLower(providerName)]; ok {
		for _, acc := range p.Accounts {
			if acc.Name != "" {
				seen[acc.Name] = true
			}
		}
	}
	var existing []string
	for acc := range seen {
		existing = append(existing, acc)
	}
	sort.Strings(existing)
	return existing
}

func runQoderFlow(ctx context.Context, p *preset.Preset, client *http.Client, store *credential.Store, accountName string, existing []string, out io.Writer) error {
	signalCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	nonce := generateDeviceID()

	fmt.Fprintf(out, "Open the following URL in your browser to authorize:\n\n  https://qoder.com/device/selectAccounts\n\nPolling for completion...\n")

	interval := 5
	deadline := time.Now().Add(600 * time.Second)

	for {
		select {
		case <-signalCtx.Done():
			return signalCtx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return errors.New("qoder device authentication timed out")
		}

		time.Sleep(time.Duration(interval) * time.Second)

		bodyMap := map[string]string{
			"nonce":      nonce,
			"machine_id": nonce,
		}
		bodyBytes, _ := json.Marshal(bodyMap)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.DeviceEndpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("create qoder poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		var res struct {
			DeviceToken string `json:"device_token"`
			Token       string `json:"token"`
			AccessToken string `json:"access_token"`
			Error       string `json:"error"`
			Status      string `json:"status"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()

		token := res.DeviceToken
		if token == "" {
			token = res.Token
		}
		if token == "" {
			token = res.AccessToken
		}

		if token != "" {
			hint := oauth.ExtractIdentityHint("", token)
			resolvedAcc, err := credential.ResolveAccount(p.Name, accountName, hint, existing)
			if err != nil {
				return fmt.Errorf("resolve account: %w", err)
			}
			rec := credential.OAuthRecord{
				Provider:      p.Name,
				Account:       resolvedAcc,
				RefreshToken:  token,
				AccessToken:   token,
				ClientID:      p.ClientID,
				TokenEndpoint: p.TokenEndpoint,
				Profile:       p.RefreshProfile,
				IdentityHint:  hint,
			}
			if err := store.Save(rec); err != nil {
				return fmt.Errorf("save credential: %w", err)
			}
			fmt.Fprintf(out, "Successfully authenticated provider %q (account %q)!\n", p.Name, resolvedAcc)
			return nil
		}
	}
}

func runTraeFlow(ctx context.Context, p *preset.Preset, client *http.Client, store *credential.Store, accountName string, existing []string, out io.Writer) error {
	signalCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.AuthorizeEndpoint, nil)
	if err != nil {
		return fmt.Errorf("create trae guidance request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("trae guidance request failed: %w", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var guidanceRes struct {
		Data struct {
			RedirectURL string `json:"redirect_url"`
			LoginURL    string `json:"login_url"`
			AuthURL     string `json:"auth_url"`
		} `json:"data"`
		RedirectURL string `json:"redirect_url"`
	}
	_ = json.Unmarshal(bodyBytes, &guidanceRes)

	authURL := guidanceRes.Data.RedirectURL
	if authURL == "" {
		authURL = guidanceRes.Data.LoginURL
	}
	if authURL == "" {
		authURL = guidanceRes.Data.AuthURL
	}
	if authURL == "" {
		authURL = guidanceRes.RedirectURL
	}
	if authURL == "" {
		authURL = p.AuthorizeEndpoint
	}

	fmt.Fprintf(out, "Open the following URL in your browser to authorize:\n\n  %s\n\nExchange token sequence completing...\n", authURL)

	exchangeForm := url.Values{}
	exchangeForm.Set("client_id", p.ClientID)
	tokenReq, err := http.NewRequestWithContext(signalCtx, http.MethodPost, p.TokenEndpoint, strings.NewReader(exchangeForm.Encode()))
	if err != nil {
		return fmt.Errorf("create trae exchange request: %w", err)
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return fmt.Errorf("trae exchange request failed: %w", err)
	}
	defer tokenResp.Body.Close()

	var tokenRes struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
	}
	_ = json.NewDecoder(tokenResp.Body).Decode(&tokenRes)

	if tokenRes.Error != "" {
		return fmt.Errorf("trae exchange failed: %s", tokenRes.Error)
	}

	var exp time.Time
	if tokenRes.ExpiresIn > 0 {
		exp = time.Now().Add(time.Duration(tokenRes.ExpiresIn) * time.Second)
	}

	hint := oauth.ExtractIdentityHint("", tokenRes.AccessToken)
	resolvedAcc, err := credential.ResolveAccount(p.Name, accountName, hint, existing)
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	rec := credential.OAuthRecord{
		Provider:      p.Name,
		Account:       resolvedAcc,
		RefreshToken:  tokenRes.RefreshToken,
		AccessToken:   tokenRes.AccessToken,
		ExpiresAt:     exp,
		ClientID:      p.ClientID,
		TokenEndpoint: p.TokenEndpoint,
		Profile:       p.RefreshProfile,
		IdentityHint:  hint,
	}
	if err := store.Save(rec); err != nil {
		return fmt.Errorf("save credential: %w", err)
	}
	fmt.Fprintf(out, "Successfully authenticated provider %q (account %q)!\n", p.Name, resolvedAcc)
	return nil
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

func generateDeviceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func runDeviceCodeFlow(ctx context.Context, p *preset.Preset, client *http.Client, store *credential.Store, accountName string, existing []string, out io.Writer) error {
	signalCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var deviceID string
	if p.DeviceHeaderProfile != "" {
		deviceID = generateDeviceID()
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
		return fmt.Errorf("create device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if p.DeviceHeaderProfile != "" {
		credential.ApplyDeviceHeaders(req, p.DeviceHeaderProfile, deviceID)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("device authorization request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("device authorization request failed (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var deviceResp deviceAuthResponse
	if err := json.Unmarshal(bodyBytes, &deviceResp); err != nil {
		return fmt.Errorf("decode device authorization response: %w", err)
	}

	if deviceResp.Error != "" {
		return fmt.Errorf("device authorization failed: %s (%s)", deviceResp.Error, deviceResp.ErrorDescription)
	}

	if deviceResp.DeviceCode == "" && deviceResp.Code != "" {
		deviceResp.DeviceCode = deviceResp.Code
	}
	if deviceResp.UserCode == "" && deviceResp.Code != "" {
		deviceResp.UserCode = deviceResp.Code
	}
	if deviceResp.VerificationURI == "" && deviceResp.VerificationURL != "" {
		deviceResp.VerificationURI = deviceResp.VerificationURL
	}
	if deviceResp.ExpiresIn <= 0 && deviceResp.ExpiresInCamel > 0 {
		deviceResp.ExpiresIn = deviceResp.ExpiresInCamel
	}

	if deviceResp.DeviceCode == "" || deviceResp.UserCode == "" {
		return fmt.Errorf("invalid device authorization response: missing device_code or user_code (response: %s)", string(bodyBytes))
	}

	uri := deviceResp.VerificationURI
	if uri == "" {
		uri = deviceResp.VerificationURIComplete
	}

	fmt.Fprintf(out, "User Code:        %s\n", deviceResp.UserCode)
	fmt.Fprintf(out, "Verification URL: %s\n", uri)
	fmt.Fprintf(out, "Polling for completion...\n")

	interval := deviceResp.Interval
	if interval <= 0 {
		interval = 5
	}
	expiresIn := deviceResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 600
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	for {
		select {
		case <-signalCtx.Done():
			return signalCtx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return errors.New("device code expired before authentication completed")
		}

		time.Sleep(time.Duration(interval) * time.Second)

		var pollReq *http.Request
		if strings.Contains(p.TokenEndpoint, "{code}") {
			pollURL := strings.ReplaceAll(p.TokenEndpoint, "{code}", deviceResp.DeviceCode)
			pollReq, err = http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
			if err != nil {
				return fmt.Errorf("create token poll request: %w", err)
			}
		} else {
			pollForm := url.Values{}
			pollForm.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
			pollForm.Set("device_code", deviceResp.DeviceCode)
			pollForm.Set("code", deviceResp.DeviceCode)
			if p.ClientID != "" {
				pollForm.Set("client_id", p.ClientID)
			}

			pollReq, err = http.NewRequestWithContext(ctx, http.MethodPost, p.TokenEndpoint, strings.NewReader(pollForm.Encode()))
			if err != nil {
				return fmt.Errorf("create token poll request: %w", err)
			}
			pollReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		pollReq.Header.Set("Accept", "application/json")
		if p.DeviceHeaderProfile != "" {
			credential.ApplyDeviceHeaders(pollReq, p.DeviceHeaderProfile, deviceID)
		}

		pollResp, err := client.Do(pollReq)
		if err != nil {
			continue
		}

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
		decodeErr := json.NewDecoder(pollResp.Body).Decode(&tokenRes)
		pollResp.Body.Close()
		if decodeErr != nil {
			continue
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
			identityHint := oauth.ExtractIdentityHint(tokenRes.IDToken, tokenRes.AccessToken)
			resolvedAcc, err := credential.ResolveAccount(p.Name, accountName, identityHint, existing)
			if err != nil {
				return fmt.Errorf("resolve account: %w", err)
			}
			rec := credential.OAuthRecord{
				Provider:            p.Name,
				Account:             resolvedAcc,
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
			if err := store.Save(rec); err != nil {
				return fmt.Errorf("save credential: %w", err)
			}
			fmt.Fprintf(out, "Successfully authenticated provider %q (account %q)!\n", p.Name, resolvedAcc)
			return nil
		}

		switch tokenRes.Status {
		case "pending":
			// Continue polling
		case "expired":
			return errors.New("device code expired")
		}

		switch tokenRes.Error {
		case "authorization_pending":
			// Continue polling
		case "slow_down":
			interval += 5
		case "access_denied":
			return errors.New("access denied by user")
		case "expired_token":
			return errors.New("device code expired")
		default:
			if tokenRes.Error != "" {
				return fmt.Errorf("oauth error: %s (%s)", tokenRes.Error, tokenRes.ErrorDesc)
			}
		}
	}
}

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

func callbackPath(p *preset.Preset) string {
	if p.CallbackPath != "" {
		return p.CallbackPath
	}
	return "/callback"
}

func callbackHost(p *preset.Preset) string {
	if p.CallbackHost != "" {
		return p.CallbackHost
	}
	return "127.0.0.1"
}

func callbackURI(p *preset.Preset, port int) string {
	return fmt.Sprintf("http://%s:%d%s", callbackHost(p), port, callbackPath(p))
}

func buildAuthorizeURL(p *preset.Preset, port int, state, challenge string) (string, error) {
	u, err := url.Parse(p.AuthorizeEndpoint)
	if err != nil {
		return "", fmt.Errorf("parse authorize endpoint: %w", err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", p.ClientID)
	rURI := callbackURI(p, port)
	q.Set("redirect_uri", rURI)
	if p.Name == "cline" || p.Name == "clinepass" {
		callbackURL := rURI
		if strings.Contains(callbackURL, "?") {
			callbackURL += "&state=" + url.QueryEscape(state)
		} else {
			callbackURL += "?state=" + url.QueryEscape(state)
		}
		q.Set("callback_url", callbackURL)
	}
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	if len(p.Scopes) > 0 {
		q.Set("scope", strings.Join(p.Scopes, " "))
	}
	for k, v := range p.ExtraParams {
		q.Set(k, v)
	}
	// Per decision D3, percent-encode spaces as %20 rather than '+' for strict OAuth providers (e.g. OpenAI/Hydra).
	u.RawQuery = strings.ReplaceAll(q.Encode(), "+", "%20")
	return u.String(), nil
}

func runPKCEFlow(ctx context.Context, p *preset.Preset, client *http.Client, store *credential.Store, accountName string, existing []string, out io.Writer) error {
	signalCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:1455")
	if err != nil {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("start local loopback server: %w", err)
		}
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := callbackURI(p, port)
	cPath := callbackPath(p)

	stateBytes := make([]byte, 16)
	_, _ = rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	authURL, err := buildAuthorizeURL(p, port, state, challenge)
	if err != nil {
		return err
	}

	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != cPath {
				http.NotFound(w, r)
				return
			}
			if r.URL.Query().Get("state") != state {
				http.Error(w, "State mismatch", http.StatusBadRequest)
				errChan <- errors.New("state mismatch in OAuth callback")
				return
			}
			if errParam := r.URL.Query().Get("error"); errParam != "" {
				desc := r.URL.Query().Get("error_description")
				http.Error(w, "Authorization failed: "+errParam, http.StatusBadRequest)
				errChan <- fmt.Errorf("oauth authorization failed: %s (%s)", errParam, desc)
				return
			}
			code := r.URL.Query().Get("code")
			if code == "" {
				http.Error(w, "Missing code", http.StatusBadRequest)
				errChan <- errors.New("missing code in OAuth callback")
				return
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintln(w, "<html><body><h2>Authentication Complete</h2><p>You can close this tab and return to the terminal.</p></body></html>")
			codeChan <- code
		}),
	}

	go func() {
		_ = srv.Serve(listener)
	}()
	defer srv.Shutdown(context.Background())

	fmt.Fprintf(out, "Open the following URL in your browser to authorize:\n\n  %s\n\nWaiting for callback on %s...\n", authURL, redirectURI)

	var authCode string
	select {
	case <-signalCtx.Done():
		return signalCtx.Err()
	case err := <-errChan:
		return err
	case code := <-codeChan:
		authCode = code
	}

	rec, err := oauth.ExchangePKCE(ctx, p, client, authCode, verifier, redirectURI)
	if err != nil {
		return err
	}

	resolvedAcc, err := credential.ResolveAccount(p.Name, accountName, rec.IdentityHint, existing)
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}
	rec.Account = resolvedAcc

	if err := store.Save(*rec); err != nil {
		return fmt.Errorf("save credential: %w", err)
	}

	fmt.Fprintf(out, "Successfully authenticated provider %q (account %q)!\n", p.Name, resolvedAcc)
	return nil
}

func cmdAuthImport(args []string, opts importOptions) error {
	var providerName string
	if len(args) > 0 {
		providerName = args[0]
	}

	if providerName == "" {
		return errors.New("usage: tinyroute auth import <provider> [options]")
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	store, err := credential.NewStore(svc.CredentialsPath)
	if err != nil {
		return fmt.Errorf("init custodian store: %w", err)
	}

	rec := credential.OAuthRecord{
		Provider: providerName,
		Account:  opts.account,
	}

	if opts.filePath != "" {
		data, err := os.ReadFile(opts.filePath)
		if err != nil {
			return fmt.Errorf("read credential file %s: %w", opts.filePath, err)
		}
		var imported struct {
			RefreshToken  string    `json:"refresh_token"`
			AccessToken   string    `json:"access_token"`
			ClientID      string    `json:"client_id"`
			TokenEndpoint string    `json:"token_endpoint"`
			ExpiresAt     time.Time `json:"expires_at"`
		}
		if err := json.Unmarshal(data, &imported); err == nil && imported.RefreshToken != "" {
			rec.RefreshToken = imported.RefreshToken
			rec.AccessToken = imported.AccessToken
			rec.ClientID = imported.ClientID
			rec.TokenEndpoint = imported.TokenEndpoint
			rec.ExpiresAt = imported.ExpiresAt
		} else {
			rec.RefreshToken = strings.TrimSpace(string(data))
		}
	} else {
		rec.RefreshToken = opts.refreshToken
		rec.AccessToken = opts.accessToken
		rec.ClientID = opts.clientID
		rec.TokenEndpoint = opts.tokenEndpoint
	}

	if rec.RefreshToken == "" && rec.AccessToken == "" {
		if interactive.CanPrompt() {
			rt, err := interactive.Password(fmt.Sprintf("Enter refresh token for provider %q:", providerName))
			if err != nil {
				return fmt.Errorf("read refresh token: %w", err)
			}
			rec.RefreshToken = rt
		} else {
			data, err := io.ReadAll(os.Stdin)
			if err == nil {
				rec.RefreshToken = strings.TrimSpace(string(data))
			}
		}
	}

	if rec.RefreshToken == "" && rec.AccessToken == "" {
		return errors.New("refresh_token (or native credential file) is required")
	}

	if p := preset.Get(providerName); p != nil {
		if rec.ClientID == "" {
			rec.ClientID = p.ClientID
		}
		if rec.TokenEndpoint == "" {
			rec.TokenEndpoint = p.TokenEndpoint
		}
		rec.Profile = p.RefreshProfile
		rec.Scopes = p.Scopes
	}

	if err := store.Save(rec); err != nil {
		return fmt.Errorf("save credential record: %w", err)
	}

	fmt.Printf("imported credential for provider %q\n", providerName)
	return nil
}

func cmdAuthStatus(args []string) error {
	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	store, err := credential.NewStore(svc.CredentialsPath)
	if err != nil {
		return fmt.Errorf("init custodian store: %w", err)
	}

	providerSet := make(map[string]bool)
	for _, p := range preset.All() {
		if p.OAuthCapable {
			providerSet[p.Name] = true
		}
	}
	for _, rec := range store.List() {
		providerSet[rec.Provider] = true
	}

	var names []string
	for name := range providerSet {
		names = append(names, name)
	}
	sort.Strings(names)

	tw := newTabWriter()
	fmt.Fprintln(tw, "PROVIDER\tFLOW\tTIER\tSTATUS")

	for _, name := range names {
		p := preset.Get(name)
		flowType := "-"
		tierTag := "-"
		if p != nil {
			if p.FlowType != "" {
				flowType = p.FlowType
			}
			if p.Tier != "" {
				tierTag = p.Tier
			}
		}

		rec, ok := store.Get(name)
		statusStr := "not connected"
		if ok && (rec.RefreshToken != "" || rec.AccessToken != "") {
			oauthCred := credential.NewOAuthRefreshable(credential.OAuthRefreshableConfig{
				Provider:     rec.Provider,
				RefreshToken: rec.RefreshToken,
				AccessToken:  rec.AccessToken,
				ExpiresAt:    rec.ExpiresAt,
			})
			statusStr = oauthCred.MaskedStatus()
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, flowType, tierTag, statusStr)
	}

	return tw.Flush()
}
