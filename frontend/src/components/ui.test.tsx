import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import {
  ConfirmDialog,
  IconButton,
  Input,
  SearchInput,
  StatusBadge,
} from "./ui";
import { restoreFocusWithoutTooltip } from "../utils/tooltipFocus";

describe("interface components", () => {
  it("marks an invalid minimum only after editing and leaving the field", async () => {
    function Fixture() {
      const [value, setValue] = useState("");
      return <Input aria-label="Name" value={value} minLength={3} onChange={(event) => setValue(event.target.value)} />;
    }
    const user = userEvent.setup();
    render(<Fixture />);
    const input = screen.getByRole("textbox", { name: "Name" });

    expect(input).not.toHaveAttribute("aria-invalid");
    await user.click(input);
    await user.tab();
    expect(input).not.toHaveAttribute("aria-invalid");

    await user.click(input);
    await user.type(input, "ab");
    expect(input).not.toHaveAttribute("aria-invalid");
    await user.tab();
    expect(input).toHaveAttribute("aria-invalid", "true");

    await user.click(input);
    await user.type(input, "c");
    expect(input).not.toHaveAttribute("aria-invalid");
  });

  it("displays the status with accessible text", () => {
    render(<StatusBadge status="online" />);
    expect(screen.getByText("Online")).toBeInTheDocument();
  });

  it("clears a search without changing the field width", async () => {
    const onChange = vi.fn();
    render(
      <SearchInput
        value="worker"
        onChange={onChange}
        placeholder="Search sender"
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(onChange).toHaveBeenCalledWith("");
  });

  it("blocks live search and requests an action before editing", async () => {
    const onChange = vi.fn();
    const onBlocked = vi.fn();
    render(
      <SearchInput
        value=""
        onChange={onChange}
        blocked
        onBlocked={onBlocked}
        placeholder="Search logs"
      />,
    );
    const input = screen.getByRole("textbox", { name: "Search logs" });
    await userEvent.click(input);
    await userEvent.type(input, "error");
    expect(onBlocked).toHaveBeenCalled();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("confirms pausing through an accessible dialog", async () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmDialog
        open
        title="Pause logs"
        description="The list must remain stable."
        confirmLabel="Pause and continue"
        onClose={vi.fn()}
        onConfirm={onConfirm}
      />,
    );
    expect(screen.getByRole("dialog", { name: "Pause logs" })).toBeVisible();
    await userEvent.click(
      screen.getByRole("button", { name: "Pause and continue" }),
    );
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("closes the button description on click and constrains it to the viewport", async () => {
    const user = userEvent.setup();
    render(
      <IconButton label="A very long button description">
        A
      </IconButton>,
    );
    const button = screen.getByRole("button", {
      name: "A very long button description",
    });

    await user.hover(button);
    const tooltip = screen.getByRole("tooltip");
    expect(tooltip).toHaveClass("fixed");
    expect(tooltip).toHaveClass("z-[300]");
    expect(tooltip).toHaveClass("max-w-[calc(100vw-16px)]");

    await user.click(button);
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });

  it("keeps only one button description open", () => {
    render(
      <>
        <IconButton label="First description">A</IconButton>
        <IconButton label="Second description">B</IconButton>
      </>,
    );

    fireEvent.mouseEnter(
      screen.getByRole("button", { name: "First description" }),
    );
    expect(screen.getByText("First description")).toBeInTheDocument();

    fireEvent.focus(
      screen.getByRole("button", { name: "Second description" }),
    );
    expect(screen.queryByText("First description")).not.toBeInTheDocument();
    expect(screen.getByText("Second description")).toBeInTheDocument();
  });

  it("does not reopen the description when restoring focus after a modal", () => {
    render(<IconButton label="Edit rule">A</IconButton>);
    const button = screen.getByRole("button", { name: "Edit rule" });
    fireEvent.focus(button);
    expect(screen.getByRole("tooltip")).toBeInTheDocument();
    fireEvent.blur(button);

    restoreFocusWithoutTooltip(button);

    expect(button).toHaveFocus();
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });
});
