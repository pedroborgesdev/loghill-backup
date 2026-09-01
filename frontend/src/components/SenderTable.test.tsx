import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import type { Sender } from "../types/api";
import { groupSenders } from "../utils/senders";
import { SenderTable } from "./SenderTable";

afterEach(() => cleanup());

function sender(id: string, name: string, logs: number): Sender {
  return {
    id,
    name,
    status: "online",
    created_at: "2026-07-30T12:00:00Z",
    updated_at: "2026-07-30T12:00:00Z",
    last_activity_at: "2026-07-30T12:00:00Z",
    last_healthcheck_at: null,
    inactive_at: null,
    compacted_at: null,
    expires_at: null,
    expired_at: null,
    log_line_count: logs,
    log_file_size: logs * 10,
  };
}

describe("sender grouping", () => {
  const items = [
    sender("simulador-teste-a1", "simulador-teste", 10),
    sender("simulador-teste-b2", "simulador-teste", 20),
    sender("worker-c3", "worker", 5),
  ];

  it("consolidates name, logs, and instance count", () => {
    const groups = groupSenders(items);
    expect(groups).toHaveLength(2);
    expect(groups[0].items).toHaveLength(2);
    expect(groups[0].logLineCount).toBe(30);
    expect(groups[0].logFileSize).toBe(300);
  });

  it("replaces the table with instance selection when opening a group", async () => {
    render(
      <MemoryRouter>
        <SenderTable items={items} />
      </MemoryRouter>,
    );

    const groupRows = screen.getAllByRole("button", {
      name: "Open simulador-teste group with 2 instances",
    });
    await userEvent.click(groupRows[0]);

    expect(screen.getByText("Choose one of 2 instances")).toBeInTheDocument();
    expect(screen.getAllByText("simulador-teste-a1")).not.toHaveLength(0);
    expect(screen.getAllByText("simulador-teste-b2")).not.toHaveLength(0);

    await userEvent.click(
      screen.getByRole("button", { name: "Back to groups" }),
    );
    expect(
      screen.getAllByRole("button", {
        name: "Open simulador-teste group with 2 instances",
      }),
    ).not.toHaveLength(0);
  });

  it("opens details when any part of the row is clicked", async () => {
    render(
      <MemoryRouter initialEntries={["/senders"]}>
        <Routes>
          <Route path="/senders" element={<SenderTable items={items} />} />
          <Route path="/senders/:id" element={<p>Sender details</p>} />
        </Routes>
      </MemoryRouter>,
    );

    await userEvent.click(
      screen.getAllByRole("link", { name: "Open sender worker" })[0],
    );

    expect(screen.getByText("Sender details")).toBeInTheDocument();
  });
});
