package middlewares

import "github.com/gin-gonic/gin"

func errorBody(c *gin.Context, code, message string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": message, "request_id": c.GetString("request_id")}}
}
