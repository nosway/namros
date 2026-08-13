package opsauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLocalManagerLoginAndAuthenticate(t *testing.T) {
	manager, err := New(Config{
		Mode:              ModeLocal,
		BootstrapUsername: "admin",
		BootstrapPassword: "secret-pass",
		SessionSecret:     "0123456789abcdef",
		SessionTTL:        time.Hour,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	session, err := manager.Login("admin", "secret-pass")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: session.Token})
	principal, err := manager.RequireRole(req, RoleObserve)
	if err != nil {
		t.Fatalf("RequireRole() error = %v", err)
	}
	if principal.Username != "admin" || len(principal.Roles) == 0 {
		t.Fatalf("principal = %+v", principal)
	}
}

func TestLocalManagerVerifiesCSRFForMutatingRequests(t *testing.T) {
	manager, err := New(Config{
		Mode:              ModeLocal,
		BootstrapUsername: "admin",
		BootstrapPassword: "secret-pass",
		SessionSecret:     "0123456789abcdef",
		SessionTTL:        time.Hour,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	session, err := manager.Login("admin", "secret-pass")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	getReq.AddCookie(&http.Cookie{Name: SessionCookieName, Value: session.Token})
	if err := manager.VerifyCSRF(getReq); err != nil {
		t.Fatalf("VerifyCSRF(GET) error = %v", err)
	}
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/operations/namros.health.check/plan", nil)
	postReq.AddCookie(&http.Cookie{Name: SessionCookieName, Value: session.Token})
	if err := manager.VerifyCSRF(postReq); err != ErrCSRFRequired {
		t.Fatalf("VerifyCSRF(missing) error = %v, want ErrCSRFRequired", err)
	}
	postReq.Header.Set(CSRFHeaderName, manager.CSRFToken(session.Token))
	if err := manager.VerifyCSRF(postReq); err != nil {
		t.Fatalf("VerifyCSRF(valid) error = %v", err)
	}
	postReq.Header.Set(CSRFHeaderName, "bad-token")
	if err := manager.VerifyCSRF(postReq); err != ErrCSRFRequired {
		t.Fatalf("VerifyCSRF(bad) error = %v, want ErrCSRFRequired", err)
	}
}

func TestLocalManagerRejectsBadPassword(t *testing.T) {
	manager, err := New(Config{
		Mode:              ModeLocal,
		BootstrapUsername: "admin",
		BootstrapPassword: "secret-pass",
		SessionSecret:     "0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := manager.Login("admin", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("Login(wrong) error = %v, want ErrInvalidCredentials", err)
	}
}
