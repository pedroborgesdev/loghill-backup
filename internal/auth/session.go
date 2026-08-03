package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const CookieName = "loghill_session"

type Session struct {
	ID        string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Manager struct {
	mu       sync.RWMutex
	password string
	ttl      time.Duration
	secure   bool
	sessions map[string]Session
}

func NewManager(password string, ttl time.Duration, secure bool) *Manager {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &Manager{
		password: password,
		ttl:      ttl,
		secure:   secure,
		sessions: map[string]Session{},
	}
}

func (m *Manager) Enabled() bool {
	return m != nil && m.password != ""
}

func (m *Manager) Authenticate(password string) bool {
	if m == nil {
		return false
	}
	return secureEqual(password, m.password)
}

func (m *Manager) Create() (Session, error) {
	token, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	session := Session{ID: token, CreatedAt: now, ExpiresAt: now.Add(m.ttl)}
	m.mu.Lock()
	m.sessions[token] = session
	m.mu.Unlock()
	return session, nil
}

func (m *Manager) Valid(token string) bool {
	if m == nil || token == "" {
		return false
	}
	m.mu.RLock()
	session, ok := m.sessions[token]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		m.Revoke(token)
		return false
	}
	return true
}

func (m *Manager) Revoke(token string) {
	if m == nil || token == "" {
		return
	}
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

func (m *Manager) SetCookie(w http.ResponseWriter, session Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.secure,
		Expires:  session.ExpiresAt,
		MaxAge:   int(time.Until(session.ExpiresAt).Seconds()),
	})
}

func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.secure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	})
}

func TokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if cookie, err := r.Cookie(CookieName); err == nil {
		return cookie.Value
	}
	return ""
}

func secureEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
