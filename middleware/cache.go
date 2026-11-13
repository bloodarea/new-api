package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// hashedStaticAsset reports whether the request targets a content-hashed build
// asset (served under /static/). Those may be cached aggressively because their
// filename changes whenever their content does. Everything else — the SPA
// index.html and any route that falls back to it — must never be cached, or a
// browser keeps loading stale HTML that points at the previous build's chunks.
func hashedStaticAsset(requestURI string) bool {
	return strings.HasPrefix(requestURI, "/static/")
}

func Cache() func(c *gin.Context) {
	return func(c *gin.Context) {
		if hashedStaticAsset(c.Request.RequestURI) {
			c.Header("Cache-Control", "max-age=604800") // one week
		} else {
			c.Header("Cache-Control", "no-store")
			c.Header("Pragma", "no-cache")
		}
		c.Header("Cache-Version", "b688f2fb5be447c25e5aa3bd063087a83db32a288bf6a4f35f2d8db310e40b14")
		c.Next()
	}
}
