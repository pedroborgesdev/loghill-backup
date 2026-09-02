import { DndContext } from "@dnd-kit/core";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { BlockConfigurationPanel } from "./BlockConfigurationPanel";
import { createBlock, type MonitoringBlock } from "./blockModel";
import { RuleCanvas } from "./RuleCanvas";

function groupBlock(): MonitoringBlock {
  return {
    id: "group",
    category: "condition",
    type: "group",
    connector: "and",
    negated: false,
    groupOperator: "or",
    children: [createBlock("message"), createBlock("severity")],
  };
}

describe("nested monitoring group editor", () => {
  it("renders nested conditions and exposes grouping actions", () => {
    const onGroup = vi.fn();
    const group = groupBlock();
    render(<DndContext><RuleCanvas blocks={[createBlock("log_received"), group, createBlock("send_email")]} onSelect={vi.fn()} onUpdate={vi.fn()} onDuplicate={vi.fn()} onMove={vi.fn()} onRemove={vi.fn()} onGroup={onGroup} onUngroup={vi.fn()} /></DndContext>);

    expect(screen.getByText("OR group with 2 blocks")).toBeInTheDocument();
    expect(screen.getByText("Log message")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Actions for Severity" }));
    fireEvent.click(screen.getByRole("button", { name: "Group with previous" }));
    expect(onGroup).toHaveBeenCalledWith(group.children![1].id);
  });

  it("configures the group operator and whole-group negation", () => {
    const onUpdate = vi.fn();
    const group = groupBlock();
    render(<BlockConfigurationPanel block={group} events={[]} alerts={[]} collapsed={false} onToggle={vi.fn()} onUpdate={onUpdate} />);

    expect(screen.getByText(/evaluated with the selected operator/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("switch", { name: /Negate the entire group/i }));
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ id: "group", negated: true }));
  });
});
