package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"logtheater/internal/alerts"
	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/emailprovider"
	"logtheater/internal/events"
	"logtheater/internal/middleware"
	"logtheater/internal/notification"
	"logtheater/internal/scheduler"
	"logtheater/internal/service"
)

type API struct {
	svc           *service.Service
	cfg           config.Config
	scheduler     *scheduler.Scheduler
	assets        fs.FS
	alerts        *alerts.Service
	emailConfig   *emailconfig.Store
	emailProvider emailprovider.Provider
	dispatcher    *notification.Dispatcher
	events        *events.Service
}

func (a *API) ConfigureEvents(eventService *events.Service) *API {
	a.events = eventService
	return a
}

func (a *API) ConfigureNotifications(alertsService *alerts.Service, emailSettings *emailconfig.Store, provider emailprovider.Provider, dispatcher *notification.Dispatcher) *API {
	a.alerts = alertsService
	a.emailConfig = emailSettings
	a.emailProvider = provider
	a.dispatcher = dispatcher
	return a
}

func New(svc *service.Service, cfg config.Config, s *scheduler.Scheduler, assets fs.FS) *API {
	return &API{svc: svc, cfg: cfg, scheduler: s, assets: assets}
}
func (a *API) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.RequestID(), gin.Logger(), gin.Recovery(), middleware.Security(), middleware.CORS(a.cfg), middleware.RateLimit(a.cfg), middleware.BodyLimit(a.cfg.MaxBodySize))
	r.GET("/health", a.health)
	r.GET("/ready", a.ready)
	r.GET("/openapi.yaml", func(c *gin.Context) { c.File("./docs/openapi.yaml") })
	r.GET("/docs", func(c *gin.Context) { c.Redirect(http.StatusTemporaryRedirect, "/docs/index.html") })
	r.GET("/docs/*any", ginSwagger.WrapHandler(
		swaggerFiles.Handler,
		ginSwagger.URL("/openapi.yaml"),
		ginSwagger.PersistAuthorization(true),
	))
	v1 := r.Group("/api/v1")
	read := v1.Group("")
	read.Use(middleware.APIKey(a.cfg.AdminAuthEnabled, a.cfg.AdminAPIKey))
	read.GET("/senders", a.listSenders)
	read.POST("/senders", a.createSender)
	read.GET("/senders/check-id", a.checkSenderID)
	read.GET("/senders/:sender", a.getSender)
	read.PUT("/senders/:sender", a.updateSender)
	read.GET("/senders/:sender/dependencies", a.senderDependencies)
	read.POST("/senders/:sender/rotate-key", a.rotateSenderKey)
	read.POST("/senders/:sender/revoke", a.revokeSender)
	read.POST("/senders/:sender/reactivate", a.reactivateSender)
	read.DELETE("/senders/:sender", a.deleteSender)
	read.GET("/senders/:sender/logs", a.logs)
	read.GET("/senders/:sender/logs/download", a.download)
	read.GET("/senders/:sender/logs/stream", a.stream)
	read.GET("/dashboard/summary", a.summary)
	read.GET("/settings", a.getSettings)
	read.PUT("/settings", a.updateSettings)
	if a.alerts != nil && a.emailConfig != nil && a.emailProvider != nil && a.dispatcher != nil {
		read.GET("/alerts", a.listAlerts)
		read.GET("/alerts/:alertID", a.getAlert)
		read.POST("/alerts", a.createAlert)
		read.PUT("/alerts/:alertID", a.updateAlert)
		read.DELETE("/alerts/:alertID", a.deleteAlert)
		read.PATCH("/alerts/:alertID/status", a.updateAlertStatus)
		read.POST("/alerts/:alertID/test", a.testAlert)
		read.GET("/settings/email", a.getEmailSettings)
		read.PUT("/settings/email", a.updateEmailSettings)
		read.POST("/settings/email/test-connection", a.testEmailConnection)
		read.POST("/settings/email/send-test", a.sendTestEmail)
	}
	if a.events != nil && a.emailConfig != nil && a.emailProvider != nil && a.dispatcher != nil {
		read.GET("/events", a.listEvents)
		read.GET("/events/check-key", a.checkEventKey)
		read.GET("/events/:eventID", a.getEvent)
		read.POST("/events", a.createEvent)
		read.PUT("/events/:eventID", a.updateEvent)
		read.PATCH("/events/:eventID/status", a.updateEventStatus)
		read.DELETE("/events/:eventID", a.deleteEvent)
		read.POST("/events/:eventID/test", a.testEvent)
	}
	v1.POST("/senders/:sender/health", a.senderHealth)
	v1.POST("/logs", a.receiveLog)
	r.NoRoute(a.spa)
	return r
}
func errBody(c *gin.Context, code, msg string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": msg, "request_id": c.GetString("request_id")}}
}
func (a *API) fail(c *gin.Context, err error) {
	status, code, msg := 500, "INTERNAL_ERROR", "Erro interno"
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code, msg = 404, "SENDER_NOT_FOUND", "Sender não encontrado"
	case errors.Is(err, domain.ErrExpired):
		status, code, msg = 409, "SENDER_EXPIRED", "Sender expirado; registre um novo sender"
	case errors.Is(err, domain.ErrLogFileNotFound):
		status, code, msg = 410, "LOG_FILE_NOT_FOUND", "Arquivo de log não está mais disponível"
	case errors.Is(err, domain.ErrInvalidSeverity):
		status, code, msg = 422, "INVALID_SEVERITY", "Severity inválida"
	case errors.Is(err, domain.ErrInvalidEventKey):
		c.AbortWithStatusJSON(http.StatusBadRequest, errorBodyWithField(c, "INVALID_EVENT_KEY", "O identificador do evento é inválido.", "event"))
		return
	case errors.Is(err, domain.ErrInvalidEventOccurrenceID):
		c.AbortWithStatusJSON(http.StatusBadRequest, errorBodyWithField(c, "INVALID_EVENT_OCCURRENCE_ID", "O identificador da ocorrência é inválido.", "event_occurrence_id"))
		return
	case errors.Is(err, domain.ErrInvalidName):
		status, code, msg = 422, "INVALID_SENDER_NAME", "Nome de sender inválido"
	case errors.Is(err, domain.ErrSenderAlreadyExists):
		status, code, msg = 409, "SENDER_ALREADY_EXISTS", "Já existe um sender com este identificador."
	case errors.Is(err, domain.ErrInvalidSenderKey):
		status, code, msg = 401, "INVALID_SENDER_KEY", "A chave informada não é válida para este sender."
	case errors.Is(err, domain.ErrSenderRevoked):
		status, code, msg = 409, "SENDER_REVOKED", "O sender está revogado. Gere uma nova chave ao reativá-lo."
	case errors.Is(err, domain.ErrConflict):
		status, code, msg = 409, "CONFLICT", "A operação não pode ser concluída no estado atual."
	case errors.Is(err, domain.ErrTooManySubscribers):
		status, code, msg = 429, "RATE_LIMIT_EXCEEDED", "Limite de streams atingido"
	case errors.Is(err, contextCanceled):
		status, code, msg = 408, "TIMEOUT", "Operação cancelada"
	}
	var validation *service.SenderValidationError
	if errors.As(err, &validation) {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, errorBodyWithField(c, "INVALID_SENDER", validation.Message, validation.Field))
		return
	}
	c.AbortWithStatusJSON(status, errBody(c, code, msg))
}

