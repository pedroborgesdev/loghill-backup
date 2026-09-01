package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"logtheater/internal/alerts"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/emailprovider"
	"logtheater/internal/notification"
	"logtheater/internal/services"
)

func (a *APIController) ListAlerts(c *gin.Context) {
	var enabled *bool
	if raw := c.Query("enabled"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, alertErrorBody(c, "INVALID_ALERT_FILTER", "The enabled filter is invalid.", "enabled"))
			return
		}
		enabled = &value
	}
	var severity domain.LogSeverity
	if raw := c.Query("severity"); raw != "" {
		var err error
		severity, err = domain.ParseSeverity(raw)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, alertErrorBody(c, "INVALID_ALERT_FILTER", "The severity filter is invalid.", "severity"))
			return
		}
	}
	page := a.alerts.List(domain.AlertFilters{
		SenderID: c.Query("sender_id"), Enabled: enabled, Severity: severity, Search: c.Query("search"),
		Page: positive(c, "page", 1, 1_000_000), PageSize: positive(c, "page_size", 20, a.cfg.MaxPageSize),
	})
	settings, _ := a.notifications.EmailSettings()
	c.JSON(http.StatusOK, gin.H{"items": page.Items, "pagination": page.Pagination, "summary": a.alerts.Summary(), "email_provider": settings})
}

func (a *APIController) GetAlert(c *gin.Context) {
	alert, err := a.alerts.Get(c.Param("alertID"))
	if err != nil {
		a.failAlert(c, err)
		return
	}
	c.JSON(http.StatusOK, alert)
}

func (a *APIController) CreateAlert(c *gin.Context) {
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

func (a *APIController) UpdateAlert(c *gin.Context) {
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

func (a *APIController) DeleteAlert(c *gin.Context) {
	id := c.Param("alertID")
	if a.monitoring != nil {
		if usage := a.monitoring.AlertUsageCount(id); usage > 0 {
			body := errBody(c, "ALERT_USED_BY_MONITORING", "The alert is associated with monitoring rules.")
			body["monitoring_rules"] = usage
			c.JSON(http.StatusConflict, body)
			return
		}
	}
	if err := a.alerts.Delete(id); err != nil {
		a.failAlert(c, err)
		return
	}
	slog.Info("email alert deleted", "alert_id", id)
	c.Status(http.StatusNoContent)
}

func (a *APIController) UpdateAlertStatus(c *gin.Context) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeOne(c, &input) || input.Enabled == nil {
		if input.Enabled == nil && !c.IsAborted() {
			c.JSON(http.StatusBadRequest, alertErrorBody(c, "INVALID_REQUEST", "Provide the alert state.", "enabled"))
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

func (a *APIController) TestAlert(c *gin.Context) {
	alert, sender, err := a.notifications.EnqueueAlertTest(c.Request.Context(), c.Param("alertID"))
	if errors.Is(err, services.ErrNotificationWithoutSenders) {
		c.JSON(http.StatusConflict, alertErrorBody(c, "ALERT_WITHOUT_SENDERS", "The alert has no available senders.", "sender_ids"))
		return
	}
	if errors.Is(err, notification.ErrQueueFull) || errors.Is(err, notification.ErrQueueClosed) {
		c.JSON(http.StatusServiceUnavailable, alertErrorBody(c, "EMAIL_QUEUE_FULL", "Unable to queue the test right now.", ""))
		return
	}
	if err != nil {
		a.failAlert(c, err)
		return
	}
	slog.Info("email alert test enqueued", "alert_id", alert.ID, "sender_id", sender.ID)
	c.JSON(http.StatusAccepted, gin.H{"accepted": true, "alert_id": alert.ID})
}

func (a *APIController) GetEmailSettings(c *gin.Context) {
	settings, err := a.notifications.EmailSettings()
	if err != nil {
		a.failAlert(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (a *APIController) UpdateEmailSettings(c *gin.Context) {
	var input emailconfig.Input
	if !decodeOne(c, &input) {
		return
	}
	settings, err := a.notifications.UpdateEmailSettings(input)
	if err != nil {
		a.failAlert(c, err)
		return
	}
	slog.Info("email provider settings updated", "provider", settings.Provider, "enabled", settings.Enabled, "managed_by_environment", settings.Outlook.ManagedByEnvironment)
	c.JSON(http.StatusOK, settings)
}

func (a *APIController) TestEmailConnection(c *gin.Context) {
	provider, err := a.notifications.TestEmailConnection(c.Request.Context())
	message := friendlyEmailError(err)
	slog.Info("email provider connection tested", "provider", provider, "success", err == nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, alertErrorBody(c, emailProviderErrorCode(err, "EMAIL_CONNECTION_FAILED"), message, ""))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Email provider connection validated successfully."})
}

func (a *APIController) SendTestEmail(c *gin.Context) {
	var input struct {
		Recipient string `json:"recipient"`
	}
	if !decodeOne(c, &input) {
		return
	}
	provider, recipient, err := a.notifications.SendTestEmail(c.Request.Context(), input.Recipient)
	if errors.Is(err, services.ErrInvalidEmailRecipient) {
		c.JSON(http.StatusUnprocessableEntity, alertErrorBody(c, "INVALID_EMAIL", "Enter a valid recipient.", "recipient"))
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, alertErrorBody(c, emailProviderErrorCode(err, "EMAIL_SEND_FAILED"), friendlyEmailError(err), ""))
		return
	}
	slog.Info("email provider test message sent", "provider", provider, "recipient", recipient)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Test email sent."})
}

func (a *APIController) failAlert(c *gin.Context, err error) {
	var alertValidation *alerts.ValidationError
	var settingsValidation *emailconfig.ValidationError
	switch {
	case errors.Is(err, alerts.ErrNotFound):
		c.JSON(http.StatusNotFound, alertErrorBody(c, "ALERT_NOT_FOUND", "Alert not found.", ""))
	case errors.Is(err, alerts.ErrEmailNotConfigured), errors.Is(err, emailprovider.ErrNotConfigured):
		c.JSON(http.StatusConflict, alertErrorBody(c, "EMAIL_NOT_CONFIGURED", "Configure and enable email before enabling the alert.", "provider"))
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
		c.JSON(http.StatusBadRequest, alertErrorBody(c, "INVALID_REQUEST", "Invalid or incomplete request body.", ""))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, alertErrorBody(c, "INVALID_REQUEST", "The request body must contain a single JSON object.", ""))
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
		return "The email provider is not configured."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "The email service did not respond within the expected time."
	}
	return "Unable to complete the operation with the email provider."
}

func emailProviderErrorCode(err error, fallback string) string {
	var providerError *emailprovider.Error
	if errors.As(err, &providerError) && providerError.Code != "" {
		return providerError.Code
	}
	return fallback
}
