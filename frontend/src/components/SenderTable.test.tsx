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

describe("agrupamento de senders", () => {
  const items = [
    sender("simulador-teste-a1", "simulador-teste", 10),
    sender("simulador-teste-b2", "simulador-teste", 20),
    sender("worker-c3", "worker", 5),
  ];

  it("consolida nome, logs e tamanho das instancias", () => {
    const groups = groupSenders(items);
    expect(groups).toHaveLength(2);
    expect(groups[0].items).toHaveLength(2);
    expect(groups[0].logLineCount).toBe(30);
    expect(groups[0].logFileSize).toBe(300);
  });

  it("troca a tabela pela escolha de instancias ao abrir um grupo", async () => {
    render(
      <MemoryRouter>
        <SenderTable items={items} />
      </MemoryRouter>,
    );

    const groupRows = screen.getAllByRole("button", {
      name: "Abrir grupo simulador-teste com 2 instâncias",
    });
    await userEvent.click(groupRows[0]);

    expect(screen.getByText("Escolha uma das 2 instâncias")).toBeInTheDocument();
    expect(screen.getAllByText("simulador-teste-a1")).not.toHaveLength(0);
    expect(screen.getAllByText("simulador-teste-b2")).not.toHaveLength(0);

    await userEvent.click(
      screen.getByRole("button", { name: "Voltar aos grupos" }),
    );
    expect(
      screen.getAllByRole("button", {
        name: "Abrir grupo simulador-teste com 2 instâncias",
      }),
    ).not.toHaveLength(0);
  });

  it("abre os detalhes ao clicar em qualquer parte da linha", async () => {
    render(
      <MemoryRouter initialEntries={["/senders"]}>
        <Routes>
          <Route path="/senders" element={<SenderTable items={items} />} />
          <Route path="/senders/:id" element={<p>Detalhes do sender</p>} />
        </Routes>
      </MemoryRouter>,
    );

    await userEvent.click(
      screen.getAllByRole("link", { name: "Abrir sender worker" })[0],
    );

    expect(screen.getByText("Detalhes do sender")).toBeInTheDocument();
  });
});
