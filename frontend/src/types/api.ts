export type SenderStatus = "never_connected" | "online" | "inactive" | "archived" | "expired" | "revoked";
export type LogSeverity = "TRACE" | "DEBUG" | "INFO" | "WARN" | "ERROR" | "FATAL";
export interface Sender {
  id:string; name:string; description?:string; status:SenderStatus; created_at:string; updated_at:string;
  key_prefix?:string; key_rotated_at?:string|null;
  last_activity_at:string|null; last_healthcheck_at:string|null; inactive_at:string|null;
  compacted_at:string|null; expires_at:string|null; expired_at:string|null;
  log_line_count:number; log_file_size:number; recent_error_count?:number; instance_count?:number;
}
export interface SenderCredentials { sender_key:string; displayed_once:true }
export interface CreateSenderRequest { name:string; description?:string }
export interface CreateSenderResponse { sender:Sender; credentials:SenderCredentials }
export interface RotateSenderKeyResponse { sender_id:string; credentials:SenderCredentials; rotated_at:string }
export interface ReactivateSenderResponse { sender:Sender; credentials:SenderCredentials }
export interface SenderDependencies { sender_id:string; alert_rules:number; events:number; monitoring_rules:number }
export interface LogEntry { timestamp:string; sender?:string; instance_id?:string; severity:LogSeverity; message:string; event?:string; event_occurrence_id?:string; metadata?:Record<string,unknown> }
export interface Pagination { page:number; page_size:number; returned?:number; total:number; total_pages:number }
export interface SenderPage { items:Sender[]; pagination:Pagination }
export interface LogPage { sender:string; items:LogEntry[]; pagination:Pagination }
export interface SenderInstance { id:string; created_at:string; last_activity_at?:string|null; last_healthcheck_at?:string|null; log_line_count:number; log_file_size:number; legacy?:boolean; status:"online"|"inactive" }
export interface SenderInstancePage { sender:string; items:SenderInstance[]; pagination:Pagination }
export interface Summary { senders:Record<"total"|"never_connected"|"online"|"inactive"|"expired"|"revoked",number>; logs:Record<"total"|"last_24_hours"|"errors_last_24_hours"|"fatal_last_24_hours",number>; executions?:import("./execution").ExecutionSummary }
export interface HealthResponse {
  status:string;
  time:string;
  uptime_seconds:number;
  senders:Record<"total"|"never_connected"|"online"|"inactive"|"expired"|"revoked",number>;
  storage:{writable:boolean;path:string};
}
export type StorageUnit = "lines" | "mb";
export interface NumberUnitValue { value:number; unit:StorageUnit }
export interface Settings {
  log_limit:NumberUnitValue;
  inactive_preservation:NumberUnitValue;
  inactive_after_seconds:number;
  delete_inactive_after_days:number;
  updated_at:string;
}
export interface SettingsUpdateResponse {
  success:boolean;
  settings:Omit<Settings, "updated_at">;
  updated_at:string;
}
export class APIError extends Error {
  constructor(
    public status:number,
    public code:string,
    message:string,
    public field?:string,
  ){
    super(message);
  }
}
