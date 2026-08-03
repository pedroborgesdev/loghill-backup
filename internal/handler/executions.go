package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"logtheater/internal/domain"
	"logtheater/internal/executions"
)

func parseExecutionFilters(c *gin.Context, max int) (executions.Filters, error) {
	f := executions.Filters{SourceType: executions.SourceType(c.Query("source_type")), SourceID: c.Query("source_id"), SenderID: c.Query("sender_id"), TriggerType: c.Query("trigger_type"), ActionType: c.Query("action_type"), Search: c.Query("search"), Order: c.DefaultQuery("order", "desc"), Recent: c.Query("recent") == "true", Page: positive(c, "page", 1, 1_000_000), PageSize: positive(c, "page_size", 20, max), Statuses: map[executions.Status]bool{}, Severities: map[domain.LogSeverity]bool{}}
	for _, raw := range strings.Split(c.Query("status"), ",") {
		if raw != "" {
			f.Statuses[executions.Status(raw)] = true
		}
	}
	for _, raw := range strings.Split(c.Query("severity"), ",") {
		if severity, err := domain.ParseSeverity(raw); err == nil {
			f.Severities[severity] = true
		}
	}
	if raw := c.Query("started_from"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return f, err
		}
		f.StartedFrom = &value
	}
	if raw := c.Query("started_to"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return f, err
		}
		f.StartedTo = &value
	}
	if f.StartedFrom != nil && f.StartedTo != nil && (!f.StartedFrom.Before(*f.StartedTo) || f.StartedTo.Sub(*f.StartedFrom) > 366*24*time.Hour) {
		return f, &executionFilterError{}
	}
	return f, nil
}

type executionFilterError struct{}

func (*executionFilterError) Error() string { return "invalid execution period" }

func (a *API) listExecutions(c *gin.Context) {
	filters, err := parseExecutionFilters(c, a.cfg.MaxPageSize)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, errBody(c, "INVALID_EXECUTION_FILTER", "Período de execução inválido."))
		return
	}
	c.JSON(http.StatusOK, a.executions.List(filters))
}
func (a *API) getExecution(c *gin.Context) {
	record, ok := a.executions.Get(c.Param("executionID"))
	if !ok {
		c.JSON(http.StatusNotFound, errBody(c, "EXECUTION_NOT_FOUND", "Execução não encontrada."))
		return
	}
	c.JSON(http.StatusOK, record)
}
func (a *API) recentExecutions(c *gin.Context) {
	limit := positive(c, "limit", 10, 100)
	filters, err := parseExecutionFilters(c, 100)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, errBody(c, "INVALID_EXECUTION_FILTER", "Filtros inválidos."))
		return
	}
	filters.Page = 1
	filters.PageSize = limit
	c.JSON(http.StatusOK, a.executions.List(filters))
}
