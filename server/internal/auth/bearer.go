package auth

import (
	"context"
	"net/http"
	"strings"
)

// Perm is the access level granted to a request based on its API key.
type Perm int

const (
	PermNone      Perm = iota // no valid key
	PermRead                  // read-only: non-mutating operations
	PermReadWrite             // full access: read + write
)

type ctxKey struct{}

// PermFromContext returns the access level stored on the request context by
// the Bearer middleware (PermNone if absent).
func PermFromContext(ctx context.Context) Perm {
	p, _ := ctx.Value(ctxKey{}).(Perm)
	return p
}

// Bearer returns an HTTP middleware that enforces Authorization: Bearer <token>
// and resolves the token to an access level:
//
//   - writeKey → PermReadWrite (full access). Required; the server is
//     misconfigured without it.
//   - readKey  → PermRead (read-only). Optional; pass "" to disable.
//
// The resolved Perm is stored on the request context for handlers to check.
// The health endpoint is exempted so Kubernetes liveness probes work without auth.
func Bearer(writeKey, readKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health check — no auth required
		if r.URL.Path == "/mp/mcp/health" {
			next.ServeHTTP(w, r)
			return
		}

		if writeKey == "" {
			jsonError(w, http.StatusServiceUnavailable, "server misconfigured: MCP_API_KEY not set")
			return
		}

		token, ok := bearerToken(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			jsonError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var perm Perm
		switch {
		case token == writeKey:
			perm = PermReadWrite
		case readKey != "" && token == readKey:
			perm = PermRead
		default:
			w.Header().Set("WWW-Authenticate", "Bearer")
			jsonError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), ctxKey{}, perm)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", false
	}
	return strings.TrimPrefix(h, "Bearer "), true
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write([]byte(`{"error":"` + msg + `"}`)) //nolint:errcheck
}
