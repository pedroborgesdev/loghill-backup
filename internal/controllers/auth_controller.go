package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"logtheater/internal/auth"
)

func (a *APIController) Login(c *gin.Context) {
	if a.sessions == nil || !a.cfg.AuthEnabled {
		c.JSON(http.StatusOK, gin.H{"authenticated": true, "auth_required": false})
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, errBody(c, "INVALID_REQUEST", "Invalid request body"))
		return
	}
	if !a.sessions.Authenticate(strings.TrimSpace(input.Password)) {
		c.JSON(http.StatusUnauthorized, errBody(c, "INVALID_CREDENTIALS", "Invalid password"))
		return
	}
	session, err := a.sessions.Create()
	if err != nil {
		c.JSON(http.StatusInternalServerError, errBody(c, "INTERNAL_ERROR", "Unable to create the session"))
		return
	}
	a.sessions.SetCookie(c.Writer, session)
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "auth_required": true, "expires_at": session.ExpiresAt})
}

func (a *APIController) Logout(c *gin.Context) {
	if a.sessions != nil {
		a.sessions.Revoke(auth.TokenFromRequest(c.Request))
		a.sessions.ClearCookie(c.Writer)
	}
	c.JSON(http.StatusOK, gin.H{"authenticated": false})
}

func (a *APIController) SessionStatus(c *gin.Context) {
	if !a.cfg.AuthEnabled || a.sessions == nil {
		c.JSON(http.StatusOK, gin.H{"authenticated": true, "auth_required": false})
		return
	}
	authenticated := a.sessions.Valid(auth.TokenFromRequest(c.Request))
	if !authenticated && a.cfg.AppPassword != "" && secureHeaderEqual(c.GetHeader("X-API-Key"), a.cfg.AppPassword) {
		authenticated = true
	}
	c.JSON(http.StatusOK, gin.H{"authenticated": authenticated, "auth_required": true})
}

func secureHeaderEqual(a, b string) bool {
	return len(a) == len(b) && subtleCompare(a, b)
}

func subtleCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
