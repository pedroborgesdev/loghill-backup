import { describe, expect, it } from "vitest";
import { blockProblem, createBlock, findBlock, fromRule, groupWithPrevious, insertBlock, moveBlock, removeBlock, toRule, ungroupBlock, updateBlock, validateBlocks } from "./blockModel";

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
    expect(rejected.message).toMatch(/actions/i);
  });

  it("allows a temporal block to start a scheduled rule", () => {
    const weekday = createBlock("weekday");
    let blocks = insertBlock([], weekday).blocks;
    blocks = insertBlock(blocks, createBlock("wait_until")).blocks;
    blocks = insertBlock(blocks, createBlock("send_email")).blocks;
    expect(blocks.map((block) => block.category)).toEqual(["trigger", "condition", "action"]);
    expect(validateBlocks(blocks).has("structure")).toBe(false);
  });

  it("reports incomplete blocks and permits accessible duplication ids", () => {
    const trigger = createBlock("event_triggered");
    const action = createBlock("send_email");
    const blocks = insertBlock(insertBlock([], trigger).blocks, action).blocks;
    const problems = validateBlocks(blocks);
    expect(problems.get(trigger.id)).toBe("Select an event.");
    expect(problems.get(action.id)).toMatch(/recipient/i);
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

  it("creates and validates an HTTP action block", () => {
    const action = createBlock("send_http");
    expect(action).toMatchObject({ category: "action", type: "send_http", action: { type: "send_http", config: { method: "POST", headers: {}, cookies: {}, body: "" } } });
    expect(blockProblem(action)).toMatch(/public HTTPS URL/i);
    const configured = updateBlock([action], action.id, { action: { ...action.action!, config: { ...action.action!.config, url: "https://api.example.com/hook" } } })[0];
    expect(blockProblem(configured)).toBe("");
  });

  it("creates Wait Until as a sequential time block", () => {
    const block = createBlock("wait_until");
    expect(block).toMatchObject({
      category: "condition",
      type: "wait_until",
      connector: "and",
      negated: false,
      condition: {
        type: "wait_until",
        operator: "next_occurrence",
        value: { weekday: "monday", time: "09:00" },
      },
    });
    expect(String(block.condition?.value.timezone)).not.toBe("");
  });

  it("preserves nested AND, OR and NOT groups when a rule is edited", () => {
    const rule = {
      name: "Nested rule",
      description: "",
      sender_ids: ["sender"],
      enabled: true,
      status: "active" as const,
      expression: {
        id: "root",
        operator: "and" as const,
        negated: false,
        nodes: [
          { condition: { id: "trigger", type: "log_received" as const, operator: "received", value: {}, negated: false } },
          { connector: "and" as const, group: { id: "outer", operator: "or" as const, negated: true, nodes: [
            { condition: { id: "severity", type: "severity" as const, operator: "equals", value: { severity: "ERROR" }, negated: false } },
            { connector: "or" as const, group: { id: "inner", operator: "and" as const, negated: false, nodes: [
              { condition: { id: "message", type: "message" as const, operator: "contains", value: { text: "timeout" }, negated: false } },
              { condition: { id: "weekday", type: "weekday" as const, operator: "equals", value: { weekday: "monday" }, negated: false } },
            ] } },
          ] } },
        ],
      },
      actions: [{ id: "action", type: "trigger_event" as const, config: { event_id: "event" } }],
    };

    const saved = toRule(fromRule(rule), { name: rule.name, description: "", sender_ids: ["sender"], enabled: true, status: "active" });
    expect(saved.expression.nodes[1].group).toMatchObject({ id: "outer", operator: "or", negated: true });
    expect(saved.expression.nodes[1].group?.nodes[1].group).toMatchObject({ id: "inner", operator: "and", negated: false });
    expect(saved.expression.nodes[1].group?.nodes[1].group?.nodes).toHaveLength(2);
  });

  it("creates, edits and dissolves recursive condition groups", () => {
    const trigger = createBlock("log_received");
    const message = createBlock("message");
    const severity = { ...createBlock("severity"), connector: "or" as const };
    let blocks = [trigger, message, severity];

    const grouped = groupWithPrevious(blocks, severity.id);
    expect(grouped.message).toBe("");
    expect(findBlock(grouped.blocks, grouped.id)).toMatchObject({ type: "group", groupOperator: "or" });

    blocks = updateBlock(grouped.blocks, grouped.id, { negated: true, groupOperator: "and" });
    expect(findBlock(blocks, grouped.id)).toMatchObject({ negated: true, groupOperator: "and" });
    expect(findBlock(blocks, message.id)).toBeDefined();

    blocks = removeBlock(blocks, message.id);
    expect(validateBlocks(blocks).get(grouped.id)).toMatch(/at least two/i);
    const dissolved = ungroupBlock(grouped.blocks, grouped.id);
    expect(dissolved.blocks.map((block) => block.id)).toEqual([trigger.id, message.id, severity.id]);
  });
});
