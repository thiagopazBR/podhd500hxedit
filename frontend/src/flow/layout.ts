// Signal-flow layout: where each block goes, in SVG units. Ported from the
// classic UI (flow.js layout()), which follows the POD HD500's LCD:
//  - lanes: pre (before the split), path A/B pre and post amp, post (after
//    the mixer);
//  - the amp sits "pre", on the paths ("a"/"b", both amp slots always there:
//    amp B as Amp Disabled) or "post";
//  - the cab is drawn right after its amp (one block on the LCD, apart here
//    so each can be clicked).
import type { Chain, Item, LaneName, PresetView, ModelGroup } from "../api";
import { isPreamp } from "./icons";

export const COL = 58, PAD = 28, Y1 = 34, Y2 = 108, BOX = 30;
const CAB_GAP = 0.72;
/** Columns between an amp and its cab (a preamp's label is wider). */
export const cabGap = (amp: Item, models: ModelGroup[]) => (isPreamp(amp, models) ? 0.92 : CAB_GAP);

export type AmpPos = "pre" | "a" | "post";

export interface FlowNode {
  x: number;
  y: number;
  it?: Item;
  special?: "in" | "out" | "mixer";
  zone?: LaneName; // end-of-lane / free-column target of an FX drag
  ampZone?: AmpPos; // where a dragged amp can go
  ampSlot?: "A" | "B"; // the "a" target is drawn as two stacked slots: amp A (a drop target) over amp B (indicator only)
}

export interface Layout { nodes: FlowNode[]; dual: boolean; bOutOnly: boolean; twoPaths: boolean; yc: number; split: number; merge: number; width: number; height: number; col: number; cols: number }

export interface DragKind { amp: boolean }

/** col: the column pitch (SVG units); the stage spreads the chain over its width (HX). */
export function layout(view: PresetView, chain: Chain, drag: DragKind | null, models: ModelGroup[], col0 = COL): Layout {
  const it = view.items;
  const ch = view.chain;
  const laneOf = (n: number) => (chain.lanes[n]?.blocks || []).map((id) => it[id]);
  const laneName = (n: number) => chain.lanes[n]?.name;
  const dragging = !!drag;
  const ampB = !it[2].empty;
  const pos = ampB ? "a" : ch.ampPos || "a";
  const aPre = laneOf(1), bPre = laneOf(2), aPost = laneOf(3), bPost = laneOf(4);
  const fxDrag = dragging && !drag.amp, ampDrag = dragging && drag.amp;
  const realDual = ch.dual || pos === "a" || pos === "b" || bPre.length > 0 || bPost.length > 0;
  const dual = fxDrag || ampDrag || realDual;
  // amp drag on a single-path preset: path B is shown only from the AMP B slot to the mixer
  const bOutOnly = ampDrag && !realDual;
  // With two paths (two amps) the chain line runs between them, as on the LCD: amp A a little above
  // it and amp B a little below, A reaching the mixer from the top and B from the bottom.
  const Yc = realDual ? (Y1 + Y2) / 2 : Y1;
  const nodes: FlowNode[] = [];
  let col = 0;
  const at = (c: number, y: number, n: Omit<FlowNode, "x" | "y">) => nodes.push({ ...n, x: PAD + c * col0, y });
  const zone = (lane: number, c: number, y: number) => { if (fxDrag) at(c - 0.5, y, { zone: laneName(lane) }); };
  const free = (lane: number, c0: number, c1: number, y: number) => { if (fxDrag) for (let c = c0; c < c1; c++) at(c, y, { zone: laneName(lane) }); };
  const ampZone = (to: AmpPos, c: number, y: number) => {
    if (!ampDrag || to === pos) return;
    // "a" (two paths) shows both amp slots, one under the other: amp A is the
    // drop target; amp B is only an indicator (an amp can't be dropped there)
    if (to === "a") { at(c - 1.5, Y1, { ampZone: to, ampSlot: "A" }); at(c - 1.5, Y2, { ampSlot: "B" }); }
    else at(c - 0.5, y, { ampZone: to });
  };
  // The cab stays close to its amp at any column pitch (one block on the
  // LCD): its gap is fixed in units, and the next column starts one pitch
  // after the cab.
  const gap = (i: number) => (cabGap(it[i], models) * COL) / col0;
  const ampW = (i: number) => (it[i].empty ? 1 : 1 + gap(i));
  const amp = (i: number, c: number, y: number) => { at(c, y, { it: it[i] }); if (!it[i].empty) at(c + gap(i), y, { it: it[i + 1] }); };

  at(col++, Yc, { special: "in" });
  for (const f of laneOf(0)) at(col++, Yc, { it: f });
  zone(0, col, Yc);
  ampZone("pre", col, Yc);
  if (pos === "pre") { amp(0, col, Yc); col += ampW(0); }
  const split = col - 0.5;
  let w: number;
  if (pos === "a" || pos === "b") {
    const bp = dual ? bPre : [], bq = dual ? bPost : [];
    const preW = Math.max(aPre.length, bp.length);
    const aw = Math.max(ampW(0), ampW(2));
    const postW = Math.max(aPost.length, bq.length);
    const ampCol = col + preW, postCol = ampCol + aw;
    aPre.forEach((f, i) => at(col + i, Y1, { it: f }));
    aPost.forEach((f, i) => at(postCol + i, Y1, { it: f }));
    bp.forEach((f, i) => at(col + i, Y2, { it: f }));
    bq.forEach((f, i) => at(postCol + i, Y2, { it: f }));
    amp(0, ampCol, Y1);
    amp(2, ampCol, Y2);
    free(1, col + aPre.length, col + preW, Y1);
    free(3, postCol + aPost.length, postCol + postW, Y1);
    if (dual) {
      free(2, col + bp.length, col + preW, Y2);
      free(4, postCol + bq.length, postCol + postW, Y2);
    }
    zone(1, col + aPre.length, Y1);
    zone(3, postCol + aPost.length, Y1);
    if (dual) {
      zone(2, col + bp.length, Y2);
      zone(4, postCol + bq.length, Y2);
    }
    w = preW + aw + postW;
  } else {
    const wA = aPre.length + aPost.length, wB = dual ? bPre.length + bPost.length : 0;
    w = Math.max(wA, wB);
    const W = Math.max(1, w);
    free(1, col + aPre.length, col + W - aPost.length, Y1);
    if (dual) free(2, col + bPre.length, col + W - bPost.length, Y2);
    aPre.forEach((f, i) => at(col + i, Y1, { it: f }));
    aPost.forEach((f, i) => at(col + w - aPost.length + i, Y1, { it: f }));
    zone(3, col + Math.max(w, 1), Y1);
    if (dual) {
      bPre.forEach((f, i) => at(col + i, Y2, { it: f }));
      bPost.forEach((f, i) => at(col + w - bPost.length + i, Y2, { it: f }));
      zone(4, col + Math.max(w, 1), Y2);
    }
  }
  col += w === 0 && !dragging && !dual ? 0 : Math.max(1, w);
  const merge = col;
  ampZone("a", col, Y1);
  ampZone("post", col + 1, Yc);
  at(col++, Yc, { special: "mixer" });
  if (pos === "post") { amp(0, col, Yc); col += ampW(0); }
  for (const f of laneOf(5)) at(col++, Yc, { it: f });
  zone(5, col, Yc);
  at(col++, Yc, { special: "out" });
  return { nodes, dual, bOutOnly, twoPaths: realDual, yc: Yc, split: PAD + split * col0, merge: PAD + merge * col0, width: PAD * 2 + (col - 1) * col0, height: dual ? Y2 + 58 : Y1 + 62, col: col0, cols: col - 1 };
}

