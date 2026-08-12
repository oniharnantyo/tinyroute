package accesslog

import (
	"net/http"
)

// responseWriter wraps an http.ResponseWriter to capture the HTTP status code
// and response body byte count for access logging.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
}

// newResponseWriter creates a responseWriter wrapping w, defaulting statusCode to 200 OK.
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

// WriteHeader records the status code and delegates to the underlying ResponseWriter.
// Subsequent calls after the first are ignored.
func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.statusCode = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

// Write writes the response body bytes and updates bytesWritten count.
// If WriteHeader has not been called yet, it implicitly calls WriteHeader(http.StatusOK).
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Flush flushes buffered data to the client if the underlying ResponseWriter implements http.Flusher.
func (rw *responseWriter) Flush() {
	if fl, ok := rw.ResponseWriter.(http.Flusher); ok {
		if !rw.wroteHeader {
			rw.wroteHeader = true
		}
		fl.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
