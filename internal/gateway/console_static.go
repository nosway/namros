package gateway

import (
	"embed"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nosway/namros/internal/opsauth"
)

//go:embed console_static/*
var consoleStatic embed.FS

func registerConsoleStatic(router *gin.Engine, auth *opsauth.Manager) {
	router.GET("/console", consoleAuthGuard(auth), func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/console/")
	})
	router.GET("/console/", consoleAuthGuard(auth), serveConsoleAsset("index.html", "text/html; charset=utf-8"))
	router.GET("/console/app.js", consoleAuthGuard(auth), serveConsoleAsset("app.js", "text/javascript; charset=utf-8"))
	router.GET("/console/styles.css", consoleAuthGuard(auth), serveConsoleAsset("styles.css", "text/css; charset=utf-8"))
	router.GET("/console/namros-logo.svg", consoleAuthGuard(auth), serveConsoleAsset("namros-logo.svg", "image/svg+xml"))
}

func serveConsoleAsset(name, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := consoleStatic.ReadFile("console_static/" + name)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, contentType, payload)
	}
}
