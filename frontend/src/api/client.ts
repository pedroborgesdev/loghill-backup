import { APIError } from "../types/api";
const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "";
const TIMEOUT = 15_000;
export async function request<T>(path:string,options:RequestInit={}):Promise<T>{
  const controller=new AbortController();const timer=window.setTimeout(()=>controller.abort(),TIMEOUT);
  try{
    const headers=new Headers(options.headers);headers.set("Accept","application/json");
    const adminKey=sessionStorage.getItem("admin_api_key");if(adminKey)headers.set("X-API-Key",adminKey);
    const response=await fetch(`${BASE_URL}${path}`,{...options,headers,signal:options.signal??controller.signal});
    if(!response.ok){let body:{error?:{code?:string;message?:string;field?:string}}={};try{body=await response.json()}catch{/* resposta inválida */}throw new APIError(response.status,body.error?.code??"HTTP_ERROR",body.error?.message??`HTTP ${response.status}`,body.error?.field)}
    if(response.status===204)return undefined as T;return await response.json() as T;
  }finally{window.clearTimeout(timer)}
}
