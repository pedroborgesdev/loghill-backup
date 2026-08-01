package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"logtheater/internal/alerts"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/emailprovider"
	"logtheater/internal/notification"
	"logtheater/internal/validation"
)

func (a *API) listAlerts(c *gin.Context) {
	var enabled *bool
	if raw := c.Query("enabled"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, alertErrorBody(c, "INVALID_ALERT_FILTER", "O filtro enabled é inválido.", "enabled"))
			return
		}
		enabled = &value
	}
	var severity domain.LogSeverity
	if raw := c.Query("severity"); raw != "" {
		var err error
		severity, err = domain.ParseSeverity(raw)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, alertErrorBody(c, "INVALID_ALERT_FILTER", "O filtro severity é inválido.", "severity"))
			return
		}
	}
	page := a.alerts.List(domain.AlertFilters{
		SenderID: c.Query("sender_id"), Enabled: enabled, Severity: severity, Search: c.Query("search"),
		Page: positive(c, "page", 1, 1_000_000), PageSize: positive(c, "page_size", 20, a.cfg.MaxPageSize),
	})
	settings, _ := a.emailConfig.Safe()
	c.JSON(http.StatusOK, gin.H{"items": page.Items, "pagination": page.Pagination, "summary": a.alerts.Summary(), "email_provider": settings})
}

func (a *API) getAlert(c *gin.Context) {
	alert, err := a.alerts.Get(c.Param("alertID"))
	if err != nil {
		a.failAlert(c, err)
		return
	}
	c.JSON(http.StatusOK, alert)
}

func (a *API) createAlert(c *gin.Context) {
	var input domain.AlertInput
	if !decodeOne(c, &input) {
		return
	}
	alert, err := a.alerts.Create(c.Request.Context(), input)
	if err != nil {
		a.failAlert(c, err)
		return
	}
	slog.Info("email alert created", "alert_id", alert.ID, "sender_ids", alert.SenderIDs, "enabled", alert.Enabled)
	c.JSON(http.StatusCreated, alert)
}

func (a *API) updateAlert(c *gin.Context) {
	var input domain.AlertInput
	if !decodeOne(c, &input) {
		return
	}
	alert, err := a.alerts.Update(c.Request.Context(), c.Param("alertID"), input)
	if err != nil {
		a.failAlert(c, err)
		return
	}
	slog.Info("email alert updated", "alert_id", alert.ID, "sender_ids", alert.SenderIDs, "enabled", alert.Enabled)
	c.JSON(http.StatusOK, alert)
}

func (a *API) deleteAlert(c *gin.Context) {
	id := c.Param("alertID")
	if err := a.alerts.Delete(id); err != nil {
		a.failAlert(c, err)
		return
	}
	slog.Info("email alert deleted", "alert_id", id)
	c.Status(http.StatusNoContent)
}

func (a *API) updateAlertStatus(c *gin.Context) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeOne(c, &input) || input.Enabled == nil {
		if input.Enabled == nil && !c.IsAborted() {
			c.JSON(http.StatusBadRequest, alertErrorBody(c, "INVALID_REQUEST", "Informe o estado do alerta.", "enabled"))
		}
		return
	}
	alert, err := a.alerts.Get(c.Param("alertID"))
	if err != nil {
		a.failAlert(c, err)
		return
	}
	updated, err := a.alerts.Update(c.Request.Context(), alert.ID, domain.AlertInput{Name: alert.Name, SenderIDs: alert.SenderIDs, Severities: alert.Severities, Recipients: alert.Recipients, Provider: alert.Provider, Enabled: *input.Enabled})
	if err != nil {
		a.failAlert(c, err)
		return
	}
	slog.Info("email alert status changed", "alert_id", alert.ID, "enabled", updated.Enabled)
	c.JSON(http.StatusOK, updated)
}

func (a *API) testAlert(c *gin.Context) {
	alert, err := a.alerts.Get(c.Param("alertID"))
	if err != nil {
		a.failAlert(c, err)
		return
	}
	if !a.emailConfig.IsReady() {
		a.failAlert(c, alerts.ErrEmailNotConfigured)
		return
	}
	if len(alert.SenderIDs) == 0 {
		c.JSON(http.StatusConflict, alertErrorBody(c, "ALERT_WITHOUT_SENDERS", "O alerta não possui senders disponíveis.", "sender_ids"))
		return
	}
	sender, err := a.svc.Get(c.Request.Context(), alert.SenderIDs[0])
	if err != nil {
		a.fail(c, err)
		return
	}
	severity := domain.Info
	if len(alert.Severities) > 0 {
		severity = alert.Severities[0]
	}
	now := time.Now()
	value := domain.Notification{Alert: alert, Sender: sender, Test: true, Entry: domain.LogEntry{Timestamp: now, Severity: severity, Message: "Mensagem de teste do alerta de e-mail do LogHill.", Metadata: map[string]any{"test": true}}}
	if err = a.dispatcher.Enqueue(value); err != nil {
		c.JSON(http.StatusServiceUnavailable, alertErrorBody(c, "EMAIL_QUEUE_FULL", "Não foi possível enfileirar o teste agora.", ""))
		return
	}
	slog.Info("email alert test enqueued", "alert_id", alert.ID, "sender_id", sender.ID)
	c.JSON(http.StatusAccepted, gin.H{"accepted": true, "alert_id": alert.ID})
}

