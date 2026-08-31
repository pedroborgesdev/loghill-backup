import { describe, expect, it } from "vitest";
import { createBlock, fromRule, insertBlock, moveBlock, toRule, validateBlocks } from "./blockModel";

describe("monitoring block model", () => {
  it("keeps trigger, conditions and actions in a valid vertical order", () => {
    const trigger = createBlock("log_received");
    const condition = createBlock("message");
    const action = createBlock("send_email");
    let blocks = insertBlock([], trigger).blocks;
    blocks = insertBlock(blocks, action).blocks;
    blocks = insertBlock(blocks, condition).blocks;
    expect(blocks.map((block) => block.category)).toEqual(["trigger", "condition", "action"]);
    const rejected = moveBlock(blocks, 2, 0);
    expect(rejected.blocks).toBe(blocks);
    expect(rejected.message).toMatch(/ações/i);
  });

  it("reports incomplete blocks and permits accessible duplication ids", () => {
    const trigger = createBlock("event_triggered");
    const action = createBlock("send_email");
    const blocks = insertBlock(insertBlock([], trigger).blocks, action).blocks;
    const problems = validateBlocks(blocks);
    expect(problems.get(trigger.id)).toBe("Select an event.");
    expect(problems.get(action.id)).toMatch(/destinatário/i);
    expect(trigger.id).not.toBe(action.id);
  });

  it("keeps the generic log trigger as a dedicated condition instead of a severity filter", () => {
    const trigger = createBlock("log_received");
    expect(trigger.condition).toMatchObject({ type: "log_received", operator: "received", value: {} });
    const saved = toRule([trigger], { name: "Logs", description: "", sender_ids: ["sender"], enabled: true, status: "active" });
    expect(saved.expression.nodes[0].condition).toMatchObject({ type: "log_received", operator: "received" });
    expect(fromRule(saved)[0]).toMatchObject({ category: "trigger", type: "log_received" });
  });

  it("migrates the legacy severity trigger back into the generic log trigger", () => {
    const legacy = {
      name: "Logs",
      description: "",
      sender_ids: ["sender"],
      enabled: true,
      status: "active" as const,
      actions: [],
      expression: {
        operator: "and" as const,
        negated: false,
        nodes: [{ connector: "and" as const, condition: { id: "cond", type: "severity" as const, operator: "in", value: { severities: ["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"] }, negated: false } }],
      },
    };
    const [block] = fromRule(legacy);
    expect(block).toMatchObject({ category: "trigger", type: "log_received" });
    expect(block.condition).toMatchObject({ type: "log_received", operator: "received", value: {} });
  });

  it("creates a sender status trigger configured as active by default", () => {
    expect(createBlock("sender_status")).toMatchObject({
      category: "trigger",
      type: "sender_status",
      condition: { type: "sender_status", operator: "became", value: { status: "online" } },
    });
  });
});
