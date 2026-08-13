package opsauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/nosway/namros/internal/config"
)

const (
	ModeDisabled = "disabled"
	ModeLocal    = "local"

	SessionCookieName = "namros_console_session"
	CSRFHeaderName    = "X-Namros-CSRF-Token"

	RoleObserve     = "observe"
	RoleProbe       = "probe"
	RoleRepair      = "repair"
	RoleProtect     = "protect"
	RoleDestructive = "destructive"
	RoleAdmin       = "admin"
)

var (
	ErrDisabled           = errors.New("console auth is disabled")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("console session is required")
	ErrForbidden          = errors.New("console role is required")
	ErrCSRFRequired       = errors.New("console csrf token is required")
)

type Config struct {
	Mode              string
	BootstrapUsername string
	BootstrapPassword string
	SessionSecret     string
	SessionTTL        time.Duration
}

func ConfigFromApp(cfg config.Config) Config {
	return Config{
		Mode:              cfg.ConsoleAuthMode,
		BootstrapUsername: cfg.ConsoleAdminUsername,
		BootstrapPassword: cfg.ConsoleAdminPassword,
		SessionSecret:     cfg.ConsoleSessionSecret,
		SessionTTL:        cfg.ConsoleSessionTTL,
	}
}

type Manager struct {
	mode          string
	sessionSecret []byte
	sessionTTL    time.Duration

	mu     sync.RWMutex
	users  map[string]User
	groups map[string]Group
}

type User struct {
	Username     string    `json:"username"`
	PasswordHash []byte    `json:"-"`
	Groups       []string  `json:"groups"`
	Disabled     bool      `json:"disabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastLoginAt  time.Time `json:"last_login_at,omitempty"`
}

type Group struct {
	Name      string    `json:"name"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Principal struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups"`
	Roles    []string `json:"roles"`
}

type Session struct {
	Token     string
	Principal Principal
	ExpiresAt time.Time
}

func New(cfg Config) (*Manager, error) {
	mode := NormalizeMode(cfg.Mode)
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	m := &Manager{
		mode:       mode,
		sessionTTL: cfg.SessionTTL,
		users:      map[string]User{},
		groups:     map[string]Group{},
	}
	if mode == ModeDisabled {
		return m, nil
	}
	if mode != ModeLocal {
		return nil, fmt.Errorf("unsupported console auth mode %q", cfg.Mode)
	}
	username := strings.TrimSpace(cfg.BootstrapUsername)
	if username == "" {
		return nil, errors.New("console bootstrap username is required")
	}
	if cfg.BootstrapPassword == "" {
		return nil, errors.New("console bootstrap password is required")
	}
	if len(cfg.SessionSecret) < 16 {
		return nil, errors.New("console session secret must be at least 16 bytes")
	}
	m.sessionSecret = []byte(cfg.SessionSecret)
	now := time.Now().UTC()
	group := Group{
		Name:      "platform-admins",
		Roles:     []string{RoleObserve, RoleProbe, RoleRepair, RoleProtect, RoleAdmin},
		CreatedAt: now,
		UpdatedAt: now,
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.BootstrapPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	m.groups[group.Name] = group
	m.users[username] = User{
		Username:     username,
		PasswordHash: hash,
		Groups:       []string{group.Name},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return m, nil
}

func MustDisabled() *Manager {
	m, err := New(Config{Mode: ModeDisabled})
	if err != nil {
		panic(err)
	}
	return m
}

func NormalizeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return ModeDisabled
	}
	return mode
}

func (m *Manager) Enabled() bool {
	return m != nil && m.mode != ModeDisabled
}

func (m *Manager) Login(username, password string) (Session, error) {
	if m == nil || !m.Enabled() {
		return Session{}, ErrDisabled
	}
	username = strings.TrimSpace(username)
	m.mu.RLock()
	user, ok := m.users[username]
	m.mu.RUnlock()
	if !ok || user.Disabled {
		return Session{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)); err != nil {
		return Session{}, ErrInvalidCredentials
	}
	expiresAt := time.Now().UTC().Add(m.sessionTTL)
	principal := m.principalForUser(user)
	token := m.signSession(username, expiresAt)
	m.mu.Lock()
	user.LastLoginAt = time.Now().UTC()
	m.users[username] = user
	m.mu.Unlock()
	return Session{Token: token, Principal: principal, ExpiresAt: expiresAt}, nil
}