func (a *API) getEmailSettings(c *gin.Context) {
	settings, err := a.emailConfig.Safe()
	if err != nil {
		a.failAlert(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (a *API) updateEmailSettings(c *gin.Context) {
	var input emailconfig.Input
	if !decodeOne(c, &input) {
		return
	}
	settings, err := a.emailConfig.Update(input, time.Now())
	if err != nil {
		a.failAlert(c, err)
		return
	}
	slog.Info("email provider settings updated", "provider", settings.Provider, "enabled", settings.Enabled, "managed_by_environment", settings.Outlook.ManagedByEnvironment)
	c.JSON(http.StatusOK, settings)
}

func (a *API) testEmailConnection(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), a.cfg.EmailAlertSendTimeout)
	defer cancel()
	err := a.emailProvider.TestConnection(ctx)
	message := ""
	if err != nil {
		message = friendlyEmailError(err)
	}
	_ = a.emailConfig.RecordTest(err == nil, message, time.Now())
	slog.Info("email provider connection tested", "provider", a.emailProvider.Provider(), "success", err == nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, alertErrorBody(c, emailProviderErrorCode(err, "OUTLOOK_CONNECTION_FAILED"), message, ""))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Conexão com o Outlook validada com sucesso."})
}

func (a *API) sendTestEmail(c *gin.Context) {
	var input struct {
		Recipient string `json:"recipient"`
	}
	if !decodeOne(c, &input) {
		return
	}
	recipient, valid := validation.EmailAddress(input.Recipient)
	if !valid {
		c.JSON(http.StatusUnprocessableEntity, alertErrorBody(c, "INVALID_EMAIL", "Informe um destinatário válido.", "recipient"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), a.cfg.EmailAlertSendTimeout)
	defer cancel()
	message, err := notification.NewTemplate(a.cfg.PublicURL).RenderProviderTest(recipient)
	if err != nil {
		c.JSON(http.StatusInternalServerError, alertErrorBody(c, "EMAIL_TEMPLATE_FAILED", "Não foi possível preparar o e-mail de teste.", ""))
		return
	}
	if err := a.emailProvider.Send(ctx, message); err != nil {
		c.JSON(http.StatusBadGateway, alertErrorBody(c, emailProviderErrorCode(err, "OUTLOOK_SEND_FAILED"), friendlyEmailError(err), ""))
		return
	}
	slog.Info("email provider test message sent", "provider", a.emailProvider.Provider(), "recipient", recipient)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "E-mail de teste enviado."})
}

func (a *API) failAlert(c *gin.Context, err error) {
	var alertValidation *alerts.ValidationError
	var settingsValidation *emailconfig.ValidationError
	switch {
	case errors.Is(err, alerts.ErrNotFound):
		c.JSON(http.StatusNotFound, alertErrorBody(c, "ALERT_NOT_FOUND", "Alerta não encontrado.", ""))
	case errors.Is(err, alerts.ErrEmailNotConfigured), errors.Is(err, emailprovider.ErrNotConfigured):
		c.JSON(http.StatusConflict, alertErrorBody(c, "OUTLOOK_NOT_CONFIGURED", "Configure e habilite o Outlook antes de ativar o alerta.", "provider"))
	case errors.As(err, &alertValidation):
		c.JSON(http.StatusUnprocessableEntity, alertErrorBody(c, alertValidation.Code, alertValidation.Message, alertValidation.Field))
	case errors.As(err, &settingsValidation):
		c.JSON(http.StatusUnprocessableEntity, alertErrorBody(c, settingsValidation.Code, settingsValidation.Message, settingsValidation.Field))
	default:
		a.fail(c, err)
	}
}

func decodeOne(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		c.JSON(http.StatusBadRequest, alertErrorBody(c, "INVALID_REQUEST", "Body inválido ou incompleto.", ""))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, alertErrorBody(c, "INVALID_REQUEST", "O body deve conter um único objeto JSON.", ""))
		return false
	}
	return true
}

func alertErrorBody(c *gin.Context, code, message, field string) gin.H {
	value := gin.H{"code": code, "message": message, "request_id": c.GetString("request_id")}
	if field != "" {
		value["field"] = field
	}
	return gin.H{"error": value}
}

func friendlyEmailError(err error) string {
	var providerError *emailprovider.Error
	if errors.As(err, &providerError) {
		return providerError.Message
	}
	if errors.Is(err, emailprovider.ErrNotConfigured) {
		return "O Outlook não está configurado."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "O serviço de e-mail não respondeu dentro do tempo esperado."
	}
	return "Não foi possível concluir a operação com o Outlook."
}

func emailProviderErrorCode(err error, fallback string) string {
	var providerError *emailprovider.Error
	if errors.As(err, &providerError) && providerError.Code != "" {
		return providerError.Code
	}
	return fallback
}
