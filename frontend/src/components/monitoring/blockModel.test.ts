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
    expect(problems.get(trigger.id)).toBe("Selecione um evento.");
    expect(problems.get(action.id)).toMatch(/destinatário/i);
    expect(trigger.id).not.toBe(action.id);
  });

  it("restores the generic log trigger without turning it into a severity block", () => {
    const trigger = createBlock("log_received");
    const saved = toRule([trigger], { name: "Logs", description: "", sender_ids: ["sender"], enabled: true, status: "active" });
    expect(fromRule(saved)[0]).toMatchObject({ category: "trigger", type: "log_received" });

    delete saved.expression.nodes[0].condition?.value.source;
    expect(fromRule(saved)[0]).toMatchObject({ category: "trigger", type: "log_received" });
  });

  it("creates a sender status trigger configured as active by default", () => {
    expect(createBlock("sender_status")).toMatchObject({
      category: "trigger",
      type: "sender_status",
      condition: { type: "sender_status", operator: "became", value: { status: "online" } },
    });
  });
});
