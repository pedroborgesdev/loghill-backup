package routes

import "github.com/gin-gonic/gin"

func (a *APIRoutes) registerSettingsRoutes(group *gin.RouterGroup) {
	group.GET("/settings", a.controller.GetSettings)
	group.PUT("/settings", a.controller.UpdateSettings)
}