/** What to select when a preset loads: the first block in signal order. */
export function defaultSel(view: PresetView, models: ModelGroup[]): number | "input" {
  const nodes = layout(view, view.chain, null, models).nodes.filter((n) => n.it).sort((a, b) => a.x - b.x || a.y - b.y);
  const first = nodes.find((n) => !n.it!.empty) || nodes[0];
  return first ? first.it!.id : "input";
}

// ---- FX drop rule (unchanged from the classic UI)
export interface DropTarget { lane: LaneName; before?: number }

/** Where block id lands for a target; null when it would stay put. Dropped
 *  on a block further right in the same lane it takes that block's place. */
export function fxDrop(chain: Chain, id: number, target: DropTarget | null): DropTarget | null {
  if (!target || target.before === id) return null;
  const lane = chain.lanes.find((l) => l.name === target.lane);
  const ids = lane ? lane.blocks : [];
  let before = target.before;
  const from = ids.indexOf(id), to = before === undefined ? -1 : ids.indexOf(before);
  if (before !== undefined && from >= 0 && to > from) before = ids.slice(to + 1).find((b) => b !== id);
  const noop = from >= 0 && (before === undefined ? from === ids.length - 1 : before === ids[from + 1]);
  return noop ? null : { lane: target.lane, before };
}

/** The chain after that drop (what the drag shows live). */
export function previewChain(chain: Chain, id: number, drop: DropTarget): Chain {
  const lanes = chain.lanes.map((l) => ({ ...l, blocks: l.blocks.filter((b) => b !== id) }));
  const l = lanes.find((x) => x.name === drop.lane);
  if (!l) return chain;
  const at = drop.before === undefined ? -1 : l.blocks.indexOf(drop.before);
  l.blocks.splice(at < 0 ? l.blocks.length : at, 0, id);
  return { ...chain, lanes };
}
