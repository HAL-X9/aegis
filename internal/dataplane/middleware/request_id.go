package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

type contextKey struct{ name string }

var requestIDKey = &contextKey{"request_id"}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, err := uuid.NewV7()
		if err != nil {
			slog.Error("failed to generate request ID", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		slog.Info("request received", "request_id", requestID)

		ctx := context.WithValue(r.Context(), requestIDKey, requestID)

		r = r.WithContext(ctx)

		w.Header().Set("X-Request-ID", requestID.String())
		next.ServeHTTP(w, r)
	})
}
