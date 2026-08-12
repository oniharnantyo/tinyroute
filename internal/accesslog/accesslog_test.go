package accesslog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/accesslog"
)

type flushableResponseWriter struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flushableResponseWriter) Flush() {
	f.flushed = true
	f.ResponseRecorder.Flush()
}

func TestResponseWriter_CustomStatusAndBytes(t *testing.T) {
	rec := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, err := w.Write([]byte("rate limit exceeded"))
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
	})

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := accesslog.Middleware(logger)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)
	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected recorder status %d, got %d", http.StatusTooManyRequests, rec.Code)
	}

	bodyStr := rec.Body.String()
	if bodyStr != "rate limit exceeded" {
		t.Errorf("expected body %q, got %q", "rate limit exceeded", bodyStr)
	}

	var logRecord map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logRecord); err != nil {
		t.Fatalf("failed to unmarshal log JSON: %v, raw: %s", err, buf.String())
	}

	if status, ok := logRecord["status"].(float64); !ok || int(status) != 429 {
		t.Errorf("expected status 429 in log, got %v", logRecord["status"])
	}

	if bytesCount, ok := logRecord["bytes"].(float64); !ok || int64(bytesCount) != int64(len("rate limit exceeded")) {
		t.Errorf("expected bytes %d in log, got %v", len("rate limit exceeded"), logRecord["bytes"])
	}
}

func TestResponseWriter_DefaultStatusAndMultipleWrites(t *testing.T) {
	rec := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello "))
		w.Write([]byte("world"))
		w.WriteHeader(http.StatusBadRequest)
	})

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := accesslog.Middleware(logger)

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	mw(handler).ServeHTTP(rec, req)

	var logRecord map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logRecord); err != nil {
		t.Fatalf("failed to unmarshal log JSON: %v, raw: %s", err, buf.String())
	}

	if status, ok := logRecord["status"].(float64); !ok || int(status) != http.StatusOK {
		t.Errorf("expected status 200 in log, got %v", logRecord["status"])
	}

	if bytesCount, ok := logRecord["bytes"].(float64); !ok || int64(bytesCount) != 11 {
		t.Errorf("expected bytes 11 in log, got %v", logRecord["bytes"])
	}
}

func TestResponseWriter_FlusherAndUnwrap(t *testing.T) {
	baseRec := httptest.NewRecorder()
	fw := &flushableResponseWriter{ResponseRecorder: baseRec}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected ResponseWriter to implement http.Flusher")
		}
		w.Write([]byte("data"))
		flusher.Flush()

		unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			t.Fatal("expected ResponseWriter to implement Unwrap()")
		}
		if unwrapper.Unwrap() != fw {
			t.Errorf("expected Unwrap() to return original writer")
		}
	})

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := accesslog.Middleware(logger)

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	mw(handler).ServeHTTP(fw, req)

	if !fw.flushed {
		t.Error("expected underlying flusher to be called")
	}
}

func TestContextRequestID(t *testing.T) {
	ctx := context.Background()

	if id := accesslog.RequestID(ctx); id != "" {
		t.Errorf("expected empty string for context without request ID, got %q", id)
	}

	reqID := "req_test_12345"
	ctxWithID := accesslog.WithRequestID(ctx, reqID)

	if id := accesslog.RequestID(ctxWithID); id != reqID {
		t.Errorf("expected %q, got %q", reqID, id)
	}
}

func TestMiddleware_RequestID_HonorsHeader(t *testing.T) {
	var capturedID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = accesslog.RequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := accesslog.Middleware(logger)

	callerID := "caller-supplied-id-999"
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-Id", callerID)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if capturedID != callerID {
		t.Errorf("expected context request_id %q, got %q", callerID, capturedID)
	}

	var logRecord map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logRecord); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}
	if logRecord["request_id"] != callerID {
		t.Errorf("expected log request_id %q, got %v", callerID, logRecord["request_id"])
	}
}

func TestMiddleware_RequestID_AutoGeneratesWhenAbsent(t *testing.T) {
	var capturedID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = accesslog.RequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := accesslog.Middleware(logger)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if capturedID == "" {
		t.Error("expected auto-generated request ID, got empty string")
	}
	if !strings.HasPrefix(capturedID, "req_") {
		t.Errorf("expected request ID to start with 'req_', got %q", capturedID)
	}

	var logRecord map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logRecord); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}
	if logRecord["request_id"] != capturedID {
		t.Errorf("expected log request_id %q to match context request_id %q", logRecord["request_id"], capturedID)
	}
}

func TestMiddleware_PreProxyEarlyReturn401(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"invalid_api_key"}`)
	})

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := accesslog.Middleware(logger)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)
	req.RemoteAddr = "192.168.1.50:12345"
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected response code 401, got %d", rec.Code)
	}

	var logRecord map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logRecord); err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if logRecord["msg"] != "access" {
		t.Errorf("expected msg 'access', got %v", logRecord["msg"])
	}
	if logRecord["level"] != "INFO" {
		t.Errorf("expected level 'INFO', got %v", logRecord["level"])
	}
	if logRecord["method"] != "POST" {
		t.Errorf("expected method 'POST', got %v", logRecord["method"])
	}
	if logRecord["path"] != "/openai/v1/chat/completions" {
		t.Errorf("expected path '/openai/v1/chat/completions', got %v", logRecord["path"])
	}
	if status, ok := logRecord["status"].(float64); !ok || int(status) != 401 {
		t.Errorf("expected status 401, got %v", logRecord["status"])
	}
	expectedBytes := int64(len(`{"error":"invalid_api_key"}`))
	if bytesCount, ok := logRecord["bytes"].(float64); !ok || int64(bytesCount) != expectedBytes {
		t.Errorf("expected bytes %d, got %v", expectedBytes, logRecord["bytes"])
	}
	if logRecord["remote"] != "192.168.1.50:12345" {
		t.Errorf("expected remote '192.168.1.50:12345', got %v", logRecord["remote"])
	}
	if _, ok := logRecord["latency"].(float64); !ok {
		t.Errorf("expected numeric latency attribute, got %v", logRecord["latency"])
	}
}
