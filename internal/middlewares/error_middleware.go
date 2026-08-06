package middlewares

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		err := c.Errors.Last().Err
		slog.Error("unhandled request error",
			"request_id", c.GetString("request_id"),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"error", err,
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, errorBody(c, "INTERNAL_ERROR", "Erro interno"))
	}
}
