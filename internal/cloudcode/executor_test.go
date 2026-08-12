package cloudcode_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/cloudcode"
)

func TestExecutor_NonStream_EnvelopeAndHeaders(t *testing.T) {
	nativePayload := `{"contents":[{"parts":[{"text":"Hello"}]}]}`
	var capturedEnvelope cloudcode.Envelope
	var capturedAuth, capturedUA, capturedCT string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:generateContent" {
			t.Errorf("got path %q, want /v1internal:generateContent", r.URL.Path)
		}
		capturedAuth = r.Header.Get("Authorization")
		capturedUA = r.Header.Get("User-Agent")
		capturedCT = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		if err := json.Unmarshal(body, &capturedEnvelope); err != nil {
			t.Fatalf("failed to unmarshal envelope: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Hi there!"}]}}]}`))
	}))
	defer server.Close()

	executor := cloudcode.NewExecutor(server.Client())
	resp, err := executor.Send(
		context.Background(),
		server.URL,
		"proj-test-1",
		"gemini-3.6-flash-medium",
		"tok-abc-123",
		[]byte(nativePayload),
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if capturedAuth != "Bearer tok-abc-123" {
		t.Errorf("got Authorization %q, want Bearer tok-abc-123", capturedAuth)
	}
	if capturedUA != cloudcode.DefaultUserAgent {
		t.Errorf("got User-Agent %q, want %q", capturedUA, cloudcode.DefaultUserAgent)
	}
	if capturedCT != "application/json" {
		t.Errorf("got Content-Type %q, want application/json", capturedCT)
	}

	if capturedEnvelope.Project != "proj-test-1" {
		t.Errorf("envelope project = %q, want proj-test-1", capturedEnvelope.Project)
	}
	if capturedEnvelope.Model != "gemini-3.6-flash-medium" {
		t.Errorf("envelope model = %q, want gemini-3.6-flash-medium", capturedEnvelope.Model)
	}
	if capturedEnvelope.UserAgent != "antigravity" {
		t.Errorf("envelope userAgent = %q, want antigravity", capturedEnvelope.UserAgent)
	}
	if capturedEnvelope.RequestType != "agent" {
		t.Errorf("envelope requestType = %q, want agent", capturedEnvelope.RequestType)
	}
	if !strings.HasPrefix(capturedEnvelope.RequestID, "agent-") {
		t.Errorf("envelope requestId = %q, expected prefix 'agent-'", capturedEnvelope.RequestID)
	}

	rawRequest := string(capturedEnvelope.Request)
	if rawRequest != nativePayload {
		t.Errorf("envelope request payload = %q, want %q", rawRequest, nativePayload)
	}

	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "Hi there!") {
		t.Errorf("expected response body to contain 'Hi there!', got %q", string(respBody))
	}
}

func TestExecutor_ModelStrippedFromInnerPayload(t *testing.T) {
	nativePayloadWithModel := `{"model":"gemini-3.6-flash-medium","contents":[{"parts":[{"text":"Test"}]}]}`
	var capturedEnvelope cloudcode.Envelope

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedEnvelope)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	executor := cloudcode.NewExecutor(server.Client())
	resp, err := executor.Send(
		context.Background(),
		server.URL,
		"proj-test-strip",
		"gemini-3.6-flash-medium",
		"tok-123",
		[]byte(nativePayloadWithModel),
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var innerMap map[string]interface{}
	if err := json.Unmarshal(capturedEnvelope.Request, &innerMap); err != nil {
		t.Fatalf("failed to parse inner request JSON: %v", err)
	}
	if _, exists := innerMap["model"]; exists {
		t.Errorf("expected top-level 'model' field to be stripped from inner request payload, but it was present: %v", innerMap)
	}
}

func TestExecutor_Stream_Endpoint(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"candidates\":[]}\n\n"))
	}))
	defer server.Close()

	executor := cloudcode.NewExecutor(server.Client())
	resp, err := executor.Send(
		context.Background(),
		server.URL,
		"proj-test-2",
		"gemini-3.6-flash-high",
		"tok-stream",
		[]byte("{}"),
		true, // stream = true
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	wantPath := "/v1internal:streamGenerateContent?alt=sse"
	if capturedPath != wantPath {
		t.Errorf("got path %q, want %q", capturedPath, wantPath)
	}
}

func TestExecutor_DefaultBaseURLAndNilClient(t *testing.T) {
	exec := cloudcode.NewExecutor(nil)
	req, err := exec.GenerateRequest(context.Background(), "", "proj", "model", "token", []byte("{}"), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.URL.String() != "https://daily-cloudcode-pa.googleapis.com/v1internal:generateContent" {
		t.Errorf("got URL %q, want default CloudCode URL", req.URL.String())
	}
}

func TestExecutor_SendError(t *testing.T) {
	exec := cloudcode.NewExecutor(&http.Client{})
	// Bad URL host to trigger HTTP send error
	_, err := exec.Send(context.Background(), "http://127.0.0.1:1", "proj", "model", "tok", []byte("{}"), false)
	if err == nil {
		t.Errorf("expected error sending to closed port, got nil")
	}
}
