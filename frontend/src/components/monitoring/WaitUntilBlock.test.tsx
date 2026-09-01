import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { BlockConfigurationPanel } from "./BlockConfigurationPanel";
import { BlockLibrary } from "./BlockLibrary";
import { createBlock } from "./blockModel";

describe("Wait Until monitoring block", () => {
  it("is available in the Time library group", () => {
    const onAdd = vi.fn();
    render(<BlockLibrary emailReady onAdd={onAdd} collapsed={false} onToggle={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /Wait Until/i }));
    expect(onAdd).toHaveBeenCalledWith("wait_until");
  });

  it("configures weekday, time, and timezone", () => {
    const block = createBlock("wait_until");
    render(<BlockConfigurationPanel block={block} events={[]} alerts={[]} collapsed={false} onToggle={vi.fn()} onUpdate={vi.fn()} />);
    expect(screen.getByText("The flow remains pending and continues at the next occurrence of this weekday and time.")).toBeInTheDocument();
    expect(screen.getByLabelText("At")).toHaveValue("09:00");
    expect(screen.getByLabelText("Timezone")).not.toHaveValue("");
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });
});
