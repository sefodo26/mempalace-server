package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newReq(authHeader string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/mp/mcp", nil)
	if authHeader != "" {
		r.Header.Set("Authorization", authHeader)
	}
	return r
}

func TestBearerResolvesPerm(t *testing.T) {
	const writeKey, readKey = "write-key", "read-key"

	cases := []struct {
		name       string
		header     string
		wantStatus int
		wantPerm   Perm
	}{
		{"full access", "Bearer " + writeKey, http.StatusOK, PermReadWrite},
		{"read only", "Bearer " + readKey, http.StatusOK, PermRead},
		{"unknown token", "Bearer nope", http.StatusUnauthorized, PermNone},
		{"missing header", "", http.StatusUnauthorized, PermNone},
		{"wrong scheme", "Basic " + writeKey, http.StatusUnauthorized, PermNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPerm Perm
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				gotPerm = PermFromContext(r.Context())
			})
			rec := httptest.NewRecorder()
			Bearer(writeKey, readKey, next).ServeHTTP(rec, newReq(tc.header))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusOK && gotPerm != tc.wantPerm {
				t.Fatalf("perm = %d, want %d", gotPerm, tc.wantPerm)
			}
		})
	}
}

func TestBearerReadOnlyDisabledWhenEmpty(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	rec := httptest.NewRecorder()
	// readKey empty → only the write key is accepted; an empty token must fail.
	Bearer("write-key", "", next).ServeHTTP(rec, newReq("Bearer "))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestBearerMisconfiguredWhenNoWriteKey(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	rec := httptest.NewRecorder()
	Bearer("", "", next).ServeHTTP(rec, newReq("Bearer anything"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestBearerHealthBypass(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/mp/mcp/health", nil)
	Bearer("write-key", "", next).ServeHTTP(rec, r)
	if !called {
		t.Fatal("health endpoint should bypass auth")
	}
}
