package routes

import "github.com/gin-gonic/gin"

func (a *APIRoutes) registerMonitoringRoutes(group *gin.RouterGroup) {
	group.GET("/monitoring/rules", a.controller.ListMonitoringRules)
	group.GET("/monitoring/rules/:ruleID", a.controller.GetMonitoringRule)
	group.POST("/monitoring/rules", a.controller.CreateMonitoringRule)
	group.PUT("/monitoring/rules/:ruleID", a.controller.UpdateMonitoringRule)
	group.PATCH("/monitoring/rules/:ruleID/status", a.controller.UpdateMonitoringRuleStatus)
	group.DELETE("/monitoring/rules/:ruleID", a.controller.DeleteMonitoringRule)
	group.POST("/monitoring/rules/:ruleID/duplicate", a.controller.DuplicateMonitoringRule)
	group.POST("/monitoring/rules/:ruleID/test", a.controller.TestMonitoringRule)
	group.POST("/monitoring/rules/validate", a.controller.ValidateMonitoringRule)
	group.GET("/monitoring/rules/:ruleID/executions", a.controller.ListMonitoringExecutions)
	group.GET("/monitoring/executions/:executionID", a.controller.GetMonitoringExecution)
}
