import { ArrowLeft, ArrowRight, Check, ShieldCheck, Zap } from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { eventsApi } from "../../api/events";
import { APIError } from "../../types/api";
import type { EventActionType, EventDefinition, EventInput } from "../../types/event";
import type { SenderOption } from "../alerts/SenderSelect";
import { SenderMultiSelect } from "../alerts/SenderSelect";
import { Button, Input, ModalCloseButton } from "../ui";
import { HTTPRequestFields } from "../http/HTTPRequestFields";
import { emptyHTTPRequest, httpRequestProblem } from "../http/httpRequestModel";
import { EventActionSelector } from "./EventActionSelector";
import { EventEmailSettings } from "./EventEmailSettings";
import { EventKeyField } from "./EventKeyField";
import { isValidEventKey } from "./eventUtils";
import { EventTemplateEditor } from "./EventTemplateEditor";
import { EventTemplatePreview } from "./EventTemplatePreview";

const steps = ["Identification", "Senders", "Action", "Template", "Review"];

function validWebhookURL(value: string) {
  try {
    const parsed = new URL(value);
    return parsed.protocol === "https:" && Boolean(parsed.hostname) && !parsed.username && !parsed.password && parsed.hostname !== "localhost";
  } catch {
    return false;
  }
}

export function EventFormDialog({ event, outlookReady, onSaved, onClose, onConfigureOutlook }: { event?: EventDefinition; outlookReady: boolean; onSaved: (event: EventDefinition) => void; onClose: () => void; onConfigureOutlook: (trigger: HTMLButtonElement) => void }) {
  const titleId = useId();
  const dialog = useRef<HTMLDivElement>(null);
  const [step, setStep] = useState(0);
  const [name, setName] = useState(event?.name ?? "");
  const [eventKey, setEventKey] = useState(event?.key ?? "");
  const [keyAvailable, setKeyAvailable] = useState(Boolean(event));
  const [senders, setSenders] = useState<SenderOption[]>(event ? event.sender_ids.map((id) => ({ id, name: id, status: "online" })) : []);
  const [actionType, setActionType] = useState<EventActionType>(event?.action_type ?? "email");
  const [recipients, setRecipients] = useState(event?.recipients ?? []);
  const [subject, setSubject] = useState(event?.subject_template ?? "");
  const [message, setMessage] = useState(event?.message_template ?? "");
  const [webhookURL, setWebhookURL] = useState(event?.webhook_url ?? "");
  const [httpRequest, setHTTPRequest] = useState(event?.http_request ?? emptyHTTPRequest());
  const [enabled, setEnabled] = useState(event?.enabled ?? true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [field, setField] = useState("");

  const emailEnabled = actionType === "email";
  const webhookEnabled = actionType === "webhook";
  const httpEnabled = actionType === "http";

  useEffect(() => {
    dialog.current?.focus();
    const close = (keyboard: KeyboardEvent) => { if (keyboard.key === "Escape" && !saving) onClose(); };
    document.addEventListener("keydown", close);
    return () => document.removeEventListener("keydown", close);
  }, [onClose, saving]);

  const errors = useMemo(() => [
    name.trim().length < 3 || name.trim().length > 100 ? "The name must be between 3 and 100 characters." : !isValidEventKey(eventKey) ? "Enter a valid event key." : !event && !keyAvailable ? "The key must be available." : "",
    senders.length > 100 ? "The limit is 100 senders." : "",
    emailEnabled && !recipients.length ? "Add at least one recipient." : emailEnabled && enabled && !outlookReady ? "Configure email or disable email delivery." : webhookEnabled && !validWebhookURL(webhookURL) ? "Enter a public HTTPS webhook URL without credentials." : httpEnabled ? httpRequestProblem(httpRequest) : "",
    emailEnabled && !subject.trim() ? "Enter the email subject." : emailEnabled && (subject.length > 200 || /[\r\n]/.test(subject)) ? "The subject must be at most 200 characters and cannot contain line breaks." : emailEnabled && !message.trim() ? "Enter the event message." : emailEnabled && message.length > 10000 ? "The message must be at most 10,000 characters." : "",
    "",
  ], [emailEnabled, enabled, event, eventKey, httpEnabled, httpRequest, keyAvailable, message, name, outlookReady, recipients.length, senders.length, subject, webhookEnabled, webhookURL]);

  const currentError = errors[step];
  const allValid = errors.slice(0, 4).every((value) => !value);
  const next = () => {
    if (currentError) { setError(currentError); return; }
    setError("");
    setStep((value) => Math.min(4, value + 1));
  };
  const save = async () => {
    if (!allValid || saving) { setError(errors.find(Boolean) || "Review the event data."); return; }
    const input: EventInput = {
      name: name.trim(), key: eventKey, sender_ids: senders.map((sender) => sender.id), action_type: actionType,
      recipients: emailEnabled ? recipients : [], subject_template: emailEnabled ? subject.trim() : "",
      message_template: emailEnabled ? message.trim() : "", webhook_url: webhookEnabled ? webhookURL.trim() : "",
      http_request: httpEnabled ? httpRequest : undefined,
      enabled,
    };
    setSaving(true); setError(""); setField("");
    try {
      const saved = event ? await eventsApi.update(event.id, input) : await eventsApi.create(input);
      onSaved(saved);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Unable to save the event.");
      if (requestError instanceof APIError) setField(requestError.field ?? "");
    } finally { setSaving(false); }
  };

  const setAction = (value: EventActionType) => { setActionType(value); setError(""); };
  const actionName = actionType === "email" ? "Send email" : actionType === "webhook" ? "Call webhook" : actionType === "http" ? "HTTP request" : "Monitoring only";

  return createPortal(
    <div className="fixed inset-0 z-[210] grid place-items-center p-3 sm:p-5">
      <button type="button" aria-label="Close form" className="absolute inset-0 bg-black/75" onClick={() => !saving && onClose()} />
      <div ref={dialog} role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1} className="relative flex max-h-[94dvh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-zinc-700 bg-[#111113] shadow-2xl shadow-black/70 outline-none">
        <header className="flex shrink-0 items-start justify-between border-b border-zinc-800 px-5 py-4">
          <div><div className="flex items-center gap-2"><Zap className="size-4 text-amber-500" /><h2 id={titleId} className="text-base font-semibold text-zinc-100">{event ? "Edit event" : "New event"}</h2></div><p className="mt-1 text-xs text-zinc-500">Run an action when the client explicitly provides this key in a log.</p></div>
          <ModalCloseButton label="Close event form" disabled={saving} onClick={onClose} />
        </header>
        <nav aria-label="Form steps" className="flex shrink-0 overflow-x-auto border-b border-zinc-800 bg-zinc-950/70 px-3">
          {steps.map((label, index) => <button key={label} type="button" disabled={saving || index > step} onClick={() => index <= step && setStep(index)} aria-current={index === step ? "step" : undefined} className={`relative min-w-max px-3 py-3 text-[10px] font-medium ${index === step ? "text-zinc-100" : index < step ? "text-zinc-400" : "text-zinc-700"}`}>{index + 1}. {label}{index === step && <span className="absolute inset-x-3 bottom-0 h-px bg-zinc-100" />}</button>)}
        </nav>
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {step === 0 && <div className="mx-auto max-w-2xl space-y-5"><label className="block text-xs font-medium text-zinc-300">Event name<Input autoFocus value={name} disabled={saving} maxLength={100} aria-invalid={field === "name"} onChange={(change) => { setName(change.target.value); setError(""); }} placeholder="Email sent successfully" className="mt-2 w-full" /></label><EventKeyField value={eventKey} onChange={(value) => { setEventKey(value); setError(""); }} immutable={Boolean(event)} disabled={saving} onAvailabilityChange={setKeyAvailable} /></div>}
          {step === 1 && <div className="mx-auto max-w-2xl"><SenderMultiSelect value={senders} onChange={(value) => { setSenders(value); setError(""); }} disabled={saving} /></div>}
          {step === 2 && <div className="mx-auto max-w-2xl space-y-5"><EventActionSelector value={actionType} disabled={saving} onChange={setAction} />{emailEnabled && <EventEmailSettings recipients={recipients} onChange={(value) => { setRecipients(value); setError(""); }} outlookReady={outlookReady} disabled={saving} onConfigureOutlook={onConfigureOutlook} />}{webhookEnabled && <label className="block text-xs font-medium text-zinc-300">Webhook URL<Input value={webhookURL} disabled={saving} aria-invalid={field === "webhook_url" || Boolean(webhookURL && !validWebhookURL(webhookURL))} onChange={(change) => { setWebhookURL(change.target.value); setError(""); }} placeholder="https://hooks.example.com/logmate" className="mt-2 w-full font-mono text-xs" /><span className="mt-2 flex items-start gap-2 text-[11px] leading-4 text-zinc-500"><ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-emerald-500" />Only public HTTPS destinations are accepted. Redirects and private networks are blocked.</span></label>}{httpEnabled && <HTTPRequestFields value={httpRequest} onChange={(value) => { setHTTPRequest(value); setError(""); }} />}</div>}
          {step === 3 && (emailEnabled ? <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(320px,.8fr)]"><EventTemplateEditor subject={subject} message={message} onSubjectChange={(value) => { setSubject(value); setError(""); }} onMessageChange={(value) => { setMessage(value); setError(""); }} disabled={saving} /><EventTemplatePreview name={name} eventKey={eventKey} subject={subject} message={message} /></div> : <div className="mx-auto max-w-2xl rounded-xl border border-zinc-800 bg-zinc-950/50 p-5 text-sm text-zinc-400">{webhookEnabled ? "The webhook receives a structured JSON payload; no text template is required." : httpEnabled ? "The HTTP body and values were configured in the previous step and support event variables." : "No template is required. This event will only be used as a monitoring trigger."}</div>)}
          {step === 4 && <div className="mx-auto max-w-2xl space-y-4"><section className="rounded-xl border border-zinc-800 bg-zinc-950/50 p-4"><h3 className="text-sm font-medium text-zinc-200">Review before saving</h3><dl className="mt-4 grid gap-4 text-xs sm:grid-cols-2"><Review label="Name" value={name} /><Review label="Key" value={eventKey} mono /><Review label="Senders" value={`${senders.length} selected`} /><Review label="Action" value={actionName} /><Review label="Recipients" value={emailEnabled ? `${recipients.length} address${recipients.length === 1 ? "" : "es"}` : "Not applicable"} /><Review label="Destination" value={webhookEnabled ? webhookURL : httpEnabled ? `${httpRequest.method} ${httpRequest.url}` : emailEnabled ? (outlookReady ? "Email configured" : "Email unavailable") : "Internal only"} mono={webhookEnabled || httpEnabled} /></dl></section><button type="button" role="switch" aria-checked={enabled} disabled={saving} onClick={() => { setEnabled((value) => !value); setError(""); }} className="flex w-full items-center justify-between rounded-xl border border-zinc-800 bg-zinc-950/60 p-4 text-left"><span><span className="block text-sm font-medium text-zinc-200">Active event</span><span className="mt-1 block text-[11px] text-zinc-600">Inactive events remain saved and do not react when the key is received.</span></span><span className={`relative h-6 w-11 rounded-full ${enabled ? "bg-emerald-600" : "bg-zinc-700"}`}><span className={`absolute top-1 size-4 rounded-full bg-white transition-transform ${enabled ? "translate-x-6" : "translate-x-1"}`} /></span></button>{emailEnabled && !outlookReady && enabled && <p className="text-xs text-amber-400">Configure email or disable the delivery action to save the active event.</p>}</div>}
          {error && <p role="alert" className="mx-auto mt-4 max-w-2xl rounded-lg border border-red-950 bg-red-950/20 px-3 py-2 text-xs text-red-300">{error}</p>}
        </div>
        <footer className="flex shrink-0 items-center justify-between gap-2 border-t border-zinc-800 bg-zinc-950 px-5 py-3"><Button onClick={() => step === 0 ? onClose() : setStep((value) => value - 1)} disabled={saving}>{step === 0 ? "Cancel" : <><ArrowLeft className="size-4" />Back</>}</Button>{step < 4 ? <Button onClick={next} disabled={saving || Boolean(currentError)}>Continue<ArrowRight className="size-4" /></Button> : <Button onClick={() => void save()} disabled={saving || !allValid}><Check className="size-4" />{saving ? "Saving..." : "Save event"}</Button>}</footer>
      </div>
    </div>, document.body,
  );
}

function Review({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div><dt className="text-zinc-600">{label}</dt><dd className={`mt-1 break-words text-zinc-300 ${mono ? "font-mono text-[10px]" : ""}`}>{value}</dd></div>;
}
