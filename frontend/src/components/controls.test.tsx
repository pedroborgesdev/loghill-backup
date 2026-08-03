import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DateTimePicker, Listbox } from "./controls";

describe("controles temáticos", () => {
  it("seleciona opções sem utilizar select nativo", async () => {
    const onChange = vi.fn();
    const { container } = render(
      <Listbox
        value={30}
        onChange={onChange}
        label="Intervalo"
        options={[
          { value: 15, label: "15 segundos" },
          { value: 30, label: "30 segundos" },
          { value: 60, label: "1 minuto" },
        ]}
      />,
    );

    expect(container.querySelector("select")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Intervalo" }));
    expect(screen.getByRole("listbox", { name: "Intervalo" })).toHaveClass(
      "z-[350]",
    );
    await userEvent.click(screen.getByRole("option", { name: "1 minuto" }));
    expect(onChange).toHaveBeenCalledWith(60);
  });

  it("abre calendário próprio e aplica data e horário", async () => {
    const onChange = vi.fn();
    const { container } = render(
      <DateTimePicker value="" onChange={onChange} label="Data inicial" />,
    );

    expect(container.querySelector('input[type="datetime-local"]')).toBeNull();
    await userEvent.click(
      screen.getByRole("button", { name: "Data inicial" }),
    );
    expect(screen.getByRole("dialog", { name: "Data inicial" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Agora" }));
    await userEvent.click(screen.getByRole("button", { name: "Aplicar" }));
    expect(onChange).toHaveBeenCalledWith(
      expect.stringMatching(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/),
    );
  });
});
