package routes

import "github.com/gin-gonic/gin"

func (a *APIRoutes) registerEventRoutes(group *gin.RouterGroup) {
	group.GET("/events", a.controller.ListEvents)
	group.GET("/events/check-key", a.controller.CheckEventKey)
	group.GET("/events/:eventID", a.controller.GetEvent)
	group.POST("/events", a.controller.CreateEvent)
	group.PUT("/events/:eventID", a.controller.UpdateEvent)
	group.PATCH("/events/:eventID/status", a.controller.UpdateEventStatus)
	group.DELETE("/events/:eventID", a.controller.DeleteEvent)
	group.POST("/events/:eventID/test", a.controller.TestEvent)
}
