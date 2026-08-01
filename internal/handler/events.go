package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"logtheater/internal/domain"
	"logtheater/internal/events"
	"logtheater/internal/validation"
)

func (a *API) listEvents(c *gin.Context) {
	var enabled *bool
	if raw := c.Query("enabled"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, eventErrorBody(c, "INVALID_EVENT_FILTER", "O filtro enabled é inválido.", "enabled"))
			return
		}
		enabled = &value
	}
	action := domain.EventActionType(c.Query("action_type"))
	if action != "" && action != domain.EventActionEmail {
		c.JSON(http.StatusUnprocessableEntity, eventErrorBody(c, "INVALID_EVENT_FILTER", "O filtro action_type é inválido.", "action_type"))
		return
	}
	page := a.events.List(domain.EventFilters{Search: c.Query("search"), SenderID: c.Query("sender_id"), ActionType: action, Enabled: enabled, Page: positive(c, "page", 1, 1_000_000), PageSize: positive(c, "page_size", 20, a.cfg.MaxPageSize)})
	settings, _ := a.emailConfig.Safe()
	c.JSON(http.StatusOK, gin.H{"items": page.Items, "pagination": page.Pagination, "summary": a.events.Summary(), "email_provider": settings})
}

func (a *API) getEvent(c *gin.Context) {
	event, err := a.events.Get(c.Param("eventID"))
	if err != nil {
		a.failEvent(c, err)
		return
	}
	c.JSON(http.StatusOK, event)
}

func (a *API) checkEventKey(c *gin.Context) {
	key := c.Query("key")
	c.JSON(http.StatusOK, gin.H{"key": key, "valid": events.ValidKey(key), "available": a.events.KeyAvailable(key)})
}

func (a *API) createEvent(c *gin.Context) {
	var input domain.EventInput
	if !decodeOne(c, &input) {
		return
	}
	event, err := a.events.Create(c.Request.Context(), input)
	if err != nil {
		a.failEvent(c, err)
		return
	}
	slog.Info("event created", "event_id", event.ID, "event_key", event.Key, "sender_count", len(event.SenderIDs), "enabled", event.Enabled)
	c.JSON(http.StatusCreated, event)
}

func (a *API) updateEvent(c *gin.Context) {
	var input domain.EventInput
	if !decodeOne(c, &input) {
		return
	}
	event, err := a.events.Update(c.Request.Context(), c.Param("eventID"), input)
	if err != nil {
		a.failEvent(c, err)
		return
	}
	slog.Info("event updated", "event_id", event.ID, "event_key", event.Key, "sender_count", len(event.SenderIDs), "enabled", event.Enabled)
	c.JSON(http.StatusOK, event)
}

func (a *API) updateEventStatus(c *gin.Context) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeOne(c, &input) {
		return
	}
	if input.Enabled == nil {
		c.JSON(http.StatusBadRequest, eventErrorBody(c, "INVALID_REQUEST", "Informe o estado do evento.", "enabled"))
		return
	}
	event, err := a.events.SetEnabled(c.Request.Context(), c.Param("eventID"), *input.Enabled)
	if err != nil {
		a.failEvent(c, err)
		return
	}
	slog.Info("event status changed", "event_id", event.ID, "event_key", event.Key, "enabled", event.Enabled)
	c.JSON(http.StatusOK, event)
}

func (a *API) deleteEvent(c *gin.Context) {
	id := c.Param("eventID")
	if err := a.events.Delete(id); err != nil {
		a.failEvent(c, err)
		return
	}
	slog.Info("event deleted", "event_id", id)
	c.Status(http.StatusNoContent)
}

func (a *API) testEvent(c *gin.Context) {
	var input struct {
		Recipient string `json:"recipient"`
	}
	if !decodeOne(c, &input) {
		return
	}
	recipient, valid := validation.EmailAddress(input.Recipient)
	if !valid {
		c.JSON(http.StatusUnprocessableEntity, eventErrorBody(c, "INVALID_EMAIL", "Informe um destinatário válido.", "recipient"))
		return
	}
	if !a.emailConfig.IsReady() {
		a.failEvent(c, events.ErrEmailNotConfigured)
		return
	}
	event, err := a.events.Get(c.Param("eventID"))
	if err != nil {
		a.failEvent(c, err)
		return
	}
	if len(event.SenderIDs) == 0 {
		c.JSON(http.StatusConflict, eventErrorBody(c, "EVENT_WITHOUT_SENDERS", "O evento não possui senders disponíveis.", "sender_ids"))
		return
	}
	sender, err := a.svc.Get(c.Request.Context(), event.SenderIDs[0])
	if err != nil {
		a.fail(c, err)
		return
	}
	now := time.Now()
	value := domain.Notification{SourceType: domain.NotificationSourceEvent, SourceID: event.ID, Event: event, Sender: sender, Recipients: []string{recipient}, Test: true, Entry: domain.LogEntry{Timestamp: now, SenderID: sender.ID, Severity: domain.Info, Message: "Mensagem fictícia de teste do evento LogHill.", Event: event.Key, Metadata: map[string]any{"destinatario": "cliente@exemplo.com", "protocolo": "TESTE-123", "test": true}}}
	if err = a.dispatcher.Enqueue(value); err != nil {
		c.JSON(http.StatusServiceUnavailable, eventErrorBody(c, "EMAIL_QUEUE_FULL", "Não foi possível enfileirar o teste agora.", ""))
		return
	}
	slog.Info("event test enqueued", "event_id", event.ID, "sender_id", sender.ID, "recipient", recipient)
	c.JSON(http.StatusAccepted, gin.H{"accepted": true, "event_id": event.ID})
}

func (a *API) failEvent(c *gin.Context, err error) {
	var validationError *events.ValidationError
	switch {
	case errors.Is(err, events.ErrNotFound):
		c.JSON(http.StatusNotFound, eventErrorBody(c, "EVENT_NOT_FOUND", "Evento não encontrado.", ""))
	case errors.Is(err, events.ErrAlreadyExists):
		c.JSON(http.StatusConflict, eventErrorBody(c, "EVENT_ALREADY_EXISTS", "Já existe um evento com esta chave.", "key"))
	case errors.Is(err, events.ErrEmailNotConfigured):
		c.JSON(http.StatusConflict, eventErrorBody(c, "OUTLOOK_NOT_CONFIGURED", "Configure e habilite o Outlook antes de ativar o evento.", "provider"))
	case errors.As(err, &validationError):
		status := http.StatusUnprocessableEntity
		if validationError.Code == "EVENT_KEY_IMMUTABLE" {
			status = http.StatusConflict
		}
		c.JSON(status, eventErrorBody(c, validationError.Code, validationError.Message, validationError.Field))
	default:
		a.fail(c, err)
	}
}

func eventErrorBody(c *gin.Context, code, message, field string) gin.H {
	value := gin.H{"code": code, "message": message, "request_id": c.GetString("request_id")}
	if field != "" {
		value["field"] = field
	}
	return gin.H{"error": value}
}
