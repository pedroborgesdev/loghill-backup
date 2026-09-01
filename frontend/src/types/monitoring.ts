import type { LogSeverity } from "./api";

export type MonitoringConditionType = "event_triggered" | "alert_triggered" | "sender_status" | "log_received" | "message" | "severity" | "metadata" | "time" | "weekday" | "date" | "wait_until";
export type MonitoringActionType = "trigger_event" | "send_email";
export type LogicalOperator = "and" | "or";
export interface MonitoringCondition { id?: string; type: MonitoringConditionType; operator: string; value: Record<string, unknown>; negated: boolean }
export interface MonitoringNode { connector?: LogicalOperator; condition?: MonitoringCondition; group?: MonitoringGroup }
export interface MonitoringGroup { id?: string; operator: LogicalOperator; negated: boolean; nodes: MonitoringNode[] }
export interface MonitoringAction { id?: string; type: MonitoringActionType; config: Record<string, unknown> }
export interface MonitoringRuleInput { name: string; description: string; sender_ids: string[]; include_new_senders?: boolean; enabled: boolean; status?: "active" | "draft"; expression: MonitoringGroup; actions: MonitoringAction[] }
export interface MonitoringRule extends MonitoringRuleInput { id: string; created_at: string; updated_at: string; last_evaluated_at?: string; last_executed_at?: string; last_result?: string; last_error?: string; execution_count: number; failure_count: number }
export interface MonitoringPage { items: MonitoringRule[]; pagination: { page: number; page_size: number; total: number; total_pages: number; returned: number }; summary: { total: number; active: number; recent_executions: number; recent_failures: number } }
export interface MonitoringConditionResult { id: string; matched: boolean; description: string; error?: string }
export interface MonitoringTestResult { matched: boolean; pending?: boolean; conditions: MonitoringConditionResult[]; actions: MonitoringActionType[]; summary: string }
export interface MonitoringTestInput { sender_id: string; trigger: { type: string; alert_id?: string; event_key?: string; severity: LogSeverity; message: string; timestamp: string; metadata?: Record<string, unknown> }; execute_actions: boolean }
