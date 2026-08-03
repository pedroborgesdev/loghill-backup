import { request } from "./client";
import type { CreateSenderRequest, CreateSenderResponse, HealthResponse, LogPage, ReactivateSenderResponse, RotateSenderKeyResponse, Sender, SenderDependencies, SenderInstancePage, SenderPage, Settings, SettingsUpdateResponse, Summary } from "../types/api";
export const api={
  health:()=>request<HealthResponse>("/health"),
  summary:()=>request<Summary>("/api/v1/dashboard/summary"),
  senders:(query:string,signal?:AbortSignal)=>request<SenderPage>(`/api/v1/senders?${query}`,{signal}),
  sender:(id:string)=>request<Sender>(`/api/v1/senders/${encodeURIComponent(id)}`),
  senderInstances:(id:string)=>request<SenderInstancePage>(`/api/v1/senders/${encodeURIComponent(id)}/instances?page_size=1000`),
  deleteSenderInstance:(id:string,instanceID:string)=>request<void>(`/api/v1/senders/${encodeURIComponent(id)}/instances/${encodeURIComponent(instanceID)}`,{method:"DELETE"}),
  createSender:(input:CreateSenderRequest)=>request<CreateSenderResponse>("/api/v1/senders",json("POST",input)),
  checkSenderID:(id:string,signal?:AbortSignal)=>request<{id:string;available:boolean}>(`/api/v1/senders/check-id?id=${encodeURIComponent(id)}`,{signal}),
  updateSender:(id:string,input:CreateSenderRequest)=>request<Sender>(`/api/v1/senders/${encodeURIComponent(id)}`,json("PUT",input)),
  rotateSenderKey:(id:string)=>request<RotateSenderKeyResponse>(`/api/v1/senders/${encodeURIComponent(id)}/rotate-key`,{method:"POST"}),
  revokeSender:(id:string)=>request<Sender>(`/api/v1/senders/${encodeURIComponent(id)}/revoke`,{method:"POST"}),
  reactivateSender:(id:string)=>request<ReactivateSenderResponse>(`/api/v1/senders/${encodeURIComponent(id)}/reactivate`,{method:"POST"}),
  senderDependencies:(id:string)=>request<SenderDependencies>(`/api/v1/senders/${encodeURIComponent(id)}/dependencies`),
  deleteSender:(id:string,removeFromAlerts=false,removeFromEvents=false,removeFromMonitoring=false)=>{const query=new URLSearchParams();if(removeFromAlerts)query.set("remove_from_alerts","true");if(removeFromEvents)query.set("remove_from_events","true");if(removeFromMonitoring)query.set("remove_from_monitoring","true");return request<void>(`/api/v1/senders/${encodeURIComponent(id)}${query.size?`?${query}`:""}`,{method:"DELETE"})},
  logs:(id:string,query:string)=>request<LogPage>(`/api/v1/senders/${encodeURIComponent(id)}/logs?${query}`),
  downloadURL:(id:string,query:string)=>`/api/v1/senders/${encodeURIComponent(id)}/logs/download?${query}`,
  settings:()=>request<Settings>("/api/v1/settings"),
  updateSettings:(settings:Omit<Settings, "updated_at">)=>request<SettingsUpdateResponse>("/api/v1/settings",{
    method:"PUT",
    headers:{"Content-Type":"application/json"},
    body:JSON.stringify(settings),
  }),
};

function json(method:string,value:unknown):RequestInit{
  return {method,headers:{"Content-Type":"application/json"},body:JSON.stringify(value)};
}
