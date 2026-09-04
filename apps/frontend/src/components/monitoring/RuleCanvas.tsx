import { useDroppable } from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { AlertTriangle, Braces, Copy, GripVertical, MoreHorizontal, ScanSearch as Radar, Trash2, Ungroup } from "lucide-react";
import { memo, useState } from "react";
import { IconButton } from "../ui";
import type { MonitoringBlock } from "./blockModel";
import { blockProblem, blockSummary, blockTitle } from "./blockModel";

function blockBorderTone(block: MonitoringBlock, selected: boolean) {
  if (block.children) return `border-cyan-800/80 hover:border-cyan-600 ${selected ? "ring-1 ring-cyan-500/40" : ""}`;
  if (block.category === "trigger") return `border-sky-700/70 hover:border-sky-500 ${selected ? "ring-1 ring-sky-500/40" : ""}`;
  if (block.category === "action") return `border-emerald-700/70 hover:border-emerald-500 ${selected ? "ring-1 ring-emerald-500/40" : ""}`;
  if (["time", "weekday", "date", "wait_until"].includes(block.type)) return `border-amber-700/70 hover:border-amber-500 ${selected ? "ring-1 ring-amber-500/40" : ""}`;
  return `border-violet-700/70 hover:border-violet-500 ${selected ? "ring-1 ring-violet-500/40" : ""}`;
}

interface BlockListProps {
  blocks: MonitoringBlock[];
  selectedID?: string;
  depth?: number;
  groupOperator?: "and" | "or";
  onSelect: (id: string) => void;
  onUpdate: (id: string, change: Partial<MonitoringBlock>) => void;
  onDuplicate: (id: string) => void;
  onMove: (id: string, direction: -1 | 1) => void;
  onRemove: (id: string) => void;
  onGroup: (id: string) => void;
  onUngroup: (id: string) => void;
}

interface RuleBlockProps extends Omit<BlockListProps, "blocks"> {
  block: MonitoringBlock;
  index: number;
  total: number;
  canGroup: boolean;
}

const RuleBlock = memo(function RuleBlock({ block, index, total, selectedID, depth = 0, groupOperator, canGroup, onSelect, onUpdate, onDuplicate, onMove, onRemove, onGroup, onUngroup }: RuleBlockProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging, isOver } = useSortable({ id: block.id, data: { source: "canvas", category: block.category } });
  const [menu, setMenu] = useState(false);
  const problem = blockProblem(block);
  const relation = index === 0 ? (depth ? "FIRST" : block.category === "action" ? "THEN" : "WHEN") : block.category === "action" ? "AND ALSO" : (groupOperator ?? "and").toUpperCase();

  return <div className="relative">
    <div className="flex h-9 items-center justify-center"><span className="absolute inset-y-0 left-1/2 w-px bg-zinc-700" /><span className="relative rounded-full border border-zinc-700 bg-zinc-950 px-2 py-0.5 text-[9px] font-semibold text-zinc-400">{isOver ? "Drop block here" : relation}</span></div>
    <article ref={setNodeRef} tabIndex={0} aria-label={`${blockTitle(block)}. Level ${depth + 1}, position ${index + 1} of ${total}`} onClick={() => onSelect(block.id)} onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); onSelect(block.id); } }} style={{ transform: CSS.Transform.toString(transform), transition }} className={`group relative rounded-lg border bg-zinc-900 p-3 text-left transition-[border-color,box-shadow,opacity] duration-150 ${blockBorderTone(block, selectedID === block.id)} ${isDragging ? "z-10 opacity-80 shadow-lg" : ""}`}>
      <div className="flex items-start gap-2">
        <button type="button" aria-label={`Drag ${blockTitle(block)}`} className="mt-0.5 cursor-grab touch-none rounded p-1 text-zinc-600 hover:text-zinc-300 active:cursor-grabbing" {...attributes} {...listeners}><GripVertical className="size-4" /></button>
        <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><span className="text-[10px] font-semibold text-zinc-500">{block.children ? "GROUP" : block.category === "trigger" ? "TRIGGER" : block.category === "action" ? "ACTION" : "CONDITION"}</span>{block.negated && <span className="rounded border border-zinc-600 px-1.5 py-0.5 text-[9px] font-bold">NOT</span>}{problem && <AlertTriangle className="size-3.5 text-red-400" />}</div><p className="mt-1 text-sm font-medium text-zinc-100">{blockSummary(block)}</p><p className="mt-1 text-[10px] text-zinc-600">{blockTitle(block)}</p>{problem && <p className="mt-2 text-[10px] text-red-400">{problem}</p>}</div>
        <div className="relative"><IconButton label={`Actions for ${blockTitle(block)}`} className="size-7" onClick={(event) => { event.stopPropagation(); setMenu((value) => !value); }}><MoreHorizontal className="size-4" /></IconButton>{menu && <div className="absolute right-0 top-8 z-30 w-48 rounded-lg border border-zinc-700 bg-zinc-900 p-1 shadow-xl"><button className="w-full rounded px-2 py-1.5 text-left text-xs hover:bg-zinc-800" onClick={() => { onSelect(block.id); setMenu(false); }}>Configure</button>{canGroup && <button className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-zinc-800" onClick={() => { onGroup(block.id); setMenu(false); }}><Braces className="size-3" />Group with previous</button>}{block.children && <button className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-zinc-800" onClick={() => { onUngroup(block.id); setMenu(false); }}><Ungroup className="size-3" />Dissolve group</button>}<button className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-zinc-800" onClick={() => { onDuplicate(block.id); setMenu(false); }}><Copy className="size-3" />Duplicate</button><button disabled={index === 0} className="w-full rounded px-2 py-1.5 text-left text-xs hover:bg-zinc-800 disabled:opacity-40" onClick={() => { onMove(block.id, -1); setMenu(false); }}>Move up</button><button disabled={index === total - 1} className="w-full rounded px-2 py-1.5 text-left text-xs hover:bg-zinc-800 disabled:opacity-40" onClick={() => { onMove(block.id, 1); setMenu(false); }}>Move down</button><button className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-red-400 hover:bg-zinc-800" onClick={() => { onRemove(block.id); setMenu(false); }}><Trash2 className="size-3" />Remove</button></div>}</div>
      </div>
      {block.children && <div className="mt-3 rounded-lg border border-cyan-950 bg-zinc-950/70 px-3 pb-3" onClick={(event) => event.stopPropagation()}><BlockList blocks={block.children} selectedID={selectedID} depth={depth + 1} groupOperator={block.groupOperator ?? "and"} onSelect={onSelect} onUpdate={onUpdate} onDuplicate={onDuplicate} onMove={onMove} onRemove={onRemove} onGroup={onGroup} onUngroup={onUngroup} /></div>}
      {block.type === "wait_until" && <p className="mt-3 border-t border-zinc-800 pt-2 text-[10px] text-amber-400">The next action runs after this wait.</p>}
    </article>
  </div>;
});

