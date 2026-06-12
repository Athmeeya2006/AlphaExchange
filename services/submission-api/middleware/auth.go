// Package middleware contains HTTP middleware for submission-api.
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/trade-eval/submission-api/apierrors"
	"github.com/trade-eval/submission-api/repository"
)

type ctxKey string

// ContestantContextKey is the context key under which the authenticated
// contestant is stored.
const ContestantContextKey ctxKey = "contestant"

// ContestantFromContext returns the authenticated contestant, if any.
func ContestantFromContext(ctx context.Context) (*repository.Contestant, bool) {
	c, ok := ctx.Value(ContestantContextKey).(*repository.Contestant)
	return c, ok
}

// Auth builds the API-key authentication + per-contestant rate-limiting
// middleware. Rate limit: 10 submissions/hour, enforced only on POST
// /v1/submissions.
func Auth(repo repository.ContestantRepository, rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				apierrors.WriteError(w, &apierrors.ErrUnauthorized{Message: "missing X-API-Key header"})
				return
			}

			contestant, err := repo.GetByAPIKey(r.Context(), apiKey)
			if err != nil {
				apierrors.WriteError(w, err)
				return
			}

			// Rate limit submission creation: 10/hour per contestant.
			if r.Method == http.MethodPost && r.URL.Path == "/v1/submissions" && rdb != nil {
				key := "rate_limit:submission:" + contestant.ID
				count, err := rdb.Incr(r.Context(), key).Result()
				if err == nil {
					if count == 1 {
						rdb.Expire(r.Context(), key, time.Hour)
					}
					if count > 10 {
						apierrors.WriteError(w, &apierrors.ErrRateLimit{RetryAfter: 3600})
						return
					}
				}
			}

			ctx := context.WithValue(r.Context(), ContestantContextKey, contestant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
