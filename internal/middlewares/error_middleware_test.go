package middlewares

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestErrorHandlerReturnsCentralizedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), ErrorHandler())
	router.GET("/error", func(c *gin.Context) {
		_ = c.Error(errors.New("failure"))
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/error", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"code":"INTERNAL_ERROR"`) || !strings.Contains(response.Body.String(), `"request_id":"req_`) {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestRecoveryConvertsPanicToErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), Recovery())
	router.GET("/panic", func(*gin.Context) {
		panic("failure")
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}
