import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DateTimePicker, Listbox } from "./controls";

describe("themed controls", () => {
  it("selects options without using a native select", async () => {
    const onChange = vi.fn();
    const { container } = render(
      <Listbox
        value={30}
        onChange={onChange}
        label="Intervalo"
        options={[
          { value: 15, label: "15 seconds" },
          { value: 30, label: "30 seconds" },
          { value: 60, label: "1 minute" },
        ]}
      />,
    );

    expect(container.querySelector("select")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Intervalo" }));
    expect(screen.getByRole("listbox", { name: "Intervalo" })).toHaveClass(
      "z-[350]",
    );
    await userEvent.click(screen.getByRole("option", { name: "1 minute" }));
    expect(onChange).toHaveBeenCalledWith(60);
  });

  it("opens the custom calendar and applies the date and time", async () => {
    const onChange = vi.fn();
    const { container } = render(
      <DateTimePicker value="" onChange={onChange} label="Start date" />,
    );

    expect(container.querySelector('input[type="datetime-local"]')).toBeNull();
    await userEvent.click(
      screen.getByRole("button", { name: "Start date" }),
    );
    expect(screen.getByRole("dialog", { name: "Start date" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Now" }));
    await userEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(onChange).toHaveBeenCalledWith(
      expect.stringMatching(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/),
    );
  });
});
