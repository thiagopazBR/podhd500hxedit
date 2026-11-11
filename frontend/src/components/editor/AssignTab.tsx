// BYPASS/CONTROLLER ASSIGN tab (HX Edit style, pages 39–43 of its guide):
// Left: every assignment of the preset (footswitch or controller);
// Right: the selected block's assignment, controller/footswitch choice,
// Min/Max range faders, and optional MIDI In CC view.
// Ported from the classic UI (cmd/hd500-edit/web/flow.js renderAuto, initAuto).
import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { createPortal } from "react-dom";
import { useEditor } from "../../store";
import { CATS, catOf } from "../../flow/icons";
import { fmtParam, type EdItem, type EdPreset } from "../../editor/format";
import {
  ASSIGN_BLOCKS,
  BYPASS,
  MIDI_OPT,
  type SortCol,
  type SourceOption,
  assignmentsOf,
  sortRows,
  assignOptions,
  sourceOptions,
  sourceCc,
  setBypassSource,
  setControllerSource,
  setRange,
  setMidiChannel,
  clearOne,
  clearAll,
} from "../../assign/assign";
import { CatIcon, PrmRow } from "./PrmRow";

const COLS = ["Block", "Parameter", "Source"] as const;

interface DropdownProps {
  options: { value: number; label: string; bar?: string; on?: boolean }[];
  value: number;
  onSelect: (v: number) => void;
}

