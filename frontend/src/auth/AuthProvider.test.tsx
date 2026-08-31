import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { request, setUnauthorizedHandler } from "../api/client";
import { AuthProvider, useAuth } from "./AuthProvider";

function Fixture() {
  const { state } = useAuth();
  return (
    <>
      <span>{state}</span>
      <button type="button" onClick={() => void request("/api/v1/senders").catch(() => undefined)}>
        Request
      </button>
    </>
  );
}

afterEach(() => {
  setUnauthorizedHandler(null);
  vi.unstubAllGlobals();
});

it("returns to the login state when an authenticated request receives 401", async () => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.includes("/auth/session")) {
      return new Response(JSON.stringify({ authenticated: true, auth_required: true }), { status: 200 });
    }
    return new Response(
      JSON.stringify({ error: { code: "UNAUTHORIZED", message: "Invalid credential" } }),
      { status: 401 },
    );
  });
  vi.stubGlobal("fetch", fetchMock);

  render(<AuthProvider><Fixture /></AuthProvider>);
  expect(await screen.findByText("authenticated")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Request" }));
  await waitFor(() => expect(screen.getByText("anonymous")).toBeInTheDocument());
});
