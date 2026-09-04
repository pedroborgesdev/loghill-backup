import type { LogicalOperator, MonitoringAction, MonitoringCondition, MonitoringNode, MonitoringRuleInput } from "../../types/monitoring";

export type BlockCategory = "trigger" | "condition" | "action";
export interface MonitoringBlock {
  id: string;
  category: BlockCategory;
  type: string;
  connector: LogicalOperator;
  negated: boolean;
  condition?: MonitoringCondition;
  action?: MonitoringAction;
  children?: MonitoringBlock[];
  groupOperator?: LogicalOperator;
}
export type LibraryBlockType = "event_triggered" | "alert_triggered" | "sender_status" | "log_received" | "message" | "severity" | "metadata" | "time" | "weekday" | "date" | "wait_until" | "send_email" | "send_http" | "trigger_event";
export const temporaryID = () => `temp_${crypto.randomUUID()}`;

const legacySeverities = ["TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"];
const temporalTypes = new Set(["time", "weekday", "date", "wait_until"]);

export function createBlock(type: LibraryBlockType): MonitoringBlock {
  const id = temporaryID();
  if (type === "send_email") return { id, category: "action", type, connector: "and", negated: false, action: { id, type: "send_email", config: { recipients: [], subject: "Monitoring: {{rule.name}}", message: "Rule {{rule.name}} matched for {{sender.name}}." } } };
  if (type === "send_http") return { id, category: "action", type, connector: "and", negated: false, action: { id, type: "send_http", config: { method: "POST", url: "", headers: {}, cookies: {}, body: "" } } };
  if (type === "trigger_event") return { id, category: "action", type, connector: "and", negated: false, action: { id, type: "trigger_event", config: { event_id: "", message: "Monitoring rule matched", severity: "INFO" } } };
  if (type === "log_received") return { id, category: "trigger", type, connector: "and", negated: false, condition: { id, type: "log_received", operator: "received", value: {}, negated: false } };

  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  const defaults: Record<string, { operator: string; value: Record<string, unknown> }> = {
    event_triggered: { operator: "triggered", value: { event_key: "", window_minutes: 0 } },
    alert_triggered: { operator: "triggered", value: { alert_id: "", window_minutes: 0 } },
    sender_status: { operator: "became", value: { status: "online" } },
    message: { operator: "contains", value: { text: "" } },
    severity: { operator: "equals", value: { severity: "ERROR" } },
    metadata: { operator: "exists", value: { path: "", value: "" } },
    time: { operator: "between", value: { start: "12:00", end: "17:00" } },
    weekday: { operator: "equals", value: { weekday: "monday" } },
    date: { operator: "between", value: { start: "", end: "" } },
    wait_until: { operator: "next_occurrence", value: { weekday: "monday", time: "09:00", timezone } },
  };
  const value = defaults[type];
  return { id, category: type === "event_triggered" || type === "alert_triggered" || type === "sender_status" ? "trigger" : "condition", type, connector: "and", negated: false, condition: { id, type: type as MonitoringCondition["type"], operator: value.operator, value: value.value, negated: false } };
}

function isLogReceived(condition: MonitoringCondition, firstCondition: boolean) {
  if (condition.type === "log_received") return true;
  const values = condition.value.severities;
  return firstCondition && condition.type === "severity" && condition.operator === "in" && (condition.value.source === "log_received" || (Array.isArray(values) && legacySeverities.every((value) => values.includes(value))));
}
function logReceivedCondition(id: string): MonitoringCondition { return { id, type: "log_received", operator: "received", value: {}, negated: false }; }

function blocksFromNodes(nodes: MonitoringNode[], state: { first: boolean }): MonitoringBlock[] {
  const blocks: MonitoringBlock[] = [];
  for (const node of nodes) {
    if (node.condition) {
      const id = node.condition.id ?? temporaryID();
      const firstCondition = state.first;
      state.first = false;
      const logReceived = isLogReceived(node.condition, firstCondition);
      blocks.push({ id, category: firstCondition ? "trigger" : "condition", type: logReceived ? "log_received" : node.condition.type, connector: node.connector ?? "and", negated: node.condition.negated, condition: logReceived ? logReceivedCondition(id) : { ...node.condition, id } });
      continue;
    }
    if (!node.group) continue;
    const children = blocksFromNodes(node.group.nodes, state);
    blocks.push({ id: node.group.id ?? temporaryID(), category: children.some((child) => child.category === "trigger") ? "trigger" : "condition", type: "group", connector: node.connector ?? "and", negated: node.group.negated, groupOperator: node.group.operator, children });
  }
  return blocks;
}

