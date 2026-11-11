// Block editor (HX Edit): EDIT | BYPASS/CONTROLLER ASSIGN tabs; EDIT has the
// categories, models and parameters of the block selected in the chain.
// Ported from the classic UI (renderEditor / editorCats / renderFxModels /
// renderAmpCabModels, renderAuto / initAuto).
import { useEffect, useRef, useState, type CSSProperties } from "react";
import { post } from "../../api";
import { useEditor } from "../../store";
import { CATS, NONE_CAT, inputIcon, isPreamp, type Cat } from "../../flow/icons";
import type { EdItem, EdModelGroup, EdPreset } from "../../editor/format";
import { AmpIcon } from "./AmpIcon";
import { CabIcon } from "./CabArt";
import { GearPicture } from "./GearPicture";
import { CatIcon } from "./PrmRow";
import { InputSvg } from "./InputParams";
import { ParamsPage } from "./Params";
import { AssignTab } from "./AssignTab";

const FX_NONE = 0x020dffff;
const XSvg = () => <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" /></svg>;
const css = (cat: string) => ({ "--cat": cat }) as CSSProperties;

interface CatEntry { name: string; group?: string }

function catStyle(name: string): Cat {
  if (name === "Preamp" || name === "Preamps") return CATS.preamp;
  if (name === "Amp" || name === "Amps") return CATS.amp;
  if (name === "Cab" || name === "Cabs") return CATS.cab;
  return CATS[name] || CATS.none;
}

// Categories offered for the selected block and the one it belongs to. Amp
// and cab are separate blocks (the cab always right after its amp):
//   amp: None (disables the amp), Amp, Preamp
//   cab: None ("No Cab": the cab block stays, disabled), Cab
function editorCats(it: EdItem, models: EdModelGroup[]): { list: CatEntry[]; own: string } {
  if (it.kind === "amp") return { list: [{ name: "None", group: "Amps" }, { name: "Amp", group: "Amps" }, { name: "Preamp", group: "Preamps" }], own: it.empty ? "None" : isPreamp(it, models) ? "Preamp" : "Amp" };
  if (it.kind === "cab") return { list: [{ name: "None", group: "Cabs" }, { name: "Cab", group: "Cabs" }], own: it.empty ? "None" : "Cab" };
  return { list: [{ name: "None" }, ...models.filter((g) => g.kind === "fx").map((g) => ({ name: g.name }))], own: it.empty ? "None" : it.group ?? "None" };
}

const GridSvg = () => <svg viewBox="0 0 16 16" aria-hidden="true">{[1, 6.3, 11.6].flatMap((y) => [1, 6.3, 11.6].map((x) => <rect key={`${x},${y}`} x={x} y={y} width="3.4" height="3.4" rx=".6" />))}</svg>;
const ListSvg = () => <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.5 3h1.6M1.5 8h1.6M1.5 13h1.6M5.5 3h9M5.5 8h9M5.5 13h9" /></svg>;

const CatHead = ({ cat, name, onGrid }: { cat: Cat; name: string; onGrid?: () => void }) => (
  <div className="ed-colhead ed-cathead" style={css(cat.color)}><CatIcon cat={cat} /><span>{name}</span>
    {onGrid && <button className="ed-grid" type="button" aria-label="Grid view" title="Grid view" onClick={onGrid}><GridSvg /></button>}</div>
);

/** The "None" category's only entry: "× (empty block)" (type = the API's none model). */
const EmptyItem = ({ cur, onPick }: { cur: boolean; onPick: () => void }) => (
  <button className={`ed-item ed-empty${cur ? " cur" : ""}`} type="button" style={css(NONE_CAT.color)} onClick={() => { if (!cur) onPick(); }}><XSvg /><span>(empty block)</span></button>
);

