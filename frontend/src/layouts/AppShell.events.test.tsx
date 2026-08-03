import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { expect, it, vi } from "vitest";
import { AuthProvider } from "../auth/AuthProvider";
import { AppShell } from "./AppShell";

function mockFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/api/v1/auth/session")) {
        return new Response(JSON.stringify({ authenticated: true, auth_required: false }), { status: 200 });
      }
      return new Response(JSON.stringify({ status: "healthy" }), { status: 200 });
    }),
  );
}

it("shows Eventos in the persistent sidebar with active state", async () => {
  mockFetch();
  render(
    <AuthProvider>
      <MemoryRouter initialEntries={["/events"]}>
        <Routes>
          <Route element={<AppShell />}>
            <Route path="events" element={<div>Conteúdo de eventos</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </AuthProvider>,
  );
  const links = await screen.findAllByRole("link", { name: "Eventos" });
  expect(links[0]).toHaveAttribute("aria-current", "page");
  expect(screen.getByText("Conteúdo de eventos")).toBeInTheDocument();
  vi.unstubAllGlobals();
});