export function fromRule(rule: MonitoringRuleInput): MonitoringBlock[] {
  const state = { first: true };
  let logical = blocksFromNodes(rule.expression.nodes, state);
  if (rule.expression.operator !== "and" || rule.expression.negated) {
    logical = [{ id: rule.expression.id ?? temporaryID(), category: logical.some((block) => block.category === "trigger") ? "trigger" : "condition", type: "group", connector: "and", negated: rule.expression.negated, groupOperator: rule.expression.operator, children: logical }];
  }
  const actions = rule.actions.map((action) => ({ id: action.id ?? temporaryID(), category: "action" as const, type: action.type, connector: "and" as const, negated: false, action: { ...action } }));
  return [...logical, ...actions];
}

function nodeFromBlock(block: MonitoringBlock): MonitoringNode | undefined {
  if (block.condition) return { connector: block.connector, condition: { ...block.condition, id: block.id, negated: block.negated } };
  if (!block.children) return undefined;
  return { connector: block.connector, group: { id: block.id, operator: block.groupOperator ?? "and", negated: block.negated, nodes: block.children.map(nodeFromBlock).filter((node): node is MonitoringNode => Boolean(node)) } };
}

export function toRule(blocks: MonitoringBlock[], base: Omit<MonitoringRuleInput, "expression" | "actions">): MonitoringRuleInput {
  const logical = blocks.filter((block) => block.category !== "action");
  const nodes = logical.map(nodeFromBlock).filter((node): node is MonitoringNode => Boolean(node));
  const actions = blocks.filter((block) => block.action).map((block) => ({ ...block.action!, id: block.id }));
  const onlyGroup = logical.length === 1 && logical[0].children ? logical[0] : undefined;
  const expression = onlyGroup ? { id: onlyGroup.id, operator: onlyGroup.groupOperator ?? "and", negated: onlyGroup.negated, nodes: onlyGroup.children!.map(nodeFromBlock).filter((node): node is MonitoringNode => Boolean(node)) } : { operator: "and" as const, negated: false, nodes };
  return { ...base, expression, actions };
}

export function findBlock(blocks: MonitoringBlock[], id?: string): MonitoringBlock | undefined {
  if (!id) return undefined;
  for (const block of blocks) {
    if (block.id === id) return block;
    const nested = block.children && findBlock(block.children, id);
    if (nested) return nested;
  }
  return undefined;
}

export function updateBlock(blocks: MonitoringBlock[], id: string, change: Partial<MonitoringBlock>): MonitoringBlock[] {
  return blocks.map((block) => block.id === id ? { ...block, ...change } : block.children ? { ...block, children: updateBlock(block.children, id, change) } : block);
}

export function removeBlock(blocks: MonitoringBlock[], id: string): MonitoringBlock[] {
  return blocks.filter((block) => block.id !== id).map((block) => {
    if (!block.children) return block;
    const children = removeBlock(block.children, id);
    return { ...block, children, category: children.some((child) => child.category === "trigger") ? "trigger" : "condition" };
  });
}

function replaceSiblings(blocks: MonitoringBlock[], id: string, transform: (siblings: MonitoringBlock[], index: number) => MonitoringBlock[]): { blocks: MonitoringBlock[]; found: boolean } {
  const index = blocks.findIndex((block) => block.id === id);
  if (index >= 0) return { blocks: transform(blocks, index), found: true };
  for (let current = 0; current < blocks.length; current++) {
    if (!blocks[current].children) continue;
    const nested = replaceSiblings(blocks[current].children!, id, transform);
    if (nested.found) {
      const next = [...blocks];
      next[current] = { ...next[current], children: nested.blocks };
      return { blocks: next, found: true };
    }
  }
  return { blocks, found: false };
}

function cloneWithNewIDs(block: MonitoringBlock): MonitoringBlock {
  const copy = structuredClone(block);
  const assign = (item: MonitoringBlock) => { item.id = temporaryID(); if (item.condition) item.condition.id = item.id; if (item.action) item.action.id = item.id; item.children?.forEach(assign); };
  assign(copy);
  return copy;
}

export function duplicateBlock(blocks: MonitoringBlock[], id: string) {
  let copyID = "";
  let message = "";
  const result = replaceSiblings(blocks, id, (siblings, index) => {
    if (siblings[index].category === "trigger") { message = "The initial trigger cannot be duplicated."; return siblings; }
    const copy = cloneWithNewIDs(siblings[index]); copyID = copy.id; const next = [...siblings]; next.splice(index + 1, 0, copy); return next;
  });
  return { blocks: result.blocks, id: copyID, message: result.found ? message : "Block not found." };
}

