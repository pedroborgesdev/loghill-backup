package routes

import (
	"github.com/gin-gonic/gin"

	"logtheater/internal/config"
	"logtheater/internal/controllers"
	"logtheater/internal/middlewares"
)

type APIRoutes struct {
	controller *controllers.APIController
	config     config.Config
}

func New(controller *controllers.APIController, cfg config.Config) *APIRoutes {
	return &APIRoutes{controller: controller, config: cfg}
}

func (a *APIRoutes) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(
		middlewares.RequestID(),
		middlewares.RequestLogger(),
		middlewares.Recovery(),
		middlewares.ErrorHandler(),
		middlewares.Security(),
		middlewares.CORS(a.config),
		middlewares.RateLimit(a.config),
		middlewares.BodyLimit(a.config.MaxBodySize),
	)

	a.registerSystemRoutes(router)
	a.registerAPIRoutes(router)
	router.NoRoute(a.controller.Spa)
	return router
}

func (a *APIRoutes) registerSystemRoutes(router *gin.Engine) {
	router.GET("/health", a.controller.Health)
	router.GET("/ready", a.controller.Ready)
	router.GET("/openapi.yaml", a.controller.OpenAPISpec)
	router.GET("/docs", a.controller.DocsRedirect)
	router.GET("/docs/*any", a.controller.Docs)
}

func (a *APIRoutes) registerAPIRoutes(router *gin.Engine) {
	v1 := router.Group("/api/v1")
	v1.POST("/auth/login", a.controller.Login)
	v1.POST("/auth/logout", a.controller.Logout)
	v1.GET("/auth/session", a.controller.SessionStatus)

	admin := v1.Group("")
	admin.Use(middlewares.Session(a.controller.Sessions(), a.config.AuthEnabled, a.config.AppPassword))
	a.registerSenderRoutes(admin)
	a.registerSettingsRoutes(admin)
	a.registerOptionalRoutes(admin)

	v1.POST("/senders/:sender/health", a.controller.SenderHealth)
	v1.POST("/senders/:sender/instances/init", a.controller.InitSenderInstance)
	v1.POST("/instances/init", a.controller.InitInstanceByKey)
	v1.POST("/logs", a.controller.ReceiveLog)
}

func (a *APIRoutes) registerOptionalRoutes(group *gin.RouterGroup) {
	if a.controller.ExecutionsEnabled() {
		group.GET("/executions", a.controller.ListExecutions)
		group.GET("/executions/:executionID", a.controller.GetExecution)
		group.GET("/dashboard/recent-executions", a.controller.RecentExecutions)
	}
	if a.controller.NotificationsEnabled() {
		a.registerAlertRoutes(group)
	}
	if a.controller.EventsEnabled() {
		a.registerEventRoutes(group)
	}
	if a.controller.MonitoringEnabled() {
		a.registerMonitoringRoutes(group)
	}
}
