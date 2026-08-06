package routes

import "github.com/gin-gonic/gin"

func (a *APIRoutes) registerAlertRoutes(group *gin.RouterGroup) {
	group.GET("/alerts", a.controller.ListAlerts)
	group.GET("/alerts/:alertID", a.controller.GetAlert)
	group.POST("/alerts", a.controller.CreateAlert)
	group.PUT("/alerts/:alertID", a.controller.UpdateAlert)
	group.DELETE("/alerts/:alertID", a.controller.DeleteAlert)
	group.PATCH("/alerts/:alertID/status", a.controller.UpdateAlertStatus)
	group.POST("/alerts/:alertID/test", a.controller.TestAlert)
	group.GET("/settings/email", a.controller.GetEmailSettings)
	group.PUT("/settings/email", a.controller.UpdateEmailSettings)
	group.POST("/settings/email/test-connection", a.controller.TestEmailConnection)
	group.POST("/settings/email/send-test", a.controller.SendTestEmail)
}
