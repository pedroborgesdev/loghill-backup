package middlewares

import (
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"logtheater/internal/config"
)

type visitor struct {
	start time.Time
	count int
}

func RateLimit(cfg config.Config) gin.HandlerFunc {
	var mu sync.Mutex
	clients := map[string]visitor{}
	return func(c *gin.Context) {
		if !cfg.RateLimit {
			c.Next()
			return
		}
		key := strings.Split(c.Request.RemoteAddr, ":")[0]
		now := time.Now()
		mu.Lock()
		current := clients[key]
		if now.Sub(current.start) >= cfg.RateWindow {
			current = visitor{start: now}
		}
		current.count++
		clients[key] = current
		blocked := current.count > cfg.RateRequests
		mu.Unlock()
		if blocked {
			c.AbortWithStatusJSON(429, errorBody(c, "RATE_LIMIT_EXCEEDED", "Limite de requisições excedido"))
			return
		}
		c.Next()
	}
}
