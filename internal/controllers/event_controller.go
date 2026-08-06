package controllers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"logtheater/internal/domain"
	"logtheater/internal/events"
	"logtheater/internal/notification"
	"logtheater/internal/services"
)

func (a *APIController) ListEvents(c *gin.Context) {
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
	if action != "" && action != domain.EventActionEmail && action != domain.EventActionNone {
		c.JSON(http.StatusUnprocessableEntity, eventErrorBody(c, "INVALID_EVENT_FILTER", "O filtro action_type é inválido.", "action_type"))
		return
	}
	page := a.events.List(domain.EventFilters{Search: c.Query("search"), SenderName: c.Query("sender_name"), ActionType: action, Enabled: enabled, Page: positive(c, "page", 1, 1_000_000), PageSize: positive(c, "page_size", 20, a.cfg.MaxPageSize)})
	settings, _ := a.notifications.EmailSettings()
	c.JSON(http.StatusOK, gin.H{"items": page.Items, "pagination": page.Pagination, "summary": a.events.Summary(), "email_provider": settings})
}

func (a *APIController) GetEvent(c *gin.Context) {
	event, err := a.events.Get(c.Param("eventID"))
	if err != nil {
		a.failEvent(c, err)
		return
	}
	c.JSON(http.StatusOK, event)
}

func (a *APIController) CheckEventKey(c *gin.Context) {
	key := c.Query("key")
	c.JSON(http.StatusOK, gin.H{"key": key, "valid": events.ValidKey(key), "available": a.events.KeyAvailable(key)})
}

func (a *APIController) CreateEvent(c *gin.Context) {
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

func (a *APIController) UpdateEvent(c *gin.Context) {
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

func (a *APIController) UpdateEventStatus(c *gin.Context) {
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

func (a *APIController) DeleteEvent(c *gin.Context) {
	id := c.Param("eventID")
	if a.monitoring != nil {
		if usage := a.monitoring.EventUsageCount(id); usage > 0 {
			body := errBody(c, "EVENT_USED_BY_MONITORING", "O evento está associado a regras de monitoramento.")
			body["monitoring_rules"] = usage
			c.JSON(http.StatusConflict, body)
			return
		}
	}
	if err := a.events.Delete(id); err != nil {
		a.failEvent(c, err)
		return
	}
	slog.Info("event deleted", "event_id", id)
	c.Status(http.StatusNoContent)
}

func (a *APIController) TestEvent(c *gin.Context) {
	var input struct {
		Recipient string `json:"recipient"`
	}
	if !decodeOne(c, &input) {
		return
	}
	event, sender, recipient, err := a.notifications.EnqueueEventTest(c.Request.Context(), c.Param("eventID"), input.Recipient)
	if errors.Is(err, services.ErrInvalidEmailRecipient) {
		c.JSON(http.StatusUnprocessableEntity, eventErrorBody(c, "INVALID_EMAIL", "Informe um destinatário válido.", "recipient"))
		return
	}
	if errors.Is(err, services.ErrNotificationWithoutSenders) {
		c.JSON(http.StatusConflict, eventErrorBody(c, "EVENT_WITHOUT_SENDERS", "O evento não possui senders disponíveis.", "sender_ids"))
		return
	}
	if errors.Is(err, notification.ErrQueueFull) || errors.Is(err, notification.ErrQueueClosed) {
		c.JSON(http.StatusServiceUnavailable, eventErrorBody(c, "EMAIL_QUEUE_FULL", "Não foi possível enfileirar o teste agora.", ""))
		return
	}
	if err != nil {
		a.failEvent(c, err)
		return
	}
	slog.Info("event test enqueued", "event_id", event.ID, "sender_id", sender.ID, "recipient", recipient)
	c.JSON(http.StatusAccepted, gin.H{"accepted": true, "event_id": event.ID})
}

func (a *APIController) failEvent(c *gin.Context, err error) {
	var validationError *events.ValidationError
	switch {
	case errors.Is(err, events.ErrNotFound):
		c.JSON(http.StatusNotFound, eventErrorBody(c, "EVENT_NOT_FOUND", "Evento não encontrado.", ""))
	case errors.Is(err, events.ErrAlreadyExists):
		c.JSON(http.StatusConflict, eventErrorBody(c, "EVENT_ALREADY_EXISTS", "Já existe um evento com esta chave.", "key"))
	case errors.Is(err, events.ErrEmailNotConfigured):
		c.JSON(http.StatusConflict, eventErrorBody(c, "EMAIL_NOT_CONFIGURED", "Configure e habilite um e-mail antes de ativar o evento.", "provider"))
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
