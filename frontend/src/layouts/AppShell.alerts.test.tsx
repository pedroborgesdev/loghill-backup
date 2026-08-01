import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";

it("shows Alerts in the persistent sidebar with active state", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ status: "healthy" }), { status: 200 })));
  render(<MemoryRouter initialEntries={["/alerts"]}><Routes><Route element={<AppShell />}><Route path="alerts" element={<div>Conteúdo de alertas</div>} /></Route></Routes></MemoryRouter>);
  const links = screen.getAllByRole("link", { name: "Alertas" });
  expect(links[0]).toHaveAttribute("aria-current", "page");
  expect(screen.getByText("Conteúdo de alertas")).toBeInTheDocument();
  vi.unstubAllGlobals();
});
