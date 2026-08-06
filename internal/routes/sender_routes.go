package routes

import "github.com/gin-gonic/gin"

func (a *APIRoutes) registerSenderRoutes(group *gin.RouterGroup) {
	group.GET("/senders", a.controller.ListSenders)
	group.POST("/senders", a.controller.CreateSender)
	group.GET("/senders/check-id", a.controller.CheckSenderID)
	group.GET("/senders/:sender", a.controller.GetSender)
	group.GET("/senders/:sender/instances", a.controller.ListSenderInstances)
	group.DELETE("/senders/:sender/instances/:instance", a.controller.DeleteSenderInstance)
	group.PUT("/senders/:sender", a.controller.UpdateSender)
	group.GET("/senders/:sender/dependencies", a.controller.SenderDependencies)
	group.POST("/senders/:sender/rotate-key", a.controller.RotateSenderKey)
	group.POST("/senders/:sender/revoke", a.controller.RevokeSender)
	group.POST("/senders/:sender/reactivate", a.controller.ReactivateSender)
	group.DELETE("/senders/:sender", a.controller.DeleteSender)
	group.GET("/senders/:sender/logs", a.controller.Logs)
	group.GET("/senders/:sender/logs/download", a.controller.Download)
	group.GET("/senders/:sender/logs/stream", a.controller.Stream)
	group.GET("/dashboard/summary", a.controller.Summary)
}
