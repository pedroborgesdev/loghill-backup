package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"logtheater/internal/monitoring"
)

func (a *API) monitoringError(c *gin.Context, err error) {
	var validation *monitoring.ValidationError
	switch {
	case errors.As(err, &validation):
		c.JSON(http.StatusUnprocessableEntity, errorBodyWithField(c, "INVALID_MONITORING_RULE", validation.Message, validation.Field))
	case errors.Is(err, monitoring.ErrNotFound):
		c.JSON(http.StatusNotFound, errBody(c, "MONITORING_RULE_NOT_FOUND", "Regra de monitoramento não encontrada."))
	default:
		a.fail(c, err)
	}
}
func monitoringFilters(c *gin.Context) monitoring.Filters {
	f := monitoring.Filters{Search: c.Query("search"), SenderName: c.Query("sender_name"), ConditionType: monitoring.ConditionType(c.Query("condition_type")), ActionType: monitoring.ActionType(c.Query("action_type")), Page: positive(c, "page", 1, 1_000_000), PageSize: positive(c, "page_size", 20, 100)}
	if raw := c.Query("enabled"); raw != "" {
		if value, err := strconv.ParseBool(raw); err == nil {
			f.Enabled = &value
		}
	}
	return f
}
func (a *API) listMonitoringRules(c *gin.Context) {
	c.JSON(http.StatusOK, a.monitoring.List(monitoringFilters(c)))
}
func (a *API) getMonitoringRule(c *gin.Context) {
	v, err := a.monitoring.Get(c.Param("ruleID"))
	if err != nil {
		a.monitoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}
func (a *API) createMonitoringRule(c *gin.Context) {
	var in monitoring.RuleInput
	if !decodeOne(c, &in) {
		return
	}
	v, err := a.monitoring.Create(c.Request.Context(), in)
	if err != nil {
		a.monitoringError(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}
func (a *API) updateMonitoringRule(c *gin.Context) {
	var in monitoring.RuleInput
	if !decodeOne(c, &in) {
		return
	}
	v, err := a.monitoring.Update(c.Request.Context(), c.Param("ruleID"), in)
	if err != nil {
		a.monitoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}
func (a *API) updateMonitoringRuleStatus(c *gin.Context) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeOne(c, &in) {
		return
	}
	v, err := a.monitoring.SetEnabled(c.Request.Context(), c.Param("ruleID"), in.Enabled)
	if err != nil {
		a.monitoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}
func (a *API) deleteMonitoringRule(c *gin.Context) {
	if err := a.monitoring.Delete(c.Param("ruleID")); err != nil {
		a.monitoringError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (a *API) duplicateMonitoringRule(c *gin.Context) {
	v, err := a.monitoring.Duplicate(c.Request.Context(), c.Param("ruleID"))
	if err != nil {
		a.monitoringError(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}
func (a *API) validateMonitoringRule(c *gin.Context) {
	var in monitoring.RuleInput
	if !decodeOne(c, &in) {
		return
	}
	if err := a.monitoring.Validate(c.Request.Context(), in, ""); err != nil {
		a.monitoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"valid": true})
}
func (a *API) testMonitoringRule(c *gin.Context) {
	rule, err := a.monitoring.Get(c.Param("ruleID"))
	if err != nil {
		a.monitoringError(c, err)
		return
	}
	var in monitoring.TestInput
	if !decodeOne(c, &in) {
		return
	}
	if in.ExecuteActions && c.GetHeader("X-Monitoring-Execute-Actions") != "confirm" {
		c.JSON(http.StatusForbidden, errBody(c, "MONITORING_ACTION_CONFIRMATION_REQUIRED", "Confirme explicitamente a execução das ações."))
		return
	}
	result, err := a.monitoring.Test(c.Request.Context(), rule, in)
	if err != nil {
		a.monitoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
func (a *API) listMonitoringExecutions(c *gin.Context) {
	limit := positive(c, "limit", 50, 200)
	items, err := a.monitoring.Executions(c.Param("ruleID"), limit)
	if err != nil {
		a.monitoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
func (a *API) getMonitoringExecution(c *gin.Context) {
	v, err := a.monitoring.Execution(c.Param("executionID"))
	if err != nil {
		a.monitoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}
