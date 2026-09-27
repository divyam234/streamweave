package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminSessionCookieAuth(t *testing.T) {
	manager := newAdminSessionManager("abcdefghijklmnopqrstuvwxyz123456", false)
	value, err := manager.issue(time.Now())
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	handler := manager.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing cookie status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: value})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("valid cookie status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/installations", nil)
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: value})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/installations", nil)
	req.Header.Set(csrfHeader, "1")
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: value})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("csrf-protected status = %d", rec.Code)
	}
}

func TestAdminLoginCookieSecurityComesFromConfiguration(t *testing.T) {
	manager := newAdminSessionManager("abcdefghijklmnopqrstuvwxyz123456", true)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"token":"abcdefghijklmnopqrstuvwxyz123456"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "http")
	rec := httptest.NewRecorder()

	manager.login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != adminCookieName || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %#v", cookie)
	}
}

func TestSecurityHeadersIgnoreForwardedProto(t *testing.T) {
	handler := securityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("unexpected HSTS from forwarded header: %q", got)
	}

	handler = securityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Strict-Transport-Security"); got == "" {
		t.Fatal("production security headers must include HSTS")
	}
}

func TestRateLimiterUsesDirectPeer(t *testing.T) {
	limiter := newFixedWindowLimiter(2, time.Minute)
	handler := limiter.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.20:1234"
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("request %d status = %d", i+1, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.20:9999"
	req.Header.Set("X-Forwarded-For", "5.6.7.8")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected direct peer rate limit, got %d", rec.Code)
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := newFixedWindowLimiter(2, time.Minute)
	now := time.Now()
	if !limiter.allow("client", now) || !limiter.allow("client", now) {
		t.Fatal("first two requests should pass")
	}
	if limiter.allow("client", now) {
		t.Fatal("third request should be limited")
	}
	if !limiter.allow("client", now.Add(time.Minute)) {
		t.Fatal("new window should reset limit")
	}
}

func TestRedactedPathHidesCapabilityTokens(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef0123456789abcdef"
	tests := map[string]string{
		"/addon/" + token + "/manifest.json":               "/addon/[token]/manifest.json",
		"/api/v1/installations/" + token:                   "/api/v1/installations/[token]",
		"/api/v1/installations/" + token + "/rotate-token": "/api/v1/installations/[token]/rotate-token",
		"/api/v1/usenet/stream/" + token:                   "/api/v1/usenet/stream/[token]",
	}
	for input, want := range tests {
		if got := redactedPath(input); got != want {
			t.Fatalf("redactedPath(%q) = %q, want %q", input, got, want)
		}
	}
}