function blockDepth(block: MonitoringBlock): number { return block.children?.length ? 1 + Math.max(...block.children.map(blockDepth)) : 0; }
function depthOf(blocks: MonitoringBlock[], id: string, depth = 1): number | undefined {
  for (const block of blocks) {
    if (block.id === id) return depth;
    if (block.children) { const nested = depthOf(block.children, id, depth + 1); if (nested !== undefined) return nested; }
  }
  return undefined;
}

export function groupWithPrevious(blocks: MonitoringBlock[], id: string) {
  const parentDepth = (depthOf(blocks, id) ?? 1) - 1;
  let groupID = "";
  let message = "Select a condition that has a previous condition.";
  const result = replaceSiblings(blocks, id, (siblings, index) => {
    if (index < 1 || siblings[index].category === "action" || siblings[index - 1].category === "action") return siblings;
    const previous = siblings[index - 1];
    const current = siblings[index];
    if (parentDepth + 2 + Math.max(blockDepth(previous), blockDepth(current)) > 5) { message = "A rule can contain at most five nested group levels."; return siblings; }
    groupID = temporaryID();
    const group: MonitoringBlock = { id: groupID, category: previous.category === "trigger" || current.category === "trigger" ? "trigger" : "condition", type: "group", connector: previous.connector, negated: false, groupOperator: current.connector === "or" ? "or" : "and", children: [{ ...previous, connector: "and" }, { ...current, connector: "and" }] };
    const next = [...siblings]; next.splice(index - 1, 2, group); message = ""; return next;
  });
  return { blocks: result.blocks, id: groupID, message: result.found ? message : "Block not found." };
}

export function ungroupBlock(blocks: MonitoringBlock[], id: string) {
  let message = "Select a group to dissolve.";
  const result = replaceSiblings(blocks, id, (siblings, index) => {
    const group = siblings[index];
    if (!group.children) return siblings;
    const children = group.children.map((child, childIndex) => ({ ...child, connector: childIndex === 0 ? group.connector : child.connector }));
    const next = [...siblings]; next.splice(index, 1, ...children); message = ""; return next;
  });
  return { blocks: result.blocks, message: result.found ? message : "Block not found." };
}

export function blockRank(block: MonitoringBlock) { return block.category === "trigger" ? 0 : block.category === "condition" ? 1 : 2; }
export function validOrder(blocks: MonitoringBlock[]) {
  let rank = -1, triggers = 0;
  for (const block of blocks) { const next = blockRank(block); if (next < rank) return false; rank = next; if (block.category === "trigger") triggers++; }
  return triggers === 1 && blocks[0]?.category === "trigger";
}

export function insertBlock(blocks: MonitoringBlock[], block: MonitoringBlock, index?: number) {
  const next = [...blocks];
  if (block.category === "condition" && !next.some((item) => item.category === "trigger") && temporalTypes.has(block.type)) block = { ...block, category: "trigger" };
  if (block.category === "trigger") { if (next.some((item) => item.category === "trigger")) return { blocks, message: "The rule already has an initial trigger." }; next.unshift(block); return { blocks: next, message: "" }; }
  if (block.category === "condition") { if (!next.some((item) => item.category === "trigger")) return { blocks, message: "Add a trigger before conditions." }; const firstAction = next.findIndex((item) => item.category === "action"); const maximum = firstAction < 0 ? next.length : firstAction; next.splice(Math.max(1, Math.min(index ?? maximum, maximum)), 0, block); return { blocks: next, message: "" }; }
  if (!next.some((item) => item.category === "trigger")) return { blocks, message: "Add a trigger before actions." };
  next.push(block); return { blocks: next, message: "" };
}

export function moveBlock(blocks: MonitoringBlock[], from: number, to: number) {
  if (from < 0 || to < 0 || from >= blocks.length || to >= blocks.length) return { blocks, message: "Invalid position." };
  const next = [...blocks]; const [block] = next.splice(from, 1); next.splice(to, 0, block);
  if (!validOrder(next)) return { blocks, message: block.category === "action" ? "Actions must remain after conditions." : "Triggers and conditions must remain before actions." };
  return { blocks: next, message: "" };
}

export function moveBlockByID(blocks: MonitoringBlock[], id: string, direction: -1 | 1) {
  let message = "Invalid position.";
  const result = replaceSiblings(blocks, id, (siblings, index) => {
    const target = index + direction;
    if (target < 0 || target >= siblings.length) return siblings;
    if (!siblings.some((block) => block.category === "trigger" || block.category === "action")) {
      const next = [...siblings]; const [block] = next.splice(index, 1); next.splice(target, 0, block); message = ""; return next;
    }
    const moved = moveBlock(siblings, index, target); message = moved.message; return moved.blocks;
  });
  return { blocks: result.blocks, message: result.found ? message : "Block not found." };
}