function BlockList({ blocks, selectedID, depth = 0, groupOperator, onSelect, onUpdate, onDuplicate, onMove, onRemove, onGroup, onUngroup }: BlockListProps) {
  return <SortableContext items={blocks.map((block) => block.id)} strategy={verticalListSortingStrategy}>{blocks.map((block, index) => <RuleBlock key={block.id} block={block} index={index} total={blocks.length} selectedID={selectedID} depth={depth} groupOperator={groupOperator} canGroup={index > 0 && block.category !== "action" && blocks[index - 1].category !== "action"} onSelect={onSelect} onUpdate={onUpdate} onDuplicate={onDuplicate} onMove={onMove} onRemove={onRemove} onGroup={onGroup} onUngroup={onUngroup} />)}</SortableContext>;
}

type RuleCanvasProps = Omit<BlockListProps, "depth" | "groupOperator">;

export function RuleCanvas({ blocks, selectedID, onSelect, onUpdate, onDuplicate, onMove, onRemove, onGroup, onUngroup }: RuleCanvasProps) {
  const { setNodeRef, isOver } = useDroppable({ id: "rule-canvas" });
  return <main ref={setNodeRef} id="monitoring-rule-canvas" className={`min-h-0 flex-1 overflow-y-auto bg-zinc-950 bg-[radial-gradient(#27272a_1px,transparent_1px)] [background-size:18px_18px] p-4 transition-colors sm:p-6 ${isOver ? "bg-zinc-900" : ""}`}>
    <div className="mx-auto max-w-2xl">
      <div className="flex items-center justify-center"><span className="rounded-full border border-zinc-700 bg-zinc-900 px-3 py-1 text-[10px] font-semibold">START</span></div>
      {blocks.length ? <BlockList blocks={blocks} selectedID={selectedID} onSelect={onSelect} onUpdate={onUpdate} onDuplicate={onDuplicate} onMove={onMove} onRemove={onRemove} onGroup={onGroup} onUngroup={onUngroup} /> : <div className="mt-6 grid min-h-72 place-items-center rounded-xl border border-dashed border-zinc-700 bg-zinc-950/80 p-8 text-center"><div><Radar className="mx-auto size-8 text-zinc-700" /><p className="mt-3 text-sm font-medium">Drag a trigger to begin</p><p className="mt-1 text-xs text-zinc-600">Choose a block from the library or click it to add.</p></div></div>}
      <div className="flex h-12 items-center justify-center"><span className="h-8 w-px bg-zinc-700" /></div><div className="text-center text-[10px] text-zinc-600">END OF FLOW</div>
    </div>
  </main>;
}
