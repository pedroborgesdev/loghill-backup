import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";

it("shows Monitoramento in the persistent sidebar with active state", () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ status: "healthy" }), { status: 200 })));
  render(<MemoryRouter initialEntries={["/monitoring"]}><Routes><Route element={<AppShell />}><Route path="monitoring" element={<div>Conteúdo de monitoramento</div>} /></Route></Routes></MemoryRouter>);
  const links = screen.getAllByRole("link", { name: "Monitoramento" });
  expect(links[0]).toHaveAttribute("aria-current", "page");
  expect(screen.getByText("Conteúdo de monitoramento")).toBeInTheDocument();
  vi.unstubAllGlobals();
});
