package middleware

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"logtheater/internal/auth"
	"logtheater/internal/config"
)

var requestCounter atomic.Uint64

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), requestCounter.Add(1))
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func Security() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		policy := "default-src 'self'; connect-src 'self'; img-src 'self' data:; script-src 'self'; style-src 'self' 'unsafe-inline'"
		if strings.HasPrefix(c.Request.URL.Path, "/docs") {
			policy = "default-src 'self'; connect-src 'self'; img-src 'self' data:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'"
		}
		c.Header("Content-Security-Policy", policy)
		c.Next()
	}
}

func BodyLimit(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		}
		c.Next()
	}
}

// Session protects UI/admin API routes with a cookie session.
// Password header X-API-Key remains accepted for non-browser clients.
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
		c.AbortWithStatusJSON(http.StatusUnauthorized, errorBody(c, "UNAUTHORIZED", "Credencial inválida"))
	}
}

// APIKey is retained for compatibility with older call sites/tests.
func APIKey(enabled bool, key string) gin.HandlerFunc {
	return Session(nil, enabled, key)
}

func secureEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func errorBody(c *gin.Context, code, msg string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": msg, "request_id": c.GetString("request_id")}}
}

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
		v := clients[key]
		if now.Sub(v.start) >= cfg.RateWindow {
			v = visitor{start: now}
		}
		v.count++
		clients[key] = v
		blocked := v.count > cfg.RateRequests
		mu.Unlock()
		if blocked {
			c.AbortWithStatusJSON(429, errorBody(c, "RATE_LIMIT_EXCEEDED", "Limite de requisições excedido"))
			return
		}
		c.Next()
	}
}
