// Signal Flow: the blocks of the preset loaded on the pedal, as one SVG chain
// (HX Edit look, POD HD500 topology). Ported from the classic UI (flow.js):
// click selects, double-click / Space turns on/off, hover shows power and ×,
// drag moves FX blocks (live preview) and amp A with its cab (drop zones).
import { useEffect, useMemo, useRef, useState, type PointerEvent as RPointerEvent } from "react";
import { post, type Chain, type Item, type PresetView } from "../api";
import { useEditor, type Sel } from "../store";
import { CATS, catGlyph, catOf, inputIcon, inputShortName, isPreamp, type Cat } from "../flow/icons";
import { BOX, COL, PAD, Y1, Y2, cabGap, fxDrop, layout, previewChain, type AmpPos, type DropTarget, type FlowNode } from "../flow/layout";

const H = BOX / 2;
const SCALE = 1.2; // block 30 units → 36 px (HX Edit's is ~29 px; larger here so the icons read well)
/** Column pitch (SVG units) at full size: 91 px between block centres (75.8 units * SCALE 1.2), as Helix Native.
 *  When the chain is wider than the panel the pitch shrinks down to PITCH_MIN; past that it scrolls. */
const PITCH0 = 75.8, PITCH_MIN = 62;
const V_NUDGE = 4.4; // units: measured so the visible chain (icons + labels) sits centred in the panel
/** Glyph size inside the 30-unit block box. */
export const ICON_SIZE = 33;
const AMP_ZONE_TIP: Record<AmpPos, string> = {
  pre: "Pre, before the split: 1 path (amp B is removed)",
  a: "On the paths: 2 paths (amp A on path A, amp B on path B)",
  post: "Post, after the mixer: 1 path (amp B is removed)",
};
const AMP_ZONE_LABEL: Record<AmpPos, string> = { pre: "1 PATH", a: "2 PATHS", post: "POST" };

interface Drag {
  id: number;
  amp: boolean;
  offX: number; // pointer offset from the block's centre (screen px)
  offY: number;
  x: number; // pointer (screen px)
  y: number;
  base: { x: number; y: number; t: DropTarget }[]; // FX targets as the drag began
  scale: number;
  drop: DropTarget | null;
  ampTarget: AmpPos | null;
}

function Glyph({ cat, x, y, size, cls = "blk-icon" }: { cat: Cat; x: number; y: number; size: number; cls?: string }) {
  const g = catGlyph(cat);
  return <svg className={cls + g.cls} x={x - size / 2} y={y - size / 2} width={size} height={size} viewBox={g.vb} dangerouslySetInnerHTML={{ __html: g.inner }} />;
}

