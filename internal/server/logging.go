package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type contextKey string

const requestIDKey contextKey = "request_id"

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func requestIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(requestIDKey).(string); ok {
		return value
	}
	return ""
}

func logAttrs(r *http.Request, attrs ...any) []any {
	base := []any{
		"request_id", requestIDFromContext(r.Context()),
		"method", r.Method,
		"path", r.URL.Path,
	}
	return append(base, attrs...)
}

func (s *Server) withRequestLogging(route string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqID := randomID(6)
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-Id", reqID)

		recorder := &statusRecorder{ResponseWriter: w}
		start := time.Now()
		next(recorder, r)
		duration := time.Since(start).Milliseconds()
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}

		level := slog.LevelInfo
		if recorder.status >= http.StatusInternalServerError {
			level = slog.LevelError
		} else if recorder.status >= http.StatusBadRequest {
			level = slog.LevelWarn
		}

		slog.Log(r.Context(), level, "http_request",
			"request_id", reqID,
			"route", route,
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"duration_ms", duration,
			"remote_addr", r.RemoteAddr,
		)
	}
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, clientMsg string, err error, attrs ...any) {
	logData := append(logAttrs(r), attrs...)
	if err != nil {
		logData = append(logData, "error", err.Error())
	}
	switch {
	case status >= http.StatusInternalServerError:
		slog.ErrorContext(r.Context(), clientMsg, logData...)
	case status >= http.StatusBadRequest:
		slog.WarnContext(r.Context(), clientMsg, logData...)
	default:
		slog.InfoContext(r.Context(), clientMsg, logData...)
	}
	http.Error(w, clientMsg, status)
}
