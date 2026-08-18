package accesslog

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Middleware returns an HTTP handler middleware that logs structured access lines
// for completed requests.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get("X-Request-Id")
			if reqID == "" {
				reqID = fmt.Sprintf("req_%d", time.Now().UnixNano())
			}

			ctx := WithRequestID(r.Context(), reqID)
			rw := newResponseWriter(w)

			start := time.Now()
			next.ServeHTTP(rw, r.WithContext(ctx))
			elapsed := time.Since(start)

			if strings.HasPrefix(r.URL.Path, "/dashboard/assets") || strings.HasPrefix(r.URL.Path, "/assets") || r.URL.Path == "/favicon.ico" {
				return
			}

			logger.Info("access",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.statusCode),
				slog.Int64("bytes", rw.bytesWritten),
				slog.Duration("latency", elapsed),
				slog.String("remote", r.RemoteAddr),
				slog.String("request_id", reqID),
			)
		})
	}
}
