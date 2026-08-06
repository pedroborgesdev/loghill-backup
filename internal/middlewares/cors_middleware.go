package middlewares

import (
	"github.com/gin-gonic/gin"
	"logtheater/internal/config"
)

func CORS(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.CORS {
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		for _, allowed := range cfg.AllowedOrigins {
			if origin == allowed {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Vary", "Origin")
				break
			}
		}
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-API-Key, X-Sender-Key, X-Sender-Instance-ID")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
