import type { LogSeverity, Pagination } from "./api";

export type ExecutionSourceType = "alert" | "event" | "monitoring";
export type ExecutionStatus = "pending" | "processing" | "success" | "partial" | "failed" | "cancelled" | "skipped";
export interface ExecutionActionResult { id:string;type:string;status:ExecutionStatus;started_at?:string;finished_at?:string;duration_ms?:number;attempt_count:number;error_code?:string;error_message?:string }
export interface ExecutionConditionResult { id:string;matched:boolean;description:string;error?:string }
export interface ExecutionRecord { id:string;source_type:ExecutionSourceType;source_id:string;source_name:string;sender_id:string;sender_name?:string;trigger_type:string;trigger_id?:string;trigger_name?:string;trigger_message?:string;severity?:LogSeverity;status:ExecutionStatus;correlation_id?:string;causation_id?:string;started_at:string;finished_at?:string;duration_ms?:number;attempt_count:number;actions:ExecutionActionResult[];conditions?:ExecutionConditionResult[];error_code?:string;error_message?:string;metadata?:Record<string,unknown> }
export interface ExecutionPage { items:ExecutionRecord[];pagination:Pagination }
export interface ExecutionSummary { last_hour:number;last_24_hours:number;running:number;failed_last_hour:number;alerts_last_24_hours:number;events_last_24_hours:number;monitoring_last_24_hours:number }
export function isRecentExecution(startedAt:string,now=new Date()){const started=new Date(startedAt).getTime(),difference=now.getTime()-started;return Number.isFinite(started)&&difference>=0&&difference<60*60*1000}
