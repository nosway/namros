package gateway

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/mcpops"
	"github.com/nosway/namros/internal/opsauth"
)

type consoleLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func registerConsoleAuthAPI(api *gin.RouterGroup, cfg config.Config, auth *opsauth.Manager) {
	api.GET("/auth/session", consoleAuthSession(auth))
	api.GET("/auth/providers", consoleAuthProviders(cfg, auth))
	api.POST("/auth/login", consoleAuthLogin(auth))
	api.POST("/auth/logout", consoleAuthLogout(auth))
}

func consoleAuthProviders(cfg config.Config, auth *opsauth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		localStatus := "disabled"
		if auth != nil && auth.Enabled() {
			localStatus = "enabled"
		}
		c.JSON(http.StatusOK, gin.H{
			"schema_version": "namros.console.auth.providers.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"providers": []gin.H{
				{
					"id":     "local",
					"kind":   "local_password",
					"status": localStatus,
				},
				authEnterpriseProvider(cfg, "oidc", "OpenID Connect"),
				authEnterpriseProvider(cfg, "saml2", "SAML2"),
				authEnterpriseProvider(cfg, "ldap", "LDAP/Active Directory"),
			},
		})
	}
}

func authEnterpriseProvider(cfg config.Config, id, name string) gin.H {
	out := gin.H{
		"id":              id,
		"kind":            id,
		"name":            name,
		"minimum_edition": edition.Enterprise,
		"status":          "unconfigured",
	}
	if !edition.Allows(cfg.Edition, edition.FeatureExternalIAMFederation) {
		out["enterprise_required"] = mcpops.EnterpriseRequired("namros.console.auth."+id, edition.FeatureExternalIAMFederation)
	}
	return out
}

func consoleAuthGuard(auth *opsauth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if auth == nil || !auth.Enabled() {
			c.Next()
			return
		}
		principal, err := auth.RequireRole(c.Request, opsauth.RoleObserve)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"schema_version": "namros.console.auth.error.v1",
				"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
				"status":         "unauthenticated",
				"error":          err.Error(),
			})
			return
		}
		if err := auth.VerifyCSRF(c.Request); err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"schema_version": "namros.console.auth.error.v1",
				"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
				"status":         "csrf_denied",
				"error":          err.Error(),
				"csrf_header":    opsauth.CSRFHeaderName,
			})
			return
		}
		c.Set("console_principal", principal)
		c.Next()
	}
}

func consoleAuthSession(auth *opsauth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if auth == nil || !auth.Enabled() {
			c.JSON(http.StatusOK, gin.H{
				"schema_version": "namros.console.auth.session.v1",
				"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
				"authenticated":  false,
				"mode":           opsauth.ModeDisabled,
				"csrf_required":  false,
			})
			return
		}
		principal, err := auth.AuthenticateRequest(c.Request)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"schema_version": "namros.console.auth.session.v1",
				"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
				"authenticated":  false,
				"mode":           opsauth.ModeLocal,
				"csrf_required":  true,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"schema_version": "namros.console.auth.session.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"authenticated":  true,
			"mode":           opsauth.ModeLocal,
			"user":           principal,
			"csrf_required":  true,
			"csrf_header":    opsauth.CSRFHeaderName,
			"csrf_token":     sessionCSRFToken(auth, c.Request),
		})
	}
}

func consoleAuthLogin(auth *opsauth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if auth == nil || !auth.Enabled() {
			c.JSON(http.StatusConflict, gin.H{
				"schema_version": "namros.console.auth.login.v1",
				"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
				"status":         "disabled",
			})
			return
		}
		var req consoleLoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"schema_version": "namros.console.auth.login.v1",
				"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
				"status":         "bad_request",
				"error":          err.Error(),
			})
			return
		}
		session, err := auth.Login(req.Username, req.Password)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"schema_version": "namros.console.auth.login.v1",
				"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
				"status":         "denied",
			})
			return
		}
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     opsauth.SessionCookieName,
			Value:    session.Token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Expires:  session.ExpiresAt,
		})
		c.JSON(http.StatusOK, gin.H{
			"schema_version": "namros.console.auth.login.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"status":         "ok",
			"user":           session.Principal,
			"expires_at":     session.ExpiresAt.Format(time.RFC3339Nano),
			"csrf_header":    opsauth.CSRFHeaderName,
			"csrf_token":     auth.CSRFToken(session.Token),
		})
	}
}

func consoleAuthLogout(auth *opsauth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     opsauth.SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		status := "ok"
		if auth == nil || !auth.Enabled() {
			status = "disabled"
		}
		c.JSON(http.StatusOK, gin.H{
			"schema_version": "namros.console.auth.logout.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"status":         status,
		})
	}
}

func consoleAdminUsers(auth *opsauth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"schema_version": "namros.console.admin.users.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"status":         "ok",
			"users":          auth.Users(),
		})
	}
}

func sessionCSRFToken(auth *opsauth.Manager, r *http.Request) string {
	token, err := auth.CSRFTokenForRequest(r)
	if err != nil {
		return ""
	}
	return token
}

func consoleAdminGroups(auth *opsauth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"schema_version": "namros.console.admin.groups.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"status":         "ok",
			"groups":         auth.Groups(),
		})
	}
}

func consoleAdminRoles() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"schema_version": "namros.console.admin.roles.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"status":         "ok",
			"roles":          opsauth.Roles(),
		})
	}
}
