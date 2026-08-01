import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";

it("shows Eventos in the persistent sidebar with active state", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ status: "healthy" }), { status: 200 })));
  render(<MemoryRouter initialEntries={["/events"]}><Routes><Route element={<AppShell />}><Route path="events" element={<div>Conteúdo de eventos</div>} /></Route></Routes></MemoryRouter>);
  const links = screen.getAllByRole("link", { name: "Eventos" });
  expect(links[0]).toHaveAttribute("aria-current", "page");
  expect(screen.getByText("Conteúdo de eventos")).toBeInTheDocument();
  vi.unstubAllGlobals();
});
