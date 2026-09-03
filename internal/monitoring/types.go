package monitoring

import (
	"encoding/json"
	"time"

	"logtheater/internal/domain"
)

type ConditionType string
type ActionType string
type LogicalOperator string

const (
	ConditionEvent        ConditionType   = "event_triggered"
	ConditionAlert        ConditionType   = "alert_triggered"
	ConditionSenderStatus ConditionType   = "sender_status"
	ConditionLogReceived  ConditionType   = "log_received"
	ConditionMessage      ConditionType   = "message"
	ConditionSeverity     ConditionType   = "severity"
	ConditionMetadata     ConditionType   = "metadata"
	ConditionTime         ConditionType   = "time"
	ConditionWeekday      ConditionType   = "weekday"
	ConditionDate         ConditionType   = "date"
	ConditionWaitUntil    ConditionType   = "wait_until"
	ActionEvent           ActionType      = "trigger_event"
	ActionEmail           ActionType      = "send_email"
	ActionHTTP            ActionType      = "send_http"
	LogicalAnd            LogicalOperator = "and"
	LogicalOr             LogicalOperator = "or"
)

type Condition struct {
	ID       string          `json:"id"`
	Type     ConditionType   `json:"type"`
	Operator string          `json:"operator"`
	Value    json.RawMessage `json:"value"`
	Negated  bool            `json:"negated"`
}

type ExpressionNode struct {
	Connector LogicalOperator  `json:"connector,omitempty"`
	Condition *Condition       `json:"condition,omitempty"`
	Group     *ExpressionGroup `json:"group,omitempty"`
}

type ExpressionGroup struct {
	ID       string           `json:"id"`
	Operator LogicalOperator  `json:"operator"`
	Negated  bool             `json:"negated"`
	Nodes    []ExpressionNode `json:"nodes"`
}

type Action struct {
	ID     string          `json:"id"`
	Type   ActionType      `json:"type"`
	Config json.RawMessage `json:"config"`
}

type Rule struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Description       string          `json:"description,omitempty"`
	SenderIDs         []string        `json:"sender_ids"`
	IncludeNewSenders bool            `json:"include_new_senders"`
	Expression        ExpressionGroup `json:"expression"`
	Actions           []Action        `json:"actions"`
	Enabled           bool            `json:"enabled"`
	Status            string          `json:"status"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	LastEvaluatedAt   *time.Time      `json:"last_evaluated_at,omitempty"`
	LastExecutedAt    *time.Time      `json:"last_executed_at,omitempty"`
	LastResult        string          `json:"last_result,omitempty"`
	LastError         string          `json:"last_error,omitempty"`
	ExecutionCount    int64           `json:"execution_count"`
	FailureCount      int64           `json:"failure_count"`
}

type RuleInput struct {
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	SenderIDs         []string        `json:"sender_ids"`
	IncludeNewSenders bool            `json:"include_new_senders"`
	Expression        ExpressionGroup `json:"expression"`
	Actions           []Action        `json:"actions"`
	Enabled           bool            `json:"enabled"`
	Status            string          `json:"status,omitempty"`
}

type Filters struct {
	Search, SenderID, SenderName string
	ConditionType                ConditionType
	ActionType                   ActionType
	Enabled                      *bool
	Page, PageSize               int
}

type Page struct {
	Items      []Rule            `json:"items"`
	Pagination domain.Pagination `json:"pagination"`
	Summary    map[string]int64  `json:"summary"`
}

type Trigger struct {
	Type      string             `json:"type"`
	AlertID   string             `json:"alert_id,omitempty"`
	EventKey  string             `json:"event_key,omitempty"`
	Severity  domain.LogSeverity `json:"severity"`
	Message   string             `json:"message"`
	Timestamp time.Time          `json:"timestamp"`
	Metadata  map[string]any     `json:"metadata,omitempty"`
}

type TestInput struct {
	SenderID       string  `json:"sender_id"`
	Trigger        Trigger `json:"trigger"`
	ExecuteActions bool    `json:"execute_actions"`
}

type ConditionResult struct {
	ID          string `json:"id"`
	Matched     bool   `json:"matched"`
	Description string `json:"description"`
	Error       string `json:"error,omitempty"`
}

type EvaluationResult struct {
	Matched    bool              `json:"matched"`
	Pending    bool              `json:"pending,omitempty"`
	Conditions []ConditionResult `json:"conditions"`
	Actions    []ActionType      `json:"actions"`
	Summary    string            `json:"summary"`
}

type Execution struct {
	ID            string           `json:"id"`
	RuleID        string           `json:"rule_id"`
	SenderID      string           `json:"sender_id"`
	TriggerType   string           `json:"trigger_type"`
	TriggerID     string           `json:"trigger_id,omitempty"`
	CorrelationID string           `json:"correlation_id"`
	StartedAt     time.Time        `json:"started_at"`
	FinishedAt    time.Time        `json:"finished_at"`
	Status        string           `json:"status"`
	Result        EvaluationResult `json:"result"`
	Error         string           `json:"error,omitempty"`
}

type PendingEvaluation struct {
	ExecutionID   string    `json:"execution_id,omitempty"`
	ID            string    `json:"id"`
	RuleID        string    `json:"rule_id"`
	SenderID      string    `json:"sender_id"`
	TriggeredAt   time.Time `json:"triggered_at"`
	DueAt         time.Time `json:"due_at"`
	Status        string    `json:"status"`
	Trigger       Trigger   `json:"trigger"`
	CorrelationID string    `json:"correlation_id"`
	LastMatched   bool      `json:"last_matched,omitempty"`
}
