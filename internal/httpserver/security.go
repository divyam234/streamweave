package httpserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	maxControlBodyBytes = 1 << 20
	adminCookieName     = "streamweave_admin"
	adminSessionTTL     = 12 * time.Hour
	csrfHeader          = "X-CSRF-Protection"
)

type SecurityConfig struct {
	AdminToken    string
	SecureCookies bool
}

type adminSessionManager struct {
	adminToken    string
	secureCookies bool
}

type adminSessionPayload struct {
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"nonce"`
}

func newAdminSessionManager(adminToken string, secureCookies bool) *adminSessionManager {
	return &adminSessionManager{
		adminToken:    strings.TrimSpace(adminToken),
		secureCookies: secureCookies,
	}
}

func (m *adminSessionManager) validAdminToken(provided string) bool {
	if m.adminToken == "" {
		return true
	}
	provided = strings.TrimSpace(provided)
	return len(provided) == len(m.adminToken) &&
		subtle.ConstantTimeCompare([]byte(provided), []byte(m.adminToken)) == 1
}

func (m *adminSessionManager) issue(now time.Time) (string, error) {
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := adminSessionPayload{
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(adminSessionTTL).Unix(),
		Nonce:     base64.RawURLEncoding.EncodeToString(nonce),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(m.adminToken))
	_, _ = mac.Write([]byte(encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + signature, nil
}

func (m *adminSessionManager) valid(value string, now time.Time) bool {
	encoded, signature, ok := strings.Cut(value, ".")
	if !ok || encoded == "" || signature == "" {
		return false
	}
	gotSignature, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(m.adminToken))
	_, _ = mac.Write([]byte(encoded))
	if !hmac.Equal(gotSignature, mac.Sum(nil)) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	var payload adminSessionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	return payload.Nonce != "" && payload.ExpiresAt > now.Unix() && payload.IssuedAt <= now.Add(time.Minute).Unix()
}

func (m *adminSessionManager) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.adminToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(adminCookieName)
		if err != nil || !m.valid(cookie.Value, time.Now()) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if isUnsafeMethod(r.Method) && r.Header.Get(csrfHeader) != "1" {
			http.Error(w, "csrf check failed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *adminSessionManager) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if !m.validAdminToken(input.Token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	value, err := m.issue(time.Now())
	if err != nil {
		http.Error(w, "session creation failed", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, adminCookie(value, int(adminSessionTTL.Seconds()), m.secureCookies))
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"authenticated":true}`))
}

func (m *adminSessionManager) logout(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(csrfHeader) != "1" {
		http.Error(w, "csrf check failed", http.StatusForbidden)
		return
	}
	http.SetCookie(w, adminCookie("", -1, m.secureCookies))
	w.WriteHeader(http.StatusNoContent)
}

func (m *adminSessionManager) session(w http.ResponseWriter, r *http.Request) {
	authenticated := false
	if m.adminToken == "" {
		authenticated = true
	} else if cookie, err := r.Cookie(adminCookieName); err == nil {
		authenticated = m.valid(cookie.Value, time.Now())
	}
	w.Header().Set("Content-Type", "application/json")
	if authenticated {
		_, _ = w.Write([]byte(`{"authenticated":true}`))
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"authenticated":false}`))
}

func adminCookie(value string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     adminCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
}

func isUnsafeMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func securityHeaders(strictTransport bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-site")
			if strictTransport {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func publicCrossOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
		w.Header().Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Length, Content-Range")
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bodyLimit(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
