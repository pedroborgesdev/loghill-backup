package controllers

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
	"logtheater/internal/alerts"
	"logtheater/internal/app"
	"logtheater/internal/auth"
	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/dto"
	"logtheater/internal/events"
	"logtheater/internal/executions"
	"logtheater/internal/monitoring"
	"logtheater/internal/scheduler"
	"logtheater/internal/services"
)

type APIController struct {
	svc           *services.SenderService
	cfg           config.Config
	scheduler     *scheduler.Scheduler
	assets        fs.FS
	sessions      *auth.Manager
	lifecycle     *app.SenderLifecycle
	alerts        *alerts.Service
	notifications *services.NotificationService
	events        *events.Service
	monitoring    *monitoring.Service
	executions    *executions.Store
}

func (a *APIController) ConfigureExecutions(store *executions.Store) *APIController {
	a.executions = store
	return a
}

func (a *APIController) ConfigureEvents(eventService *events.Service) *APIController {
	a.events = eventService
	a.ensureLifecycle()
	return a
}

func (a *APIController) ConfigureMonitoring(service *monitoring.Service) *APIController {
	a.monitoring = service
	a.ensureLifecycle()
	return a
}

func (a *APIController) ConfigureNotifications(alertsService *alerts.Service, notificationService *services.NotificationService) *APIController {
	a.alerts = alertsService
	a.notifications = notificationService
	a.ensureLifecycle()
	return a
}

func (a *APIController) ConfigureAuth(sessions *auth.Manager) *APIController {
	a.sessions = sessions
	return a
}

func (a *APIController) ensureLifecycle() {
	a.lifecycle = &app.SenderLifecycle{
		Senders:    a.svc,
		Alerts:     a.alerts,
		Events:     a.events,
		Monitoring: a.monitoring,
	}
}

func New(svc *services.SenderService, cfg config.Config, s *scheduler.Scheduler, assets fs.FS) *APIController {
	api := &APIController{svc: svc, cfg: cfg, scheduler: s, assets: assets}
	api.ensureLifecycle()
	return api
}
func (a *APIController) Sessions() *auth.Manager { return a.sessions }

func (a *APIController) ExecutionsEnabled() bool { return a.executions != nil }

func (a *APIController) NotificationsEnabled() bool {
	return a.alerts != nil && a.notifications != nil
}

func (a *APIController) EventsEnabled() bool {
	return a.events != nil && a.notifications != nil
}

func (a *APIController) MonitoringEnabled() bool { return a.monitoring != nil }
func errBody(c *gin.Context, code, msg string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": msg, "request_id": c.GetString("request_id")}}
}
func (a *APIController) fail(c *gin.Context, err error) {
	status, code, msg := 500, "INTERNAL_ERROR", "Erro interno"
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code, msg = 404, "SENDER_NOT_FOUND", "Sender not encontrado"
	case errors.Is(err, domain.ErrExpired):
		status, code, msg = 409, "SENDER_EXPIRED", "Sender expired; register a new sender"
	case errors.Is(err, domain.ErrLogFileNotFound):
		status, code, msg = 410, "LOG_FILE_NOT_FOUND", "The log file is no longer available"
	case errors.Is(err, domain.ErrInvalidSeverity):
		status, code, msg = 422, "INVALID_SEVERITY", "Invalid severity"
	case errors.Is(err, domain.ErrInvalidEventKey):
		c.AbortWithStatusJSON(http.StatusBadRequest, errorBodyWithField(c, "INVALID_EVENT_KEY", "The event identifier is invalid.", "event"))
		return
	case errors.Is(err, domain.ErrInvalidEventOccurrenceID):
		c.AbortWithStatusJSON(http.StatusBadRequest, errorBodyWithField(c, "INVALID_EVENT_OCCURRENCE_ID", "The occurrence identifier is invalid.", "event_occurrence_id"))
		return
	case errors.Is(err, domain.ErrInvalidName):
		status, code, msg = 422, "INVALID_SENDER_NAME", "Invalid sender name"
	case errors.Is(err, domain.ErrSenderAlreadyExists):
		status, code, msg = 409, "SENDER_ALREADY_EXISTS", "A sender with this identifier already exists."
	case errors.Is(err, domain.ErrInvalidSenderKey):
		status, code, msg = 401, "INVALID_SENDER_KEY", "The provided key is not valid for this sender."
	case errors.Is(err, domain.ErrInvalidInstanceToken):
		status, code, msg = 401, "INVALID_INSTANCE_TOKEN", "The instance credential is invalid. Initialize a new instance."
	case errors.Is(err, domain.ErrSenderRevoked):
		status, code, msg = 409, "SENDER_REVOKED", "The sender is revoked. Generate a new key when reactivating it."
	case errors.Is(err, domain.ErrConflict):
		status, code, msg = 409, "CONFLICT", "The operation cannot be completed in its current state."
	case errors.Is(err, domain.ErrTooManySubscribers):
		status, code, msg = 429, "RATE_LIMIT_EXCEEDED", "Stream limit reached"
	case errors.Is(err, contextCanceled):
		status, code, msg = 408, "TIMEOUT", "Operation canceled"
	}
	var validation *services.SenderValidationError
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