var contextCanceled = errors.New("context canceled")

func errorBodyWithField(c *gin.Context, code, message, field string) gin.H {
	value := gin.H{"code": code, "message": message, "request_id": c.GetString("request_id")}
	if field != "" {
		value["field"] = field
	}
	return gin.H{"error": value}
}

func (a *API) createSender(c *gin.Context) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeOne(c, &in) {
		return
	}
	s, credentials, err := a.svc.CreateSender(c.Request.Context(), in.Name, in.Description)
	if err != nil {
		if errors.Is(err, domain.ErrSenderAlreadyExists) {
			id, _ := service.NormalizeName(in.Name)
			c.JSON(http.StatusConflict, errorBodyWithField(c, "SENDER_ALREADY_EXISTS", fmt.Sprintf("Já existe um sender com o identificador %s.", id), "name"))
			return
		}
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"sender": s, "credentials": credentials})
}

func (a *API) checkSenderID(c *gin.Context) {
	id, err := service.NormalizeName(c.Query("id"))
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, errorBodyWithField(c, "INVALID_SENDER_ID", "O identificador informado é inválido.", "id"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "available": a.svc.SenderIDAvailable(c.Request.Context(), id)})
}

func (a *API) updateSender(c *gin.Context) {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeOne(c, &input) {
		return
	}
	item, err := a.svc.UpdateSender(c.Request.Context(), c.Param("sender"), input.Name, input.Description)
	if err != nil {
		a.fail(c, err)
		return
	}
	if a.alerts != nil {
		if err = a.alerts.RenameSender(item.ID, item.Name); err != nil {
			a.failAlert(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, item)
}

func (a *API) rotateSenderKey(c *gin.Context) {
	item, credentials, rotatedAt, err := a.svc.RotateSenderKey(c.Request.Context(), c.Param("sender"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"sender_id": item.ID, "credentials": credentials, "rotated_at": rotatedAt})
}

func (a *API) revokeSender(c *gin.Context) {
	item, err := a.svc.RevokeSender(c.Request.Context(), c.Param("sender"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (a *API) reactivateSender(c *gin.Context) {
	item, credentials, err := a.svc.ReactivateSender(c.Request.Context(), c.Param("sender"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"sender": item, "credentials": credentials})
}

func (a *API) senderDependencies(c *gin.Context) {
	count, eventCount := 0, 0
	if a.alerts != nil {
		count = a.alerts.SenderUsageCount(c.Param("sender"))
	}
	if a.events != nil {
		eventCount = a.events.SenderUsageCount(c.Param("sender"))
	}
	c.JSON(http.StatusOK, gin.H{"sender_id": c.Param("sender"), "alert_rules": count, "events": eventCount})
}

func (a *API) deleteSender(c *gin.Context) {
	id := c.Param("sender")
	usage := 0
	eventUsage := 0
	if a.alerts != nil {
		usage = a.alerts.SenderUsageCount(id)
	}
	if a.events != nil {
		eventUsage = a.events.SenderUsageCount(id)
	}
	if (usage > 0 && c.Query("remove_from_alerts") != "true") || (eventUsage > 0 && c.Query("remove_from_events") != "true") {
		code := "SENDER_HAS_DEPENDENCIES"
		message := fmt.Sprintf("Este sender está associado a %d regras de alerta e %d eventos.", usage, eventUsage)
		if eventUsage == 0 {
			code = "SENDER_HAS_ALERTS"
			message = fmt.Sprintf("Este sender está associado a %d regras de alerta.", usage)
		} else if usage == 0 {
			code = "SENDER_HAS_EVENTS"
			message = fmt.Sprintf("Este sender está associado a %d eventos.", eventUsage)
		}
		body := errorBodyWithField(c, code, message, "")
		body["alert_rules"] = usage
		body["events"] = eventUsage
		c.JSON(http.StatusConflict, body)
		return
	}
	if usage > 0 {
		if _, err := a.alerts.RemoveSender(id); err != nil {
			a.failAlert(c, err)
			return
		}
	}
	if eventUsage > 0 {
		if _, err := a.events.RemoveSender(id); err != nil {
			a.failEvent(c, err)
			return
		}
	}
	if err := a.svc.DeleteSender(c.Request.Context(), id); err != nil {
		a.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (a *API) receiveLog(c *gin.Context) {
	var in struct {
		Sender            string         `json:"sender" binding:"required"`
		Severity          string         `json:"severity" binding:"required"`
		Message           string         `json:"message" binding:"required"`
		Timestamp         *time.Time     `json:"timestamp"`
		Event             string         `json:"event"`
		EventOccurrenceID string         `json:"event_occurrence_id"`
		Metadata          map[string]any `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, errBody(c, "INVALID_REQUEST", "Body inválido"))
		return
	}
	_, at, err := a.svc.ReceiveLogWithEvent(c.Request.Context(), in.Sender, c.GetHeader("X-Sender-Key"), in.Severity, in.Message, in.Event, in.EventOccurrenceID, in.Timestamp, in.Metadata)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(202, gin.H{"accepted": true, "sender": in.Sender, "received_at": at})
}
func (a *API) senderHealth(c *gin.Context) {
	var body map[string]any
	if c.Request.ContentLength > 0 && c.ShouldBindJSON(&body) != nil {
		c.JSON(400, errBody(c, "INVALID_REQUEST", "Body inválido"))
		return
	}
	s, at, err := a.svc.Health(c.Request.Context(), c.Param("sender"), c.GetHeader("X-Sender-Key"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"sender": s.ID, "status": s.Status, "received_at": at})
}
func positive(c *gin.Context, key string, def, max int) int {
	v, err := strconv.Atoi(c.DefaultQuery(key, strconv.Itoa(def)))
	if err != nil || v < 1 {
		return def
	}
	if v > max {
		return max
	}
	return v
}
func (a *API) listSenders(c *gin.Context) {
	f := domain.SenderFilters{Status: domain.SenderStatus(c.Query("status")), Name: c.Query("name"), Search: c.Query("search"), HasErrors: c.Query("has_errors") == "true", GroupByName: c.Query("group_by") == "name", Sort: c.DefaultQuery("sort", "last_activity_at"), Order: c.DefaultQuery("order", "desc"), Page: positive(c, "page", 1, 1_000_000), PageSize: positive(c, "page_size", 20, a.cfg.MaxPageSize)}
	page, err := a.svc.Senders(c.Request.Context(), f)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, page)
}
func (a *API) getSender(c *gin.Context) {
	s, err := a.svc.Get(c.Request.Context(), c.Param("sender"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, s)
}
func parseLogFilters(c *gin.Context, max int) domain.LogFilters {
	f := domain.LogFilters{Severities: map[domain.LogSeverity]bool{}, Search: c.Query("search"), EventMode: c.Query("event"), EventKey: c.Query("event_key"), Page: positive(c, "page", 1, 1_000_000), PageSize: positive(c, "page_size", 100, max), Order: c.DefaultQuery("order", "desc")}
	for _, raw := range strings.Split(c.Query("severity"), ",") {
		if sev, e := domain.ParseSeverity(raw); e == nil {
			f.Severities[sev] = true
		}
	}
	if t, e := time.Parse(time.RFC3339, c.Query("start_date")); e == nil {
		f.Start = &t
	}
	if t, e := time.Parse(time.RFC3339, c.Query("end_date")); e == nil {
		f.End = &t
	}
	return f
}
func (a *API) logs(c *gin.Context) {
	page, err := a.svc.Logs(c.Request.Context(), c.Param("sender"), parseLogFilters(c, a.cfg.MaxPageSize))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, page)
}
func (a *API) download(c *gin.Context) {
	f := parseLogFilters(c, a.cfg.MaxLogLines)
	f.Page = 1
	f.PageSize = a.cfg.MaxLogLines
	page, err := a.svc.Logs(c.Request.Context(), c.Param("sender"), f)
	if err != nil {
		a.fail(c, err)
		return
	}
	format := c.DefaultQuery("format", "jsonl")
	if format != "txt" && format != "jsonl" {
		c.JSON(422, errBody(c, "INVALID_REQUEST", "Formato inválido"))
		return
	}
	filename := url.PathEscape(c.Param("sender") + "-logs." + format)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	w := bufio.NewWriter(c.Writer)
	defer w.Flush()
	for _, e := range page.Items {
		b, _ := json.Marshal(e)
		_, _ = w.Write(append(b, '\n'))
	}
}
func (a *API) stream(c *gin.Context) {
	id := c.Param("sender")
	if _, err := a.svc.Get(c.Request.Context(), id); err != nil {
		a.fail(c, err)
		return
	}
	ch, cancel, err := a.svc.Hub.Subscribe(id)
	if err != nil {
		a.fail(c, err)
		return
	}
	defer cancel()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(500, errBody(c, "INTERNAL_ERROR", "Streaming indisponível"))
		return
	}
	writeEvent(c.Writer, "status", gin.H{"status": "connected"})
	flusher.Flush()
	ticker := time.NewTicker(a.cfg.SSEHeartbeat)
	defer ticker.Stop()
	filters := parseLogFilters(c, a.cfg.MaxPageSize)
	allowed := filters.Severities
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case e, open := <-ch:
			if !open {
				return
			}
			if len(allowed) > 0 && !allowed[e.Severity] {
				continue
			}
			if filters.EventMode == "with" && e.Event == "" || filters.EventMode == "without" && e.Event != "" || filters.EventKey != "" && e.Event != filters.EventKey {
				continue
			}
			writeEvent(c.Writer, "log", e)
			flusher.Flush()
		case t := <-ticker.C:
			writeEvent(c.Writer, "heartbeat", gin.H{"time": t})
			flusher.Flush()
		}
	}
}
func writeEvent(w http.ResponseWriter, event string, v any) {
	b, _ := json.Marshal(v)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
}
func (a *API) summary(c *gin.Context) {
	v, err := a.svc.Summary(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, v)
}

func (a *API) getSettings(c *gin.Context) {
	c.JSON(http.StatusOK, a.svc.Settings())
}

func (a *API) updateSettings(c *gin.Context) {
	var input struct {
		LogLimit             *domain.NumberUnitValue `json:"log_limit"`
		InactivePreservation *domain.NumberUnitValue `json:"inactive_preservation"`
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		c.JSON(http.StatusBadRequest, settingsErrorBody(c, "Configurações inválidas ou incompletas.", ""))
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, settingsErrorBody(c, "O body deve conter um único objeto JSON.", ""))
		return
	}
	if input.LogLimit == nil || input.InactivePreservation == nil {
		c.JSON(http.StatusBadRequest, settingsErrorBody(c, "Configurações inválidas ou incompletas.", ""))
		return
	}
	updated, err := a.svc.UpdateSettings(domain.Settings{
		LogLimit:             *input.LogLimit,
		InactivePreservation: *input.InactivePreservation,
	})
	if err != nil {
		var validation *domain.SettingsValidationError
		if errors.As(err, &validation) {
			c.JSON(http.StatusUnprocessableEntity, settingsErrorBody(c, validation.Message, validation.Field))
			return
		}
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"settings": gin.H{
			"log_limit":             updated.LogLimit,
			"inactive_preservation": updated.InactivePreservation,
		},
		"updated_at": updated.UpdatedAt,
	})
}

func settingsErrorBody(c *gin.Context, message, field string) gin.H {
	errorValue := gin.H{
		"code":       "INVALID_SETTINGS",
		"message":    message,
		"request_id": c.GetString("request_id"),
	}
	if field != "" {
		errorValue["field"] = field
	}
	return gin.H{"error": errorValue}
}
func (a *API) health(c *gin.Context) {
	sum, err := a.svc.Summary(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "healthy", "time": time.Now(), "uptime_seconds": int64(a.svc.Uptime().Seconds()), "senders": sum["senders"], "storage": gin.H{"writable": true, "path": a.cfg.DataDir}})
}
func (a *API) ready(c *gin.Context) {
	if !a.scheduler.Ready() {
		c.JSON(503, gin.H{"status": "not_ready"})
		return
	}
	c.JSON(200, gin.H{"status": "ready", "time": time.Now()})
}
func (a *API) spa(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/health" || c.Request.URL.Path == "/ready" {
		c.JSON(404, errBody(c, "NOT_FOUND", "Rota não encontrada"))
		return
	}
	path := strings.TrimPrefix(c.Request.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := a.assets.Open(path); err == nil {
		_ = f.Close()
		http.FileServer(http.FS(a.assets)).ServeHTTP(c.Writer, c.Request)
		return
	}
	b, err := fs.ReadFile(a.assets, "index.html")
	if err != nil {
		c.String(500, "frontend unavailable")
		return
	}
	c.Data(200, "text/html; charset=utf-8", b)
}