export function SignalFlow() {
  const { state, meta, models, preset, presetError, sel, select, refetchPreset, patchPreset, toast } = useEditor();
  const stageRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const [drag, setDrag] = useState<Drag | null>(null);
  const [preview, setPreview] = useState<Chain | null>(null);
  const press = useRef<{ id: number; amp: boolean; x: number; y: number; offX: number; offY: number; pointerId: number } | null>(null);
  const justDragged = useRef(false);

  const chain = preview || preset?.chain;

  // The panel's width: the chain is centred in it, and squeezed (down to PITCH_MIN) when it does not fit.
  const [stageW, setStageW] = useState(0), [stageH, setStageH] = useState(0);
  const connected = !!state?.connected, hasPreset = !!preset;
  useEffect(() => {
    const el = stageRef.current;
    if (!el) return;
    const measure = () => {
      const cs = getComputedStyle(el);
      setStageW(el.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight)); // content width
      setStageH(el.clientHeight - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom));
    };
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    measure();
    return () => ro.disconnect();
  }, [connected, hasPreset]);
  const avail = Math.max(0, stageW);
  // Fit measured on the preset without any drag, so the chain does not shift while a block is dragged.
  // The cab gap is fixed in units, so the column count depends on the pitch: refine a few times.
  const fit = useMemo(() => {
    if (!preset) return { pitch: PITCH0, width: 0, height: 0 };
    let pitch = PITCH0;
    let lay = layout(preset, preset.chain, null, models, pitch);
    if (avail && lay.width * SCALE > avail) {
      for (let i = 0; i < 4; i++) {
        pitch = Math.max(PITCH_MIN, Math.min(PITCH0, (avail / SCALE - 2 * PAD) / lay.cols));
        lay = layout(preset, preset.chain, null, models, pitch);
        if (lay.width * SCALE <= avail + 0.5 || pitch === PITCH_MIN) break;
      }
    }
    return { pitch, width: lay.width * SCALE, height: lay.height * SCALE };
  }, [preset, models, avail]);
  const pitch = fit.pitch;
  const marginLeft = Math.max(0, (avail - fit.width) / 2);
  // Centred vertically too (one path or two); from the layout without a drag, so the chain stays put while dragging.
  // + V_NUDGE: the layout box has a little more room above the blocks (hover controls) than below their labels.
  const marginTop = Math.max(0, (stageH - fit.height) / 2 + V_NUDGE * SCALE);
  const L = useMemo(() => {
    if (!preset || !chain) return null;
    return layout(preset, chain, drag, models, pitch);
  }, [preset, chain, drag, models, pitch]);

  // ---- actions (the backend does the work; the UI says what)
  async function toggleBlock(id: number) {
    const it = preset?.items[id];
    if (!it || it.empty || it.kind === "cab" || it.dspOver) return;
    const active = !it.active;
    patchPreset((p) => ({ ...p, items: p.items.map((x) => (x.id === id ? { ...x, active } : x)) }));
    try {
      await post("/api/block", { id, active });
    } catch (e) {
      patchPreset((p) => ({ ...p, items: p.items.map((x) => (x.id === id ? { ...x, active: !active } : x)) }));
      toast(`Couldn't turn ${active ? "on" : "off"} ${it.model}: ${(e as Error).message}`);
    }
  }

  async function removeBlock(id: number) {
    const it = preset?.items[id];
    if (!it || (it.empty && !it.dspOver)) return;
    let type = 0x020dffff; // FX: None
    if (it.kind === "amp" || it.kind === "cab") {
      const none = models.find((x) => x.name === (it.kind === "amp" ? "Amps" : "Cabs"))?.models.find((m) => m.none);
      if (!none) return;
      type = none.type;
    }
    try {
      await post("/api/block/model", { id, type });
      await refetchPreset();
    } catch (e) {
      toast(`Couldn't change the model: ${(e as Error).message}`);
    }
  }

  async function moveBlock(block: number, drop: DropTarget) {
    try {
      await post("/api/chain/move", drop.before === undefined ? { block, lane: drop.lane } : { block, lane: drop.lane, before: drop.before });
      await refetchPreset();
    } catch (e) {
      toast(`Couldn't move the block: ${(e as Error).message}`);
    }
  }

  async function moveAmp(pos: AmpPos) {
    try {
      await post("/api/amp/move", { pos });
      await refetchPreset();
    } catch (e) {
      toast(`Couldn't move the amp: ${(e as Error).message}`);
    }
  }

  // ---- pointer: click / double-click / drag
  const idOf = (el: Element): Sel => {
    const g = el.closest<SVGGElement>(".blk[data-id]");
    if (!g) return null;
    const d = g.dataset.id!;
    return d === "mixer" || d === "input" ? d : Number(d);
  };

  function onClick(e: React.MouseEvent) {
    if (justDragged.current) return;
    const t = e.target as Element;
    const id = idOf(t);
    if (id === null) return;
    if (typeof id === "number" && t.closest("[data-toggle]")) { void toggleBlock(id); return; }
    if (typeof id === "number" && t.closest("[data-remove]")) { void removeBlock(id); return; }
    select(id);
    stageRef.current?.focus({ preventScroll: true });
  }

  function onDoubleClick(e: React.MouseEvent) {
    const t = e.target as Element;
    const id = idOf(t);
    if (typeof id === "number" && !t.closest("[data-toggle], [data-remove]")) void toggleBlock(id);
  }

  function onPointerDown(e: RPointerEvent) {
    const t = e.target as Element;
    const g = t.closest<SVGGElement>(".blk[data-id]");
    if (!g || e.button !== 0 || t.closest("[data-toggle], [data-remove]") || !preset) return;
    const it = preset.items[Number(g.dataset.id)];
    // FX blocks (empty slots too: the pedal moves them); amp A (even None) or
    // cab A move amp A with its cab. Amp B never moves (as on the pedal).
    const isAmp = !!it && (it.id === 0 || (it.id === 1 && !preset.items[0].empty));
    if (!it || (!isAmp && it.kind !== "fx")) return;
    e.preventDefault();
    stageRef.current?.focus({ preventScroll: true });
    const box = g.querySelector(".blk-box, .amp-empty-box, .slot-box")?.getBoundingClientRect();
    press.current = { id: it.id, amp: isAmp, x: e.clientX, y: e.clientY, pointerId: e.pointerId,
      offX: box ? e.clientX - (box.left + box.width / 2) : 0, offY: box ? e.clientY - (box.top + box.height / 2) : 0 };
  }

  function fxTargets(id: number): { base: Drag["base"]; scale: number } {
    const ctm = svgRef.current?.getScreenCTM();
    if (!ctm || !preset) return { base: [], scale: 1 };
    const laneOfId = (b: number) => preset.chain.lanes.find((l) => l.blocks.includes(b))?.name;
    const nodes = layout(preset, preset.chain, { amp: false }, models, L?.col ?? COL).nodes;
    const base = nodes.filter((n) => n.zone || (n.it && n.it.kind === "fx")).map((n) => {
      const pt = new DOMPoint(n.x, n.y).matrixTransform(ctm);
      const t: DropTarget = n.zone ? { lane: n.zone } : { lane: laneOfId(n.it!.id)!, before: n.it!.id };
      return { x: pt.x, y: pt.y, t };
    });
    void id;
    return { base, scale: ctm.a };
  }

  function fxTarget(d: Drag, x: number, y: number): DropTarget | null {
    if (!preset) return null;
    let best: DropTarget | null = null, bd = Infinity;
    const rx = ((L?.col ?? COL) / 2) * d.scale, ry = 32 * d.scale;
    for (const b of d.base) {
      if (b.t.before !== d.id && !fxDrop(preset.chain, d.id, b.t)) continue;
      const dx = Math.abs(x - b.x), dy = Math.abs(y - b.y);
      if (dx < rx && dy < ry && dx + dy < bd) { bd = dx + dy; best = b.t; }
    }
    return best;
  }

  function onPointerMove(e: RPointerEvent) {
    const p = press.current;
    if (!p || !preset) return;
    let d = drag;
    if (!d) {
      if (Math.hypot(e.clientX - p.x, e.clientY - p.y) < 6) return;
      stageRef.current?.setPointerCapture(p.pointerId);
      window.getSelection()?.removeAllRanges();
      const { base, scale } = p.amp ? { base: [], scale: 1 } : fxTargets(p.id);
      d = { id: p.id, amp: p.amp, offX: p.offX, offY: p.offY, x: e.clientX, y: e.clientY, base, scale, drop: null, ampTarget: null };
    }
    const next: Drag = { ...d, x: e.clientX, y: e.clientY };
    if (d.amp) {
      const z = document.elementFromPoint(e.clientX, e.clientY)?.closest<SVGGElement>(".amp-zone");
      next.ampTarget = (z?.dataset.amp as AmpPos) || null;
    } else {
      const drop = fxDrop(preset.chain, d.id, fxTarget(d, e.clientX - d.offX, e.clientY - d.offY));
      const key = (t: DropTarget | null) => (t ? `${t.lane}:${t.before}` : "");
      if (key(drop) !== key(d.drop)) setPreview(drop ? previewChain(preset.chain, d.id, drop) : null);
      next.drop = drop;
    }
    setDrag(next);
  }

  function endDrag(cancel: boolean) {
    const d = drag;
    press.current = null;
    if (!d) return;
    setDrag(null);
    justDragged.current = true; // swallow the click that follows
    setTimeout(() => { justDragged.current = false; }, 0);
    if (cancel) { setPreview(null); return; }
    if (d.amp) { if (d.ampTarget) void moveAmp(d.ampTarget); return; }
    if (!d.drop) { setPreview(null); return; }
    // keep showing the new order until the pedal has it (no flash back)
    void moveBlock(d.id, d.drop).finally(() => setPreview(null));
  }

  useEffect(() => {
    if (!drag) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") endDrag(true); };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  });

  // ---- drawing
  if (!state?.connected) return <div className="flow-stage" ref={stageRef}><span className="flow-empty">No pedal connected</span></div>;
  if (!preset || !L) return <div className="flow-stage" ref={stageRef}><span className="flow-empty">{presetError || "Reading the preset…"}</span></div>;

  // the dragged block (and its amp/cab mate) follow the pointer
  const dragOffset = (): string | undefined => {
    if (!drag) return undefined;
    const ctm = svgRef.current?.getScreenCTM();
    const n = L.nodes.find((x) => x.it?.id === drag.id);
    if (!ctm || !n) return undefined;
    const pt = new DOMPoint(drag.x - drag.offX, drag.y - drag.offY).matrixTransform(ctm.inverse());
    return `translate(${(pt.x - n.x).toFixed(2)} ${(pt.y - n.y).toFixed(2)})`;
  };
  const tf = dragOffset();
  const moving = (id: number) => !!drag && (id === drag.id || (drag.amp && (id === 0 || id === 1)));

  const ampOf = (cab: Item) => preset.items[cab.id === 1 ? 0 : 2];
  const r = 12;
  const yc = L.yc;
  // Amp B's curve into the mixer is split where it first touches the "Mixer" label (its bottom is
  // ~15 below the mixer's centre line): opaque before that point, very faint from there on.
  const tt = Math.sqrt(Math.max(0, Math.min(1, (Y2 - (yc + H + 15)) / r)));  // quadratic Bezier parameter
  const q0x = L.merge - r + r * tt, q1y = Y2 - r * tt;
  const rx = q0x + tt * (L.merge - q0x), ry = Y2 + tt * (q1y - Y2);
  const wires = L.twoPaths
    ? [
        <path key="main" d={`M${PAD} ${yc}H${L.split}`} />,
        <path key="mainR" d={`M${L.merge} ${yc}H${L.width - PAD}`} />,
        <path key="a" d={`M${L.split} ${yc}V${Y1 + r}q0 -${r} ${r} -${r}H${L.merge - r}q${r} 0 ${r} ${r}V${yc - H}`} />,
        <path key="bp" d={`M${L.split} ${yc}V${Y2 - r}q0 ${r} ${r} ${r}H${L.merge - r}Q${q0x} ${Y2} ${rx} ${ry}`} />,
        <path key="bpEnd" className="wire-faint" d={`M${rx} ${ry}Q${L.merge} ${q1y} ${L.merge} ${Y2 - r}V${yc + H}`} />,
      ]
    : [<path key="main" d={`M${PAD} ${Y1}H${L.width - PAD}`} />];
  if (L.dual) {
    if (L.bOutOnly) {
      // Amp drag on a single-path preset: dotted, dimmed hints. Amp A -> amp B (vertical) and
      // amp B -> mixer, straight into the mixer (it does not join the main line).
      const xb = L.merge - 1.5 * L.col;
      wires.push(<path key="ab" className="wire-hint" d={`M${xb} ${Y1 + H}V${Y2 - H}`} />);
      wires.push(<path key="b" className="wire-hint" d={`M${xb + H} ${Y2}H${L.merge - r}q${r} 0 ${r} -${r}V${Y1 + H + 19}`} />);
    } else if (!L.twoPaths) {
      // dual only because of an FX drag on a single-amp preset: the old second row
      wires.push(<path key="b" d={`M${L.split} ${Y1}V${Y2 - r}q0 ${r} ${r} ${r}H${L.merge - L.col / 2 - r}q${r} 0 ${r} -${r}V${Y1}`} />);
    }
  }
  const links: React.ReactNode[] = [];
  for (const [ai, ci] of [[0, 1], [2, 3]]) {
    const na = L.nodes.find((n) => n.it?.id === ai), nc = L.nodes.find((n) => n.it?.id === ci);
    if (!na || !nc || na.it!.empty) continue;
    const linked = !nc.it!.empty && na.it!.active;
    const ampHalf = isPreamp(na.it, models) ? 16 : 9, cabHalf = 9;
    links.push(<text key={`plus${ai}`} className={`amp-plus${linked ? " on" : ""}`} x={(na.x + ampHalf + nc.x - cabHalf) / 2} y={na.y + H + 13}>+</text>);
    if (linked) wires.push(<path key={`link${ai}`} className="amp-link" style={{ stroke: CATS.amp.color }} d={`M${na.x + H} ${na.y}H${nc.x - H}`} />);
  }

  const input = (n: FlowNode) => {
    const label = (key: "input1" | "input2", v: number) => (meta?.[key] as { value: number; label: string }[] | undefined)?.find((o) => o.value === v)?.label ?? "—";
    const src = label("input1", state.input1);
    const src2 = state.input2 !== 0 ? label("input2", state.input2) : "";
    const ic = inputIcon(src);
    let ty = n.y + 30;
    const lines = [<text key="l1" className="io-label" x={n.x} y={ty}>{inputShortName(src)}</text>];
    if (src2) { ty += 10; lines.push(<text key="l2" className="io-label2" x={n.x} y={ty}>{inputShortName(src2)}</text>); }
    if (state.inputsSetup === 1) { ty += 10; lines.push(<text key="l3" className="io-tag" x={n.x} y={ty}>GLOBAL</text>); }
    const tip = `Input: ${src}${src2 ? " · Input 2: " + src2 : ""}${state.inputsSetup === 1 ? " (Global)" : ""}`;
    return (
      <g key="in" className={`blk io io-in${sel === "input" ? " sel" : ""}`} data-id="input">
        <title>{tip}</title>
        <circle className="io-ring" cx={n.x} cy={n.y} r={18} /><circle cx={n.x} cy={n.y} r={14} />
        <svg className="io-icon" x={n.x - 10} y={n.y - 10} width={20} height={20} viewBox="0 0 24 24"><path d={ic.d} transform={ic.tf} /></svg>
        {lines}
      </g>
    );
  };

  const block = (n: FlowNode, it: Pick<Item, "id" | "kind" | "model" | "active"> & Partial<Item>, cat: Cat, tip: string, extraCls = "") => {
    const { x, y } = n;
    const id = it.id as number | "mixer";
    const iz = cat.svg ? ICON_SIZE : 18;
    const kindLabel = it.kind === "amp" ? (isPreamp(it as Item, models) ? "Preamp" : "Amp") : it.kind === "cab" ? "Cab" : (it.kind as string) === "mixer" ? "Mixer" : it.short || "";
    const fsY = y + H + (kindLabel ? 21 : 12);
    const cy = y - H - 9;
    const power = (cx: number) => (
      <g className="blk-ctl" data-toggle="1"><title>On/off</title><rect x={cx - 6} y={cy - 6} width={12} height={12} />
        <path d={`M${cx} ${cy - 4.6}v4.2M${cx - 3.3} ${cy - 2.7}a4.5 4.5 0 1 0 6.6 0`} /></g>
    );
    const remove = (cx: number) => (
      <g className="blk-ctl" data-remove="1"><title>Remove (None)</title><rect x={cx - 6} y={cy - 6} width={12} height={12} />
        <path d={`M${cx - 3} ${cy - 3}l6 6M${cx + 3} ${cy - 3}l-6 6`} /></g>
    );
    const isMixer = id === "mixer";
    const mv = typeof id === "number" && moving(id);
    return (
      <g key={String(id)} className={`blk${it.active ? " on" : ""}${sel === id ? " sel" : ""}${extraCls}${mv ? " drag-src" : ""}`} data-id={id}
        style={{ "--cat": cat.color } as React.CSSProperties} transform={mv ? tf : undefined}>
        <title>{tip}</title>
        <rect className="blk-sel" x={x - H - 5} y={y - H - 5} width={BOX + 10} height={BOX + 10} rx={9} />
        <rect className="blk-box" x={x - H} y={y - H} width={BOX} height={BOX} rx={3} />
        <Glyph cat={cat} x={x} y={y} size={iz} />
        {!isMixer && !mv && (
          <g className="blk-hover"><rect className="blk-hover-hit" x={x - H} y={cy - 7} width={BOX} height={16} />
            {it.kind === "cab" || it.dspOver ? remove(x) : <>{power(x - 7)}{remove(x + 7)}</>}</g>
        )}
        {kindLabel && <text className="blk-fs blk-ac" x={x} y={y + H + 13}>{kindLabel}</text>}
        {it.assign && <text className="blk-fs blk-fsw" x={it.kind === "amp" ? x + (cabGap(it as Item, models) * COL) / 2 : x} y={fsY}>{it.assign}</text>}
      </g>
    );
  };

  const node = (n: FlowNode, i: number): React.ReactNode => {
    const { x, y } = n;
    if (n.ampSlot === "B" && !n.ampZone) {
      // Amp B while an amp is dragged: only a hint that a second amp can be added later on path B
      return (
        <g key="ampB-hint" className="amp-hint">
          <title>Amp B: a second amp can be added here later (paths A and B)</title>
          <rect x={x - H} y={y - H} width={BOX} height={BOX} rx={3} />
          <Glyph cat={{ ...CATS.amp, color: "#6f6f6f" }} x={x} y={y} size={ICON_SIZE} cls="dz-icon" />
          <text className="dz-label" x={x} y={y + H + 11}>AMP B</text>
        </g>
      );
    }
    if (n.ampZone) {
      return (
        <g key={`az${n.ampZone}${n.ampSlot ?? ""}`} className={`drop-zone amp-zone${drag?.ampTarget === n.ampZone ? " drop-hover" : ""}`} data-amp={n.ampZone}>
          <title>{`Move the amp here: ${AMP_ZONE_TIP[n.ampZone]}`}</title>
          <rect x={x - H} y={y - H} width={BOX} height={BOX} rx={3} />
          <Glyph cat={CATS.amp} x={x} y={y} size={ICON_SIZE} cls="dz-icon" />
          <text className="dz-label" x={x} y={y - H - 6}>{n.ampSlot ? `AMP ${n.ampSlot}` : AMP_ZONE_LABEL[n.ampZone]}</text>
        </g>
      );
    }
    if (n.zone) return null; // FX targets aren't drawn: the live preview shows where it goes
    if (n.special === "in") return input(n);
    if (n.special === "out") return <g key="out" className="io"><title>Output</title><circle cx={x} cy={y} r={14} /><path d={`M${x - 7} ${y}h13M${x + 1} ${y - 5}l5 5-5 5`} /></g>;
    if (n.special === "mixer") return block(n, { id: "mixer" as unknown as number, kind: "mixer" as Item["kind"], model: "Mixer", active: true }, CATS.mixer, "Mixer A/B");
    const it = n.it!;
    const mv = moving(it.id);
    if (it.empty && it.kind === "amp") {
      return (
        <g key={it.id} className={`blk empty amp-empty${sel === it.id ? " sel" : ""}${mv ? " drag-src" : ""}`} data-id={it.id} transform={mv ? tf : undefined}>
          <title>None (empty block): click to choose an amp</title>
          <rect className="blk-sel" x={x - H - 5} y={y - H - 5} width={BOX + 10} height={BOX + 10} rx={9} />
          <rect className="amp-empty-box" x={x - H} y={y - H} width={BOX} height={BOX} rx={3} />
          <Glyph cat={CATS.amp} x={x} y={y} size={ICON_SIZE} cls="blk-icon" />
          <text className="blk-fs blk-ac" x={x} y={y + H + 13}>Amp</text>
        </g>
      );
    }
    if (it.kind === "cab") {
      const amp = ampOf(it);
      const tip = it.empty ? "Cab: None (disabled)" : `${it.model}${it.basedOn ? " — " + it.basedOn : ""}${amp.active ? "" : " (amp off)"}`;
      return block(n, { ...it, active: !it.empty && amp.active }, CATS.cab, tip);
    }
    if (it.dspOver) return block(n, { ...it, active: false, short: "DSP OVER", assign: "" }, catOf(it, models), `${it.model}: DSP limit reached, not running`, " dsp-over");
    if (it.empty) {
      return (
        <g key={it.id} className={`blk empty slot-empty${sel === it.id ? " sel" : ""}${mv ? " drag-src" : ""}`} data-id={it.id} transform={mv ? tf : undefined}>
          <title>Empty slot: click to choose an effect</title>
          <rect className="blk-sel" x={x - 14} y={y - 14} width={28} height={28} rx={7} />
          <rect className="slot-box" x={x - 8} y={y - 8} width={16} height={16} rx={4} />
        </g>
      );
    }
    void i;
    return block(n, it, catOf(it, models), `${it.model}${it.basedOn ? " — " + it.basedOn : ""}`);
  };

  // Ghosts: faded copies left at the dragged block's place.
  const ghosts = drag ? L.nodes.filter((n) => n.it && moving(n.it.id)).map((n) => ({ ...n, ghost: true })) : [];
  // Draw the dragged blocks last (on top).
  const ordered = [...L.nodes].sort((a, b) => Number(!!(a.it && moving(a.it.id))) - Number(!!(b.it && moving(b.it.id))));

  return (
    <div
      className={`flow-stage${drag ? " dragging" : ""}`}
      ref={stageRef}
      tabIndex={0}
      aria-label="Block chain"
      onClick={onClick}
      onDoubleClick={onDoubleClick}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={() => endDrag(false)}
      onPointerCancel={() => endDrag(true)}
      onKeyDown={(e) => { if (e.key === " " && typeof sel === "number") { e.preventDefault(); void toggleBlock(sel); } }}
    >
      <svg
        ref={svgRef}
        className={`chain${L.twoPaths ? " two-paths" : ""}`}
        viewBox={`0 0 ${L.width} ${L.height}`}
        width={L.width * SCALE}
        height={L.height * SCALE}
        style={{ width: L.width * SCALE, left: marginLeft, top: marginTop }}
        role="img"
        aria-label="Block chain"
      >
        <g className="wires">{wires}</g>
        {ghosts.length > 0 && <g aria-hidden="true">{ghosts.map((n) => <GhostOf key={`g${n.it!.id}`} n={n} preset={preset} models={models} />)}</g>}
        {ordered.map(node)}
        {links}
      </svg>
    </div>
  );
}

/** A faded copy of a block at its original place while it is dragged. */
function GhostOf({ n, preset, models }: { n: FlowNode; preset: PresetView; models: ReturnType<typeof useEditor>["models"] }) {
  const it = n.it!;
  const amp = it.kind === "cab" ? preset.items[it.id === 1 ? 0 : 2] : null;
  const on = it.kind === "cab" ? !it.empty && !!amp?.active : it.active;
  if (it.empty && it.kind !== "cab") {
    return it.kind === "amp"
      ? <g className="blk blk-ghost empty amp-empty"><rect className="amp-empty-box" x={n.x - H} y={n.y - H} width={BOX} height={BOX} rx={3} /></g>
      : <g className="blk blk-ghost empty slot-empty"><rect className="slot-box" x={n.x - 8} y={n.y - 8} width={16} height={16} rx={4} /></g>;
  }
  const cat = it.kind === "cab" ? CATS.cab : catOf(it, models);
  return (
    <g className={`blk blk-ghost${on ? " on" : ""}`} style={{ "--cat": cat.color } as React.CSSProperties}>
      <rect className="blk-box" x={n.x - H} y={n.y - H} width={BOX} height={BOX} rx={3} />
      <Glyph cat={cat} x={n.x} y={n.y} size={cat.svg ? ICON_SIZE : 18} />
    </g>
  );
}