function Dropdown({ options, value, onSelect }: DropdownProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const cur = options.find((o) => o.value === value) || options[0] || { value, label: "—" };
  const barText = cur.bar ?? cur.label;

  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", onDown);
    document.addEventListener("keydown", onKey);
    listRef.current?.querySelector(".cur")?.scrollIntoView({ block: "nearest" });
    return () => {
      document.removeEventListener("pointerdown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div className="as-dd" ref={ref}>
      <button
        type="button"
        className="as-dd-btn"
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <span>{barText}</span>
        <svg viewBox="0 0 12 8" aria-hidden="true">
          <path d="M1.5 1.5 6 6l4.5-4.5" />
        </svg>
      </button>
      {open && (
        <div className="as-dd-list" role="listbox" ref={listRef}>
          {options.map((o) => {
            const isCur = o.value === value;
            return (
              <button
                key={o.value}
                type="button"
                role="option"
                className={`${isCur ? "cur" : ""}${o.on ? " on" : ""}`}
                aria-selected={isCur}
                onClick={() => {
                  setOpen(false);
                  if (o.value !== value) onSelect(o.value);
                }}
              >
                {o.label}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function AssignTab() {
  const { state, meta, sel, select, refetchPreset, toast } = useEditor();
  const preset = useEditor().preset as EdPreset | null;
  const models = useEditor().models;

  const [sort, setSort] = useState<{ col: SortCol; dir: 1 | -1 }>({ col: 0, dir: 1 });
  const [assignK, setAssignK] = useState<number>(BYPASS);
  const [showMidi, setShowMidi] = useState<boolean>(false);
  const [menu, setMenu] = useState<{ id: number; k: number; x: number; y: number } | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  // Close context menu on outside click or Escape
  useEffect(() => {
    if (!menu) return;
    const onDown = (e: PointerEvent) => {
      if (!menuRef.current?.contains(e.target as Node)) {
        setMenu(null);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMenu(null);
    };
    document.addEventListener("pointerdown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [menu]);

  const rawRows = useMemo(() => (preset ? assignmentsOf(preset, meta) : []), [preset, meta]);
  const rows = useMemo(() => (preset ? sortRows(rawRows, preset, sort.col, sort.dir) : []), [rawRows, preset, sort]);

  // Determine active block: sel if valid, cabs follow their amp, else fallback to first assigned
  let targetId: number | null = typeof sel === "number" ? (sel === 1 ? 0 : sel === 3 ? 2 : sel) : null;
  if (targetId !== null && (!preset?.items[targetId] || preset.items[targetId].empty || !ASSIGN_BLOCKS.includes(targetId))) {
    targetId = null;
  }
  if (targetId === null && rows.length > 0) {
    targetId = rows[0].id;
  }

  const it = targetId !== null && preset?.items[targetId] ? (preset.items[targetId] as EdItem) : null;

  // When block changes outside of row click, reset assignK and showMidi
  const prevIdRef = useRef<number | null>(targetId);
  useEffect(() => {
    if (prevIdRef.current !== targetId) {
      prevIdRef.current = targetId;
      setAssignK(BYPASS);
      setShowMidi(false);
    }
  }, [targetId]);

  const k = it && (it.params ?? []).some((q) => q.index === assignK) ? assignK : BYPASS;
  const p = it && k >= 0 ? (it.params ?? []).find((q) => q.index === k) ?? null : null;
  const cat = it ? catOf(it, models) : CATS.none;

  const isBypass = !p;
  const curSourceVal = p ? p.ctl : it?.fs ?? 0;
  const cc = useMemo(() => {
    if (!it) return undefined;
    return sourceCc(meta, isBypass, curSourceVal);
  }, [meta, isBypass, curSourceVal, it]);

  const sourceDropdownOpts = useMemo(() => {
    if (!preset) return [];
    const baseOpts = sourceOptions(preset, meta, isBypass).map((o: SourceOption) => ({
      ...o,
      on: Boolean(o.bar),
    }));
    if (cc !== undefined) {
      return [...baseOpts, { value: MIDI_OPT, label: "MIDI CC" }];
    }
    return baseOpts;
  }, [preset, meta, isBypass, cc]);

  const assignMenuOpts = useMemo(() => {
    if (!it) return [];
    return assignOptions(it).map((o) => ({
      ...o,
      on: o.value === BYPASS ? (it.fs ?? 0) !== 0 : Boolean(it.params?.find((q) => q.index === o.value)?.ctl),
    }));
  }, [it]);

  if (!preset || !state?.connected) return null;

  const sendRange = (which: "min" | "max", val: number) => {
    if (!it || !p) return;
    if (which === "min") p.lo = val;
    else p.hi = val;
    setRange(it.id, k, which, val).catch((e: Error) => {
      toast(`Couldn't change the assignment: ${e.message}`);
    });
  };

  const midiCh = state.midiChannel || 0;

  return (
    <div className="as-wrap">
      <div className="as-list">
        <div className="as-cols">
          {COLS.map((colName, idx) => {
            const isSort = sort.col === idx;
            return (
              <button
                key={colName}
                type="button"
                className={isSort ? "on" : ""}
                title={`Sort by ${colName}`}
                onClick={() => {
                  setSort((s) => (s.col === idx ? { col: idx as SortCol, dir: (s.dir > 0 ? -1 : 1) as 1 | -1 } : { col: idx as SortCol, dir: 1 }));
                }}
              >
                {colName}
                {isSort && (
                  <svg viewBox="0 0 12 8" aria-hidden="true">
                    <path d={sort.dir > 0 ? "M1.5 6.5 6 2l4.5 4.5" : "M1.5 1.5 6 6l4.5-4.5"} />
                  </svg>
                )}
              </button>
            );
          })}
        </div>
        {rows.length === 0 ? (
          <p className="as-none">No bypass or controller assignments in this preset.</p>
        ) : (
          rows.map((r) => {
            const b = preset.items[r.id];
            const bCat = catOf(b, models);
            const isMine = r.id === targetId;
            const isCur = isMine && r.k === k;
            const cls = `as-row${isMine ? " mine" : ""}${isCur ? " cur" : ""}`;
            return (
              <button
                key={`${r.id}-${r.k}`}
                type="button"
                className={cls}
                style={{ "--cat": bCat.color } as CSSProperties}
                onClick={() => {
                  prevIdRef.current = r.id;
                  select(r.id);
                  setAssignK(r.k);
                  setShowMidi(false);
                }}
                onContextMenu={(e) => {
                  e.preventDefault();
                  setMenu({
                    id: r.id,
                    k: r.k,
                    x: Math.min(e.clientX, window.innerWidth - 180),
                    y: Math.min(e.clientY, window.innerHeight - 80),
                  });
                }}
              >
                <span className="as-blk">{b.model}</span>
                <span>{r.param}</span>
                <span>{r.source}</span>
              </button>
            );
          })
        )}
      </div>

      {!it ? (
        <div className="as-detail">
          <div className="ac-tabs"><div className="ac-blank" /></div>
        </div>
      ) : (
        <div className="as-detail" style={{ "--cat": cat.color } as CSSProperties}>
          <div className="as-head">
            <div className="as-hrow">
              <div className="as-blk-title" style={{ "--cat": cat.color } as CSSProperties}>
                <CatIcon cat={cat} />
                <span>{it.model}</span>
              </div>
              <div className="as-assign">
                <Dropdown
                  options={assignMenuOpts}
                  value={k}
                  onSelect={(val) => {
                    setAssignK(val);
                    setShowMidi(false);
                  }}
                />
              </div>
            </div>
            <div className="as-hrow">
              <span className="as-hlabel">{p ? "Controller" : "Bypass"}</span>
              <Dropdown
                options={sourceDropdownOpts}
                value={showMidi && cc !== undefined ? MIDI_OPT : curSourceVal}
                onSelect={async (val) => {
                  if (val === MIDI_OPT) {
                    setShowMidi(true);
                    return;
                  }
                  setShowMidi(false);
                  try {
                    if (p) {
                      await setControllerSource(it.id, k, val);
                    } else {
                      await setBypassSource(it.id, val);
                    }
                    await refetchPreset();
                  } catch (e) {
                    toast(`Couldn't change the assignment: ${(e as Error).message}`);
                  }
                }}
              />
            </div>
          </div>

          {p && p.ctl ? (
            <div className="params">
              <PrmRow
                key={`as.${it.id}.${k}.min`}
                name={`Min ${p.name}`}
                desc={{
                  value: p.lo ?? 0,
                  steps: p.kind === "list" && p.options ? p.options.length : 0,
                  options: p.kind === "list" ? p.options : undefined,
                  fmt: (x) => fmtParam({ ...p, value: x }),
                  send: (x) => sendRange("min", x),
                }}
              />
              <PrmRow
                key={`as.${it.id}.${k}.max`}
                name={`Max ${p.name}`}
                desc={{
                  value: p.hi ?? 1,
                  steps: p.kind === "list" && p.options ? p.options.length : 0,
                  options: p.kind === "list" ? p.options : undefined,
                  fmt: (x) => fmtParam({ ...p, value: x }),
                  send: (x) => sendRange("max", x),
                }}
              />
            </div>
          ) : null}

          {showMidi && cc !== undefined ? (
            <div className="params as-midi">
              <PrmRow
                name="MIDI Channel"
                desc={{
                  value: midiCh ? (midiCh - 1) / 15 : 0,
                  steps: 16,
                  fmt: (x) => (midiCh || x ? `CH ${1 + Math.round(x * 15)}` : "—"),
                  send: (x) => {
                    setMidiChannel(1 + Math.round(x * 15)).catch((e: Error) => {
                      toast(`Couldn't change MIDI channel: ${e.message}`);
                    });
                  },
                }}
              />
              <div className="prm ro">
                <span className="prm-name">MIDI CC</span>
                <span className="prm-valbox">
                  <span className="prm-val">{`CC ${cc}`}</span>
                  <span className="prm-step">
                    <button type="button" disabled aria-label="Fixed">
                      <svg viewBox="0 0 7 4" aria-hidden="true"><path d="M0 4L3.5 0L7 4Z" /></svg>
                    </button>
                    <button type="button" disabled aria-label="Fixed">
                      <svg viewBox="0 0 7 4" aria-hidden="true"><path d="M0 0L3.5 4L7 0Z" /></svg>
                    </button>
                  </span>
                </span>
                <span
                  className="prm-bar"
                  aria-label="MIDI CC"
                  aria-valuenow={Math.round((cc / 127) * 100)}
                  style={{ "--v": cc / 127 } as CSSProperties}
                >
                  <i />
                  <b />
                </span>
              </div>
            </div>
          ) : null}
        </div>
      )}

      {menu && createPortal(
        <div
          ref={menuRef}
          className="menu as-menu"
          role="menu"
          style={{ position: "fixed", left: menu.x, top: menu.y, zIndex: 60 }}
        >
          <button
            type="button"
            role="menuitem"
            onClick={async () => {
              const target = menu;
              setMenu(null);
              try {
                await clearOne(target.id, target.k);
                await refetchPreset();
              } catch (e) {
                toast(`Couldn't clear: ${(e as Error).message}`);
              }
            }}
          >
            Clear Assignment
          </button>
          <button
            type="button"
            role="menuitem"
            onClick={async () => {
              setMenu(null);
              try {
                await clearAll();
                await refetchPreset();
              } catch (e) {
                toast(`Couldn't clear: ${(e as Error).message}`);
              }
            }}
          >
            Clear All Assignments
          </button>
        </div>,
        document.body
      )}
    </div>
  );
}
