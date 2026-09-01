import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import App from "./App";

afterEach(() => {
  vi.unstubAllGlobals();
  window.history.replaceState({}, "", "/");
});

it("redirects an anonymous session to the login URL", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(
    JSON.stringify({ authenticated: false, auth_required: true }),
    { status: 200 },
  )));
  window.history.replaceState({}, "", "/senders");

  const view = render(<App />);

  await waitFor(() => expect(window.location.pathname).toBe("/login"));
  await screen.findByRole("heading", { name: "LogHill" });
  view.unmount();
});
