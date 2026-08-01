import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { EventBadge } from "./EventBadge";
import { EventKeyField } from "./EventKeyField";
import { isValidEventKey } from "./eventUtils";
import { EventTemplatePreview } from "./EventTemplatePreview";

it("validates stable event keys", () => {
  expect(isValidEventKey("processamento_finalizado")).toBe(true);
  expect(isValidEventKey("boleto-gerado")).toBe(true);
  expect(isValidEventKey("../segredo")).toBe(false);
  expect(isValidEventKey("Com Espaço")).toBe(false);
});

it("copies an event badge without painting the whole log row", () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
  render(<EventBadge eventKey="boleto_gerado" />);
  fireEvent.click(screen.getByRole("button", { name: /copiar chave do evento/i }));
  expect(writeText).toHaveBeenCalledWith("boleto_gerado");
  expect(screen.getByText("EVENT")).toBeInTheDocument();
});

it("keeps an existing event key immutable", () => {
  render(<EventKeyField value="evento_existente" onChange={vi.fn()} immutable />);
  expect(screen.getByLabelText("Chave do evento")).toHaveAttribute("readonly");
  expect(screen.getByText(/imutável depois da criação/i)).toBeInTheDocument();
});

it("renders preview content as text", () => {
  render(<EventTemplatePreview name="Teste" eventKey="evento_teste" subject="Assunto" message={'<script>alert("x")</script> {{metadata.ausente}}'} />);
  expect(screen.getByText(/<script>alert\("x"\)<\/script>/)).toBeInTheDocument();
  expect(document.querySelector("script")).toBeNull();
});