func (m *Manager) AuthenticateRequest(r *http.Request) (Principal, error) {
	if m == nil || !m.Enabled() {
		return Principal{}, nil
	}
	username, _, err := m.sessionFromRequest(r)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	m.mu.RLock()
	user, ok := m.users[username]
	m.mu.RUnlock()
	if !ok || user.Disabled {
		return Principal{}, ErrUnauthenticated
	}
	return m.principalForUser(user), nil
}

func (m *Manager) CSRFToken(sessionToken string) string {
	if m == nil || !m.Enabled() || strings.TrimSpace(sessionToken) == "" {
		return ""
	}
	mac := hmac.New(sha256.New, m.sessionSecret)
	_, _ = mac.Write([]byte("csrf:" + sessionToken))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *Manager) CSRFTokenForRequest(r *http.Request) (string, error) {
	if m == nil || !m.Enabled() {
		return "", nil
	}
	_, token, err := m.sessionFromRequest(r)
	if err != nil {
		return "", err
	}
	return m.CSRFToken(token), nil
}

func (m *Manager) VerifyCSRF(r *http.Request) error {
	if m == nil || !m.Enabled() || consoleCSRFSafeMethod(r.Method) {
		return nil
	}
	expected, err := m.CSRFTokenForRequest(r)
	if err != nil {
		return err
	}
	provided := strings.TrimSpace(r.Header.Get(CSRFHeaderName))
	if provided == "" {
		return ErrCSRFRequired
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return ErrCSRFRequired
	}
	return nil
}

func (m *Manager) RequireRole(r *http.Request, role string) (Principal, error) {
	principal, err := m.AuthenticateRequest(r)
	if err != nil || m == nil || !m.Enabled() || role == "" {
		return principal, err
	}
	for _, candidate := range principal.Roles {
		if candidate == role || candidate == RoleAdmin {
			return principal, nil
		}
	}
	return Principal{}, ErrForbidden
}

func (m *Manager) Users() []User {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]User, 0, len(m.users))
	for _, user := range m.users {
		user.PasswordHash = nil
		out = append(out, user)
	}
	return out
}

func (m *Manager) Groups() []Group {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Group, 0, len(m.groups))
	for _, group := range m.groups {
		out = append(out, group)
	}
	return out
}

func Roles() []string {
	return []string{RoleObserve, RoleProbe, RoleRepair, RoleProtect, RoleDestructive, RoleAdmin}
}

func (m *Manager) principalForUser(user User) Principal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	roleSet := map[string]struct{}{}
	groups := append([]string(nil), user.Groups...)
	for _, groupName := range user.Groups {
		group, ok := m.groups[groupName]
		if !ok {
			continue
		}
		for _, role := range group.Roles {
			roleSet[role] = struct{}{}
		}
	}
	roles := make([]string, 0, len(roleSet))
	for role := range roleSet {
		roles = append(roles, role)
	}
	return Principal{Username: user.Username, Groups: groups, Roles: roles}
}

func (m *Manager) sessionFromRequest(r *http.Request) (string, string, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", "", ErrUnauthenticated
	}
	username, expiresAt, ok := m.verifySession(cookie.Value)
	if !ok || time.Now().UTC().After(expiresAt) {
		return "", "", ErrUnauthenticated
	}
	return username, cookie.Value, nil
}

func consoleCSRFSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func (m *Manager) signSession(username string, expiresAt time.Time) string {
	payload := username + "|" + fmt.Sprintf("%d", expiresAt.Unix())
	mac := hmac.New(sha256.New, m.sessionSecret)
	_, _ = mac.Write([]byte(payload))
	signature := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func (m *Manager) verifySession(token string) (string, time.Time, bool) {
	payloadEncoded, sigEncoded, ok := strings.Cut(token, ".")
	if !ok {
		return "", time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadEncoded)
	if err != nil {
		return "", time.Time{}, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(sigEncoded)
	if err != nil {
		return "", time.Time{}, false
	}
	mac := hmac.New(sha256.New, m.sessionSecret)
	_, _ = mac.Write(payload)
	if subtle.ConstantTimeCompare(signature, mac.Sum(nil)) != 1 {
		return "", time.Time{}, false
	}
	username, rawExpires, ok := strings.Cut(string(payload), "|")
	if !ok || strings.TrimSpace(username) == "" {
		return "", time.Time{}, false
	}
	expiresUnix, err := parseUnix(rawExpires)
	if err != nil {
		return "", time.Time{}, false
	}
	return username, time.Unix(expiresUnix, 0).UTC(), true
}

func parseUnix(raw string) (int64, error) {
	var out int64
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0, errors.New("invalid unix timestamp")
		}
		out = out*10 + int64(ch-'0')
	}
	return out, nil
}
