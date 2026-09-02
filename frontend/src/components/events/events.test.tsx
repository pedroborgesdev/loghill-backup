import { fireEvent, render, screen, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { EventBadge } from "./EventBadge";
import { EventKeyField } from "./EventKeyField";
import { isValidEventKey } from "./eventUtils";
import { EventTemplatePreview } from "./EventTemplatePreview";
import { EventActionSelector } from "./EventActionSelector";
import { normalizeEvent } from "../../api/events";
import type { EventDefinition } from "../../types/event";

it("validates stable event keys", () => {
  expect(isValidEventKey("processing_completed")).toBe(true);
  expect(isValidEventKey("boleto-gerado")).toBe(true);
  expect(isValidEventKey("../segredo")).toBe(false);
  expect(isValidEventKey("Com Espaço")).toBe(false);
});

it("copies an event badge without painting the whole log row", () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
  render(<EventBadge eventKey="boleto_gerado" />);
  fireEvent.click(screen.getByRole("button", { name: /copy event key/i }));
  expect(writeText).toHaveBeenCalledWith("boleto_gerado");
  expect(screen.getByText("EVENT")).toBeInTheDocument();
});

it("keeps an existing event key immutable", () => {
  render(<EventKeyField value="evento_existente" onChange={vi.fn()} immutable />);
  expect(screen.getByLabelText("Event key")).toHaveAttribute("readonly");
  expect(screen.getByText(/immutable after creation/i)).toBeInTheDocument();
});

it("renders preview content as text", () => {
  render(<EventTemplatePreview name="Teste" eventKey="evento_teste" subject="Assunto" message={'<script>alert("x")</script> {{metadata.ausente}}'} />);
  expect(screen.getByText(/<script>alert\("x"\)<\/script>/)).toBeInTheDocument();
  expect(document.querySelector("script")).toBeNull();
});

it("selects webhook as an event action", () => {
  const onChange = vi.fn();
  render(<EventActionSelector value="none" onChange={onChange} />);
  fireEvent.click(screen.getByRole("radio", { name: /call webhook/i }));
  expect(onChange).toHaveBeenCalledWith("webhook");
});

it("selects SMS as an event action", () => {
  const onChange = vi.fn();
  const view = render(<EventActionSelector value="none" onChange={onChange} />);
  fireEvent.click(within(view.container).getByRole("radio", { name: /send sms/i }));
  expect(onChange).toHaveBeenCalledWith("sms");
});

it("normalizes null collections returned by legacy event records", () => {
  const event = normalizeEvent({
    id: "evt_legacy",
    name: "Evento legado",
    key: "evento_legado",
    sender_ids: null,
    action_type: "none",
    recipients: null,
    subject_template: "",
    message_template: "",
    enabled: false,
    created_at: "2026-08-01T00:00:00Z",
    updated_at: "2026-08-01T00:00:00Z",
    trigger_count: 0,
    delivery_count: 0,
    failure_count: 0,
    test_delivery_count: 0,
  } as unknown as EventDefinition);

  expect(event.sender_ids).toEqual([]);
  expect(event.recipients).toEqual([]);
  expect(event.phone_numbers).toEqual([]);
});
