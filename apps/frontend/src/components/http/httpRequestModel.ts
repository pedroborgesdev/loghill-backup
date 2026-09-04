import type { HTTPRequestConfig } from "../../types/event";

export const httpMethods = ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE"].map((value) => ({ value, label: value }));

export const emptyHTTPRequest = (): HTTPRequestConfig => ({ method: "POST", url: "", headers: {}, cookies: {}, body: "" });

export function httpRequestProblem(request: HTTPRequestConfig) {
  if (!httpMethods.some((method) => method.value === request.method)) return "Select a valid HTTP method.";
  try {
    const url = new URL(request.url);
    const hostname = url.hostname.toLowerCase();
    if (url.protocol !== "https:" || !hostname || url.username || url.password || url.hash || hostname === "localhost" || hostname.endsWith(".localhost")) return "Enter a public HTTPS URL without credentials.";
  } catch {
    return "Enter a public HTTPS URL without credentials.";
  }
  if (new TextEncoder().encode(request.body ?? "").length > 64 * 1024) return "The HTTP body must be at most 64 KiB.";
  return "";
}
