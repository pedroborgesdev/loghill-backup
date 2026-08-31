import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { RecipientInput } from "./RecipientInput";
import { SeveritySelector } from "./SeveritySelector";

describe("alert form controls", () => {
  it("adds, normalizes, rejects duplicates and removes recipients", async () => {
    const user = userEvent.setup();
    function Fixture() {
      const [value, setValue] = useState<string[]>([]);
      return <RecipientInput value={value} onChange={setValue} />;
    }
    render(<Fixture />);
    const input = screen.getByPlaceholderText("user@company.com");
    await user.type(input, "DEV@Example.com{enter}");
    expect(screen.getByText("dev@example.com")).toBeInTheDocument();
    await user.type(screen.getByPlaceholderText("Add another email"), " dev@example.com{enter}");
    expect(screen.getByRole("alert")).toHaveTextContent("já foi adicionado");
    await user.click(screen.getByRole("button", { name: "Remove dev@example.com" }));
    expect(screen.queryByText("dev@example.com")).not.toBeInTheDocument();
  });

  it("selects multiple severities with themed buttons", () => {
    const change = vi.fn();
    const { rerender } = render(<SeveritySelector value={["ERROR"]} onChange={change} />);
    fireEvent.click(screen.getByRole("button", { name: "FATAL" }));
    expect(change).toHaveBeenCalledWith(["ERROR", "FATAL"]);
    rerender(<SeveritySelector value={["ERROR", "FATAL"]} onChange={change} />);
    expect(screen.getByRole("button", { name: "FATAL" })).toHaveAttribute("aria-pressed", "true");
  });
});