export function blockProblem(block: MonitoringBlock): string {
  if (block.children) return block.children.length < 2 ? "A group needs at least two conditions." : "";
  if (block.category === "trigger" && block.type === "event_triggered" && !block.condition?.value.event_key) return "Select an event.";
  if (block.category === "trigger" && block.type === "alert_triggered" && !block.condition?.value.alert_id) return "Select an alert.";
  if (block.type === "message" && !String(block.condition?.value.text ?? "").trim()) return "Enter the text to search for.";
  if (block.type === "metadata" && !String(block.condition?.value.path ?? "").trim()) return "Enter the metadata path.";
  if (block.type === "wait_until" && (!block.condition?.value.weekday || !block.condition.value.time || !String(block.condition.value.timezone ?? "").trim())) return "Select a weekday, time, and timezone.";
  if (block.type === "trigger_event" && !block.action?.config.event_id) return "Select a target event.";
  if (block.type === "send_email" && (!Array.isArray(block.action?.config.recipients) || !block.action?.config.recipients.length)) return "Select at least one recipient.";
  if (block.type === "send_http") {
    try {
      const url = new URL(String(block.action?.config.url ?? ""));
      const hostname = url.hostname.toLowerCase();
      if (url.protocol !== "https:" || !hostname || url.username || url.password || url.hash || hostname === "localhost" || hostname.endsWith(".localhost")) return "Enter a public HTTPS URL without credentials.";
    } catch {
      return "Enter a public HTTPS URL without credentials.";
    }
  }
  return "";
}

export function validateBlocks(blocks: MonitoringBlock[]) {
  const problems = new Map<string, string>();
  if (!blocks.some((block) => block.category === "trigger")) problems.set("structure", "Add an initial trigger.");
  if (!blocks.some((block) => block.category === "action")) problems.set("actions", "Add an action.");
  const visit = (items: MonitoringBlock[]) => items.forEach((block) => { const problem = blockProblem(block); if (problem) problems.set(block.id, problem); if (block.children) visit(block.children); });
  visit(blocks); return problems;
}

export function blockTitle(block: MonitoringBlock) {
  const names: Record<string, string> = { group: "Condition group", event_triggered: "Event triggered", alert_triggered: "Alert triggered", sender_status: "Sender status", log_received: "Log received", message: "Log message", severity: "Severity", metadata: "Metadata", time: "Time", weekday: "Weekday", date: "Date", wait_until: "Wait Until", send_email: "Send email", send_http: "HTTP request", trigger_event: "Trigger event" };
  return names[block.type] ?? block.type;
}

export function blockSummary(block: MonitoringBlock): string {
  if (block.children) return `${block.negated ? "NOT " : ""}${block.groupOperator?.toUpperCase() ?? "AND"} group with ${block.children.length} blocks`;
  const value = block.condition?.value ?? block.action?.config ?? {};
  switch (block.type) {
    case "event_triggered": return `Event “${value.event_key || "not selected"}” ${block.condition?.operator.replaceAll("_", " ")}`;
    case "alert_triggered": return `Alert ${value.alert_id || "not selected"} ${block.condition?.operator.replaceAll("_", " ")}`;
    case "sender_status": return `When it becomes ${value.status === "inactive" ? "inactive" : "active"}`;
    case "log_received": return "When any log is received";
    case "message": return `Message ${block.condition?.operator.replaceAll("_", " ")} “${value.text || "…"}”`;
    case "severity": return `Severity ${block.condition?.operator.replaceAll("_", " ")} ${value.severity || "…"}`;
    case "metadata": return `Metadata ${value.path || "…"} ${block.condition?.operator.replaceAll("_", " ")}`;
    case "time": return `Time ${block.condition?.operator.replaceAll("_", " ")} ${value.start || "…"}${value.end ? ` and ${value.end}` : ""}`;
    case "weekday": return `Weekday ${block.condition?.operator.replaceAll("_", " ")} ${value.weekday || "…"}`;
    case "date": return `Date ${block.condition?.operator.replaceAll("_", " ")} ${value.start || "…"}`;
    case "wait_until": return `Wait until ${value.weekday || "…"} at ${value.time || "…"}`;
    case "send_email": return `Send email to ${Array.isArray(value.recipients) && value.recipients.length ? value.recipients.join(", ") : "recipients not defined"}`;
    case "send_http": return `${value.method || "HTTP"} ${value.url || "destination not defined"}`;
    case "trigger_event": return `Trigger event ${value.event_id || "not selected"}`;
    default: return blockTitle(block);
  }
}
