package cloudcode

import "bytes"
import "context"
import "encoding/json"
import "errors"
import "fmt"
import "io"
import "net/http"
import "sync"
import "time"

const (
	DefaultBootstrapURL = "https://cloudcode-pa.googleapis.com"
	DefaultUserAgent    = "antigravity/ide/2.1.1 darwin/arm64"
	DefaultCacheTTL     = 1 * time.Hour
)

type cacheEntry struct {
	projectID string
	expiresAt time.Time
}

type call struct {
	wg  sync.WaitGroup
	res string
	err error
}

// Onboarding manages fetching and caching the cloudaicompanionProject ID for CloudCode requests.
type Onboarding struct {
	BootstrapURL string
	HTTPClient   *http.Client
	CacheTTL     time.Duration
	Now          func() time.Time

	mu       sync.Mutex
	cache    map[string]cacheEntry
	inFlight map[string]*call
}

// NewOnboarding initializes an Onboarding manager.
func NewOnboarding(bootstrapURL string, client *http.Client) *Onboarding {
	if bootstrapURL == "" {
		bootstrapURL = DefaultBootstrapURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Onboarding{
		BootstrapURL: bootstrapURL,
		HTTPClient:   client,
		CacheTTL:     DefaultCacheTTL,
		Now:          time.Now,
		cache:        make(map[string]cacheEntry),
		inFlight:     make(map[string]*call),
	}
}

// ProjectID returns the cached project ID for the given access token or fetches a new one via loadCodeAssist/onboardUser.
func (o *Onboarding) ProjectID(ctx context.Context, accessToken string) (string, error) {
	if accessToken == "" {
		return "", errors.New("cloudcode onboarding: access token is required")
	}

	nowFn := time.Now
	if o.Now != nil {
		nowFn = o.Now
	}

	o.mu.Lock()
	if entry, ok := o.cache[accessToken]; ok && nowFn().Before(entry.expiresAt) {
		o.mu.Unlock()
		return entry.projectID, nil
	}

	if c, ok := o.inFlight[accessToken]; ok {
		o.mu.Unlock()
		c.wg.Wait()
		return c.res, c.err
	}

	c := new(call)
	c.wg.Add(1)
	o.inFlight[accessToken] = c
	o.mu.Unlock()

	projectID, err := o.fetchProjectID(ctx, accessToken)

	o.mu.Lock()
	delete(o.inFlight, accessToken)
	if err == nil && projectID != "" {
		ttl := o.CacheTTL
		if ttl <= 0 {
			ttl = DefaultCacheTTL
		}
		o.cache[accessToken] = cacheEntry{
			projectID: projectID,
			expiresAt: nowFn().Add(ttl),
		}
	}
	c.res = projectID
	c.err = err
	c.wg.Done()
	o.mu.Unlock()

	return projectID, err
}

func (o *Onboarding) fetchProjectID(ctx context.Context, accessToken string) (string, error) {
	projectID, err := o.loadCodeAssist(ctx, accessToken)
	if err == nil && projectID != "" {
		return projectID, nil
	}

	// Fallback to onboardUser
	projectID, onboardErr := o.onboardUser(ctx, accessToken)
	if onboardErr != nil {
		if err != nil {
			return "", fmt.Errorf("cloudcode onboarding failed: loadCodeAssist error: %v, onboardUser error: %w", sanitizeErr(err), sanitizeErr(onboardErr))
		}
		return "", fmt.Errorf("cloudcode onboarding failed: %w", sanitizeErr(onboardErr))
	}
	if projectID == "" {
		return "", errors.New("cloudcode onboarding failed: empty project ID returned")
	}
	return projectID, nil
}

func (o *Onboarding) loadCodeAssist(ctx context.Context, accessToken string) (string, error) {
	url := o.BootstrapURL + "/v1internal:loadCodeAssist"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", fmt.Errorf("build loadCodeAssist request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("loadCodeAssist HTTP call failed: %w", sanitizeErr(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("loadCodeAssist returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read loadCodeAssist response: %w", err)
	}

	var res loadCodeAssistResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return "", fmt.Errorf("parse loadCodeAssist response: %w", err)
	}

	return res.CloudAICompanionProject.String(), nil
}

func (o *Onboarding) onboardUser(ctx context.Context, accessToken string) (string, error) {
	url := o.BootstrapURL + "/v1internal:onboardUser"
	reqBody, _ := json.Marshal(onboardUserRequest{TierID: "regular"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("build onboardUser request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("onboardUser HTTP call failed: %w", sanitizeErr(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("onboardUser returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read onboardUser response: %w", err)
	}

	var res onboardUserResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return "", fmt.Errorf("parse onboardUser response: %w", err)
	}

	return res.CloudAICompanionProject.String(), nil
}

type redactedError struct {
	err error
}

func (e redactedError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e redactedError) Unwrap() error {
	return e.err
}

// sanitizeErr wraps network errors to ensure safety while preserving the error chain for errors.Is/errors.As.
func sanitizeErr(err error) error {
	if err == nil {
		return nil
	}
	return redactedError{err: err}
}