func (a *APIController) CreateSender(c *gin.Context) {
	var in dto.CreateSenderRequest
	if !decodeOne(c, &in) {
		return
	}
	s, credentials, err := a.svc.CreateSender(c.Request.Context(), in.Name, in.Description)
	if err != nil {
		if errors.Is(err, domain.ErrSenderAlreadyExists) {
			id, _ := services.NormalizeName(in.Name)
			c.JSON(http.StatusConflict, errorBodyWithField(c, "SENDER_ALREADY_EXISTS", fmt.Sprintf("A sender with identifier %s already exists.", id), "name"))
			return
		}
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"sender": s, "credentials": credentials})
}

func (a *APIController) CheckSenderID(c *gin.Context) {
	id, err := services.NormalizeName(c.Query("id"))
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, errorBodyWithField(c, "INVALID_SENDER_ID", "The provided identifier is invalid.", "id"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "available": a.svc.SenderIDAvailable(c.Request.Context(), id)})
}

func (a *APIController) UpdateSender(c *gin.Context) {
	var input dto.UpdateSenderRequest
	if !decodeOne(c, &input) {
		return
	}
	item, err := a.lifecycle.Update(c.Request.Context(), c.Param("sender"), input.Name, input.Description)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (a *APIController) RotateSenderKey(c *gin.Context) {
	item, credentials, rotatedAt, err := a.svc.RotateSenderKey(c.Request.Context(), c.Param("sender"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"sender_id": item.ID, "credentials": credentials, "rotated_at": rotatedAt})
}

func (a *APIController) RevokeSender(c *gin.Context) {
	item, err := a.svc.RevokeSender(c.Request.Context(), c.Param("sender"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (a *APIController) ReactivateSender(c *gin.Context) {
	item, credentials, err := a.svc.ReactivateSender(c.Request.Context(), c.Param("sender"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"sender": item, "credentials": credentials})
}

func (a *APIController) SenderDependencies(c *gin.Context) {
	deps := a.lifecycle.Dependencies(c.Param("sender"))
	c.JSON(http.StatusOK, gin.H{"sender_id": c.Param("sender"), "alert_rules": deps.AlertRules, "events": deps.Events, "monitoring_rules": deps.MonitoringRules})
}

func (a *APIController) DeleteSender(c *gin.Context) {
	err := a.lifecycle.Delete(c.Request.Context(), c.Param("sender"), app.DeleteSenderOptions{
		RemoveFromAlerts:     c.Query("remove_from_alerts") == "true",
		RemoveFromEvents:     c.Query("remove_from_events") == "true",
		RemoveFromMonitoring: c.Query("remove_from_monitoring") == "true",
	})
	if err != nil {
		var conflict *app.DependencyConflictError
		if errors.As(err, &conflict) {
			body := errorBodyWithField(c, conflict.Code, conflict.Message, "")
			body["alert_rules"] = conflict.Dependencies.AlertRules
			body["events"] = conflict.Dependencies.Events
			body["monitoring_rules"] = conflict.Dependencies.MonitoringRules
			c.JSON(http.StatusConflict, body)
			return
		}
		a.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (a *APIController) ReceiveLog(c *gin.Context) {
	var in dto.ReceiveLogRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, errBody(c, "INVALID_REQUEST", "Invalid request body"))
		return
	}
	senderID := strings.TrimSpace(in.SenderID)
	if senderID == "" {
		senderID = strings.TrimSpace(in.Sender)
	}
	if senderID == "" {
		c.JSON(http.StatusBadRequest, errorBodyWithField(c, "INVALID_REQUEST", "Informe sender_id.", "sender_id"))
		return
	}
	instanceID := strings.TrimSpace(c.GetHeader("X-Sender-Instance-ID"))
	if instanceID == "" {
		instanceID = strings.TrimSpace(c.Query("instance_id"))
	}
	originInstanceID := strings.TrimSpace(c.GetHeader("X-LogHill-Origin-Instance-ID"))
	if originInstanceID == "" {
		originInstanceID = instanceID
	}
	_, at, err := a.svc.ReceiveLogWithAuthenticatedInstanceAndEvent(c.Request.Context(), senderID, c.GetHeader("X-Sender-Key"), instanceID, strings.TrimSpace(c.GetHeader("X-Sender-Instance-Token")), originInstanceID, in.Severity, in.Message, in.Event, in.EventOccurrenceID, in.Timestamp, in.Metadata)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(202, gin.H{"accepted": true, "sender_id": senderID, "sender": senderID, "instance_id": originInstanceID, "received_at": at})
}

func (a *APIController) InitSenderInstance(c *gin.Context) {
	instance, err := a.svc.InitInstance(c.Request.Context(), c.Param("sender"), c.GetHeader("X-Sender-Key"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"sender_id": c.Param("sender"), "sender": c.Param("sender"), "instance_id": instance.ID, "initialized_at": instance.CreatedAt})
}

func (a *APIController) InitInstanceByKey(c *gin.Context) {
	var in dto.InitInstanceRequest
	if err := c.ShouldBindJSON(&in); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, errBody(c, "INVALID_REQUEST", "Invalid request body"))
		return
	}
	if strings.TrimSpace(in.SenderName) != "" {
		sender, instance, token, err := a.svc.InitInstanceByName(c.Request.Context(), in.SenderName)
		if err != nil {
			a.fail(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"sender_id": sender.ID, "sender": sender.ID, "instance_id": instance.ID, "instance_token": token, "initialized_at": instance.CreatedAt})
		return
	}
	sender, instance, err := a.svc.InitInstanceByKey(c.Request.Context(), c.GetHeader("X-Sender-Key"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"sender_id": sender.ID, "sender": sender.ID, "instance_id": instance.ID, "initialized_at": instance.CreatedAt})
}

func (a *APIController) ListSenderInstances(c *gin.Context) {
	items, err := a.svc.Instances(c.Request.Context(), c.Param("sender"))
	if err != nil {
		a.fail(c, err)
		return
	}
	page, pageSize := positive(c, "page", 1, 1_000_000), positive(c, "page_size", 20, a.cfg.MaxPageSize)
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	c.JSON(http.StatusOK, domain.SenderInstancePage{Sender: c.Param("sender"), Items: items[start:end], Pagination: domain.Pagination{Page: page, PageSize: pageSize, Returned: end - start, Total: int64(total), TotalPages: totalPages}})
}

func (a *APIController) DeleteSenderInstance(c *gin.Context) {
	if err := a.svc.DeleteInstance(c.Request.Context(), c.Param("sender"), c.Param("instance")); err != nil {
		a.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (a *APIController) SenderHealth(c *gin.Context) {
	var body map[string]any
	if c.Request.ContentLength > 0 && c.ShouldBindJSON(&body) != nil {
		c.JSON(400, errBody(c, "INVALID_REQUEST", "Invalid request body"))
		return
	}
	s, at, err := a.svc.HealthWithInstanceAuth(c.Request.Context(), c.Param("sender"), c.GetHeader("X-Sender-Key"), strings.TrimSpace(c.GetHeader("X-Sender-Instance-ID")), strings.TrimSpace(c.GetHeader("X-Sender-Instance-Token")))
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
func (a *APIController) ListSenders(c *gin.Context) {
	f := domain.SenderFilters{Status: domain.SenderStatus(c.Query("status")), Name: c.Query("name"), Search: c.Query("search"), HasErrors: c.Query("has_errors") == "true", GroupByName: c.Query("group_by") == "name", Sort: c.DefaultQuery("sort", "last_activity_at"), Order: c.DefaultQuery("order", "desc"), Page: positive(c, "page", 1, 1_000_000), PageSize: positive(c, "page_size", 20, a.cfg.MaxPageSize)}
	page, err := a.svc.Senders(c.Request.Context(), f)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, page)
}
func (a *APIController) GetSender(c *gin.Context) {
	s, err := a.svc.Get(c.Request.Context(), c.Param("sender"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, s)
}
func parseLogFilters(c *gin.Context, max int) domain.LogFilters {
	f := domain.LogFilters{Severities: map[domain.LogSeverity]bool{}, Search: c.Query("search"), InstanceID: c.Query("instance_id"), EventMode: c.Query("event"), EventKey: c.Query("event_key"), Page: positive(c, "page", 1, 1_000_000), PageSize: positive(c, "page_size", 100, max), Order: c.DefaultQuery("order", "desc")}
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
func (a *APIController) Logs(c *gin.Context) {
	page, err := a.svc.Logs(c.Request.Context(), c.Param("sender"), parseLogFilters(c, a.cfg.MaxPageSize))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, page)
}
func (a *APIController) Download(c *gin.Context) {
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
		c.JSON(422, errBody(c, "INVALID_REQUEST", "Invalid format"))
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
func (a *APIController) Stream(c *gin.Context) {
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
		c.JSON(500, errBody(c, "INTERNAL_ERROR", "Streaming unavailable"))
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
			if filters.InstanceID == "legacy" && e.InstanceID != "" || filters.InstanceID != "" && filters.InstanceID != "legacy" && e.InstanceID != filters.InstanceID {
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
func (a *APIController) Summary(c *gin.Context) {
	v, err := a.svc.Summary(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	if a.executions != nil {
		v["executions"] = a.executions.Summary()
	}
	c.JSON(200, v)
}

func (a *APIController) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, a.svc.Settings())
}

func (a *APIController) UpdateSettings(c *gin.Context) {
	var input dto.UpdateSettingsRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		c.JSON(http.StatusBadRequest, settingsErrorBody(c, "Invalid or incomplete settings.", ""))
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, settingsErrorBody(c, "The request body must contain a single JSON object.", ""))
		return
	}
	if input.LogLimit == nil || input.InactivePreservation == nil {
		c.JSON(http.StatusBadRequest, settingsErrorBody(c, "Invalid or incomplete settings.", ""))
		return
	}
	current := a.svc.Settings()
	if input.InactiveAfterSeconds != nil {
		current.InactiveAfterSeconds = *input.InactiveAfterSeconds
	}
	if input.DeleteInactiveDays != nil {
		current.DeleteInactiveDays = *input.DeleteInactiveDays
	}
	updated, err := a.svc.UpdateSettings(domain.Settings{
		LogLimit:             *input.LogLimit,
		InactivePreservation: *input.InactivePreservation,
		InactiveAfterSeconds: current.InactiveAfterSeconds,
		DeleteInactiveDays:   current.DeleteInactiveDays,
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
			"log_limit":                  updated.LogLimit,
			"inactive_preservation":      updated.InactivePreservation,
			"inactive_after_seconds":     updated.InactiveAfterSeconds,
			"delete_inactive_after_days": updated.DeleteInactiveDays,
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
func (a *APIController) Health(c *gin.Context) {
	sum, err := a.svc.Summary(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "healthy", "time": time.Now(), "uptime_seconds": int64(a.svc.Uptime().Seconds()), "senders": sum["senders"], "storage": gin.H{"writable": true, "path": a.cfg.DataDir}})
}
func (a *APIController) Ready(c *gin.Context) {
	if !a.scheduler.Ready() {
		c.JSON(503, gin.H{"status": "not_ready"})
		return
	}
	c.JSON(200, gin.H{"status": "ready", "time": time.Now()})
}
func (a *APIController) Spa(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/health" || c.Request.URL.Path == "/ready" {
		c.JSON(404, errBody(c, "NOT_FOUND", "Rota not encontrada"))
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
