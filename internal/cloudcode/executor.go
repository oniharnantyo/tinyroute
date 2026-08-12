package cloudcode

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://daily-cloudcode-pa.googleapis.com"

// Executor builds and executes CloudCode HTTP requests wrapping native Gemini payloads in the CloudCode envelope.
type Executor struct {
	HTTPClient *http.Client
}

// NewExecutor creates a new CloudCode Executor.
func NewExecutor(client *http.Client) *Executor {
	if client == nil {
		client = http.DefaultClient
	}
	return &Executor{HTTPClient: client}
}

// GenerateRequest constructs the HTTP request with the CloudCode envelope and headers.
func (e *Executor) GenerateRequest(ctx context.Context, baseURL, projectID, model, accessToken string, rawPayload []byte, isStream bool) (*http.Request, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	path := "/v1internal:generateContent"
	if isStream {
		path = "/v1internal:streamGenerateContent?alt=sse"
	}
	endpointURL := baseURL + path

	requestID := generateRequestID()

	cleanedPayload := rawPayload
	var rawMap map[string]interface{}
	if err := json.Unmarshal(rawPayload, &rawMap); err == nil && rawMap != nil {
		if _, exists := rawMap["model"]; exists {
			delete(rawMap, "model")
			if marshaled, err := json.Marshal(rawMap); err == nil {
				cleanedPayload = marshaled
			}
		}
	}

	envelope := Envelope{
		Project:     projectID,
		Model:       model,
		UserAgent:   "antigravity",
		RequestType: "agent",
		RequestID:   requestID,
		Request:     json.RawMessage(cleanedPayload),
	}

	bodyBytes, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("cloudcode: marshal envelope: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("cloudcode: new request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// Send builds and executes a CloudCode request, returning the HTTP response.
func (e *Executor) Send(ctx context.Context, baseURL, projectID, model, accessToken string, rawPayload []byte, isStream bool) (*http.Response, error) {
	req, err := e.GenerateRequest(ctx, baseURL, projectID, model, accessToken, rawPayload, isStream)
	if err != nil {
		return nil, err
	}
	resp, err := e.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudcode request failed: %w", sanitizeErr(err))
	}
	return resp, nil
}

func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	return "agent-" + hex.EncodeToString(b)
}
