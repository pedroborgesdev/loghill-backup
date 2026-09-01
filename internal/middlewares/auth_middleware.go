package middlewares

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
	"logtheater/internal/auth"
)

func Session(manager *auth.Manager, enabled bool, password string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		if manager != nil && manager.Valid(auth.TokenFromRequest(c.Request)) {
			c.Next()
			return
		}
		if password != "" && secureEqual(c.GetHeader("X-API-Key"), password) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, errorBody(c, "UNAUTHORIZED", "Invalid credential"))
	}
}

func APIKey(enabled bool, key string) gin.HandlerFunc {
	return Session(nil, enabled, key)
}

func secureEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
