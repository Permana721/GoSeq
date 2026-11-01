package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SessionCookieName = "goseq_session"
	SessionDuration   = 7 * 24 * time.Hour
	MaxLoginAttempts  = 5
	RateLimitWindow   = 5 * time.Minute
)

type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		attempts: make(map[string][]time.Time),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-RateLimitWindow)

	// Filter attempts in sliding window
	var recent []time.Time
	for _, t := range rl.attempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	rl.attempts[ip] = recent

	return len(recent) < MaxLoginAttempts
}

func (rl *RateLimiter) RecordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-RateLimitWindow)

	var recent []time.Time
	for _, t := range rl.attempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	rl.attempts[ip] = recent
}

func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-RateLimitWindow)
		for ip, timestamps := range rl.attempts {
			var valid []time.Time
			for _, t := range timestamps {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.attempts, ip)
			} else {
				rl.attempts[ip] = valid
			}
		}
		rl.mu.Unlock()
	}
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

type AuthManager struct {
	cfg         *Config
	rateLimiter *RateLimiter
}

func NewAuthManager(cfg *Config) *AuthManager {
	return &AuthManager{
		cfg:         cfg,
		rateLimiter: NewRateLimiter(),
	}
}

func (a *AuthManager) Authenticate(username, password string) bool {
	userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(a.cfg.AdminUsername)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(a.cfg.AdminPassword)) == 1
	return userMatch && passMatch
}

func (a *AuthManager) GenerateSessionToken(username string) string {
	expiresAt := time.Now().Add(SessionDuration).Unix()
	payload := fmt.Sprintf("%s:%d", username, expiresAt)

	mac := hmac.New(sha256.New, a.cfg.SessionSecret)
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	rawToken := fmt.Sprintf("%s:%s", payload, signature)
	return base64.RawURLEncoding.EncodeToString([]byte(rawToken))
}

func (a *AuthManager) ValidateSessionToken(tokenStr string) bool {
	if tokenStr == "" {
		return false
	}

	decoded, err := base64.RawURLEncoding.DecodeString(tokenStr)
	if err != nil {
		return false
	}

	parts := strings.Split(string(decoded), ":")
	if len(parts) != 3 {
		return false
	}

	user := parts[0]
	expiresStr := parts[1]
	sigHex := parts[2]

	expiresUnix, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return false
	}

	if time.Now().Unix() > expiresUnix {
		return false
	}

	if subtle.ConstantTimeCompare([]byte(user), []byte(a.cfg.AdminUsername)) != 1 {
		return false
	}

	payload := fmt.Sprintf("%s:%d", user, expiresUnix)
	mac := hmac.New(sha256.New, a.cfg.SessionSecret)
	mac.Write([]byte(payload))
	expectedSig := mac.Sum(nil)

	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	return hmac.Equal(sigBytes, expectedSig)
}

func (a *AuthManager) SetSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	isSecure := a.cfg.SecureCookie || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(SessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *AuthManager) ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	isSecure := a.cfg.SecureCookie || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *AuthManager) IsAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	return a.ValidateSessionToken(cookie.Value)
}