export function BlockEditor() {
  const { state, meta, sel, toast, refetchPreset } = useEditor();
  const preset = useEditor().preset as EdPreset | null;
  const models = useEditor().models as EdModelGroup[];
  const [tab, setTab] = useState<"edit" | "auto">("edit");
  const [cat, setCat] = useState<string | null>(null); // category shown (null = the block's own)
  const [sub, setSub] = useState<"guitar" | "bass" | null>(null); // Guitar / Bass list of amps & cabs
  const [subOpen, setSubOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  // the model just picked, shown as selected until the pedal's preset is read back
  const [pending, setPending] = useState<{ id: number; type: number } | null>(null);
  // HX grid view: the categories become a dropdown and the models a grid of large pictures (amps)
  const [gridView, setGridView] = useState(() => { try { return localStorage.getItem("edGrid") === "1"; } catch { return false; } });
  const [catOpen, setCatOpen] = useState(false);
  const setGrid = (on: boolean) => { setGridView(on); setCatOpen(false); try { localStorage.setItem("edGrid", on ? "1" : "0"); } catch { /* private window */ } };
  const modelsEl = useRef<HTMLDivElement>(null);

  // another block: back to its own category
  useEffect(() => { setCat(null); setSub(null); setSubOpen(false); }, [sel]);

  const online = !!state?.connected;
  const it = preset && typeof sel === "number" ? preset.items[sel] : null;
  const info = it ? editorCats(it, models) : null;
  const show = info ? (info.list.some((g) => g.name === cat) ? cat! : info.own) : "";

  // keep the current model in view
  useEffect(() => { modelsEl.current?.querySelector(".ed-item.cur")?.scrollIntoView({ block: "nearest" }); }, [sel, show, sub, preset]);

  async function setModel(target: number, type: number) {
    setBusy(true);
    setPending({ id: target, type });
    try {
      await post("/api/block/model", { id: target, type });
      await refetchPreset(); // the pedal filled in the model's defaults
    } catch (e) {
      toast(`Couldn't change the model: ${(e as Error).message}`);
    } finally {
      setBusy(false);
      setPending(null);
    }
  }
  const isCur = (item: EdItem, type: number) => (pending && pending.id === item.id ? pending.type === type : !item.empty && item.type === type);

  // ---- categories and models columns
  let cats: React.ReactNode = null, list: React.ReactNode = null;
  if (online && preset) {
    if (sel === null) {
      cats = <span className="ed-hint">Select a block or an empty slot in the chain.</span>;
    } else if (sel === "input") { // one "Input" entry; Input 1 sources in the middle
      const grid = <svg viewBox="0 0 16 16" aria-hidden="true">{[1, 6.3, 11.6].flatMap((y) => [1, 6.3, 11.6].map((x) => <rect key={`${x},${y}`} x={x} y={y} width="3.4" height="3.4" rx=".6" />))}</svg>;
      cats = <>
        <div className="ed-colhead ed-cathead ed-inhead" style={css("#f2f3f5")}><InputSvg /><span>Input</span><button className="ed-grid" type="button" aria-label="Grid view" title="Grid view">{grid}</button></div>
        <button className="ed-item cur" type="button" style={css("#f2f3f5")}><InputSvg /><span>Input</span></button>
      </>;
      const setup = (meta?.setupInput as Record<string, number> | undefined) || {};
      list = <>
        <div className="ed-colhead ed-colhead-dark" title="Input 1 Source"><span>Input 1</span></div>
        {((meta?.input1 as { value: number; label: string }[] | undefined) || []).map((o) => {
          const ic = inputIcon(o.label);
          const cur = o.value === state?.input1;
          return (
            <button key={o.value} className={`ed-item${cur ? " cur" : ""}`} type="button" style={css("#f2f3f5")}
              onClick={() => { if (!cur) post("/api/input", { id: setup.input1, value: o.value }).catch((e: Error) => toast(`Couldn't change the input: ${e.message}`)); }}>
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d={ic.d} transform={ic.tf} /></svg><span>{o.label}</span>
            </button>
          );
        })}
      </>;
    } else if (sel === "mixer") {
      const mixItem = <button className="ed-item cur" type="button" style={css(CATS.mixer.color)}><CatIcon cat={CATS.mixer} /><span>Mixer</span></button>;
      cats = <><CatHead cat={CATS.mixer} name="Mixer" />{mixItem}</>;
      list = <><div className="ed-colhead ed-colhead-dark"><span>Mixer</span></div>{mixItem}</>;
    } else if (it && info) {
      cats = <>
        <CatHead cat={show === "None" ? NONE_CAT : catStyle(show)} name={show} onGrid={it.kind !== "fx" ? () => setGrid(true) : undefined} />
        {info.list.map((g) => {
          const st = g.name === "None" ? NONE_CAT : catStyle(g.name);
          return (
            <button key={g.name} className={`ed-item${g.name === show ? " cur" : ""}`} type="button" style={css(st.color)}
              onClick={() => { setCat(g.name); setSub(null); setSubOpen(false); }}>
              {g.name === "None" ? <XSvg /> : <CatIcon cat={st} />}<span>{g.name}</span>
            </button>
          );
        })}
      </>;
      if (it.kind === "amp" || it.kind === "cab") {
        // Amp / Preamp / Cab: None = only "(empty block)" (the API's none
        // model: Amp Disabled / No Cab); otherwise a Guitar / Bass dropdown.
        const group = info.list.find((g) => g.name === show)?.group;
        const ms = models.find((g) => g.name === group)?.models || [];
        if (show === "None") {
          const off = ms.find((m) => m.none);
          list = <><div className="ed-colhead ed-colhead-blank" />{off && <EmptyItem cur={it.empty} onPick={() => setModel(it.id, off.type)} />}</>;
        } else {
          const color = catStyle(show).color;
          const curBass = !it.empty && !!ms.find((m) => m.type === it.type)?.bass;
          const s = sub || (curBass ? "bass" : "guitar");
          const grid = gridView && (it.kind === "amp" || it.kind === "cab");
          const opts: ["guitar" | "bass", string][] = [["guitar", `Guitar ${show}`], ["bass", `Bass ${show}`]];
          list = <>
            <div className="ed-colhead ed-subhead">
              {grid && <>
                <div className="ed-gridcat">
                  <button className="ed-catbtn" type="button" style={css(color)} aria-haspopup="listbox" aria-expanded={catOpen} onClick={() => setCatOpen((o) => !o)}>
                    <CatIcon cat={catStyle(show)} /><span>{show}</span><svg viewBox="0 0 12 8" aria-hidden="true"><path d="M1.5 1.5 6 6l4.5-4.5" /></svg>
                  </button>
                  {catOpen && (
                    <div className="ed-submenu ed-catmenu" role="listbox">
                      {info.list.map((g) => (
                        <button key={g.name} className={`ed-subopt${g.name === show ? " cur" : ""}`} type="button" role="option" aria-selected={g.name === show}
                          onClick={() => { setCat(g.name); setSub(null); setCatOpen(false); if (g.name === "None") setGrid(false); }}>{g.name}</button>
                      ))}
                    </div>
                  )}
                </div>
                <button className="ed-grid ed-listbtn" type="button" aria-label="List view" title="List view" onClick={() => setGrid(false)}><ListSvg /></button>
              </>}
              <button className="ed-subbtn" type="button" aria-haspopup="listbox" aria-expanded={subOpen} onClick={() => setSubOpen((o) => !o)}>
                <span>{opts.find(([k]) => k === s)![1]}</span><svg viewBox="0 0 12 8" aria-hidden="true"><path d="M1.5 1.5 6 6l4.5-4.5" /></svg>
              </button>
              {subOpen && (
                <div className="ed-submenu" role="listbox">
                  {opts.map(([k, l]) => (
                    <button key={k} className={`ed-subopt${k === s ? " cur" : ""}`} type="button" role="option" aria-selected={k === s} onClick={() => { setSub(k); setSubOpen(false); }}>{l}</button>
                  ))}
                </div>
              )}
            </div>
            {grid ? <div className="ed-tiles">{ms.filter((m) => !m.none && !!m.bass === (s === "bass")).map((m) => {
              const cur = isCur(it, m.type);
              return (
                <button key={m.type} className={`ed-tile${cur ? " cur" : ""}`} type="button" title={m.basedOn || m.name} style={css(color)}
                  onClick={() => { if (subOpen) { setSubOpen(false); return; } if (!cur) void setModel(it.id, m.type); }}>
                  {m.style && <GearPicture style={m.style} kind={it.kind === "cab" ? "cab" : "amp"} />}<span>{m.name}</span>
                </button>
              );
            })}</div> : ms.filter((m) => !m.none && !!m.bass === (s === "bass")).map((m) => {
              const cur = isCur(it, m.type);
              return (
                <button key={m.type} className={`ed-item${cur ? " cur" : ""}`} type="button" title={m.basedOn || m.name} style={css(color)}
                  onClick={() => { if (subOpen) { setSubOpen(false); return; } if (!cur) void setModel(it.id, m.type); }}>
                  {m.style ? (it.kind === "cab" ? <CabIcon style={m.style} /> : <AmpIcon style={m.style} />) : <span className="dot" />}<span>{m.name}</span>
                </button>
              );
            })}
          </>;
        }
      } else if (show === "None") {
        list = <><div className="ed-colhead ed-colhead-blank" /><EmptyItem cur={it.empty} onPick={() => setModel(it.id, FX_NONE)} /></>;
      } else {
        const grp = models.find((g) => g.kind === "fx" && g.name === show);
        list = grp ? <>
          <div className="ed-colhead"><span>{show}</span></div>
          {grp.models.map((m) => {
            const cur = isCur(it, m.type);
            return (
              <button key={m.type} className={`ed-item${cur ? " cur" : ""}`} type="button" title={m.basedOn || m.name} style={css(catStyle(show).color)}
                onClick={() => { if (!cur) void setModel(it.id, m.type); }}>
                <span className="dot" /><span>{m.name}</span>
              </button>
            );
          })}
        </> : null;
      }
    }
  }

  return (
    <section className={`panel editors${tab === "auto" ? " auto" : ""}${gridView && (it?.kind === "amp" || it?.kind === "cab") && show !== "None" ? " gridview" : ""}${busy ? " ed-busy" : ""}`} aria-label="Block editor">
      <div className="ed-tabs" role="tablist">
        <button className={`ed-tab${tab === "edit" ? " cur" : ""}`} type="button" role="tab" aria-selected={tab === "edit"} onClick={() => setTab("edit")}>Edit</button>
        <button className={`ed-tab${tab === "auto" ? " cur" : ""}`} type="button" role="tab" aria-selected={tab === "auto"} onClick={() => setTab("auto")}>Bypass/Controller Assign</button>
      </div>
      <div className="ed-auto" role="tabpanel" aria-label="Bypass/Controller Assign">
        {tab === "auto" && <AssignTab />}
      </div>
      <div className="ed-col ed-cats" role="listbox" aria-label="Categories">{cats}</div>
      <div className="ed-col ed-models" role="listbox" aria-label="Models" ref={modelsEl}>{list}</div>
      <ParamsPage />
    </section>
  );
}
