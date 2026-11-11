// Editor state shared by the components: the live State (Server-Sent Events),
// the loaded preset (re-read when bufferRev changes), option lists and models,
// the selected block and toasts. No pedal logic here: that lives in the API.
import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { getJSON, type Meta, type ModelGroup, type PresetView, type State } from "./api";
import { defaultSel } from "./flow/layout";

export type Sel = number | "input" | "mixer" | null;
export interface ToastAction { label: string; run: () => void }
export interface Toast { id: number; msg: string; ok: boolean; action?: ToastAction }

interface Editor {
  state: State | null;
  link: "connecting" | "open" | "down"; // the event stream to the editor server
  meta: Meta | null;
  models: ModelGroup[];
  preset: PresetView | null;
  presetError: string;
  sel: Sel;
  select: (s: Sel) => void;
  refetchPreset: () => Promise<void>;
  /** Locally changed preset (optimistic edits), replaced by the next read. */
  patchPreset: (fn: (p: PresetView) => PresetView) => void;
  toast: (msg: string, ok?: boolean, action?: ToastAction) => void;
  dismissToast: (id: number) => void;
  toasts: Toast[];
}

const Ctx = createContext<Editor | null>(null);

export function useEditor(): Editor {
  const e = useContext(Ctx);
  if (!e) throw new Error("useEditor outside EditorProvider");
  return e;
}

export function EditorProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<State | null>(null);
  const [link, setLink] = useState<Editor["link"]>("connecting");
  const [meta, setMeta] = useState<Meta | null>(null);
  const [models, setModels] = useState<ModelGroup[]>([]);
  const [preset, setPreset] = useState<PresetView | null>(null);
  const [presetError, setPresetError] = useState("");
  const [sel, setSel] = useState<Sel>(null);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const fetching = useRef<{ busy: boolean; again: boolean }>({ busy: false, again: false });
  const rev = useRef(-1);
  const presetKey = useRef("");
  const modelsRef = useRef<ModelGroup[]>([]);
  modelsRef.current = models;

  const dismissToast = useCallback((id: number) => setToasts((t) => t.filter((x) => x.id !== id)), []);
  const toast = useCallback((msg: string, ok = false, action?: ToastAction) => {
    const id = Date.now() + Math.random();
    setToasts((t) => [...t, { id, msg, ok, action }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), action ? 10000 : ok ? 3500 : 6000);
  }, []);

  // One read at a time; a change during a read triggers one more.
  const refetchPreset = useCallback(async () => {
    const f = fetching.current;
    if (f.busy) { f.again = true; return; }
    f.busy = true;
    try {
      const v = await getJSON<PresetView>("/api/preset");
      setPreset(v);
      setPresetError("");
      setSel((s) => {
        if (s === null) return defaultSel(v, modelsRef.current);
        // a cab is shown only while its amp exists
        if ((s === 1 && v.items[0].empty) || (s === 3 && v.items[2].empty)) return s - 1;
        return s;
      });
    } catch (e) {
      setPresetError((e as Error).message);
    } finally {
      f.busy = false;
      if (f.again) { f.again = false; void refetchPreset(); }
    }
  }, []);

  useEffect(() => {
    let server = "";
    getJSON<Meta>("/api/meta").then((m) => { setMeta(m); server = m.server ?? ""; }).catch(() => {});
    getJSON<ModelGroup[]>("/api/models").then(setModels).catch(() => {});
    const es = new EventSource("/api/events");
    es.onmessage = (ev) => setState(JSON.parse(ev.data) as State);
    es.onopen = async () => {
      setLink("open");
      // another server run (maybe a new build): reload to get the new page
      try {
        const m = await getJSON<Meta>("/api/meta");
        if (server && m.server !== server) location.reload();
      } catch { /* still restarting */ }
    };
    es.onerror = () => setLink("down");
    // The pedal doesn't announce changes made on its front panel to global
    // settings: re-read them when the user comes back to the window.
    const onFocus = () => { void fetch("/api/refresh?globals=1", { method: "POST" }).catch(() => {}); };
    window.addEventListener("focus", onFocus);
    return () => { es.close(); window.removeEventListener("focus", onFocus); };
  }, []);

  // React to the live state: offline clears; another preset drops the
  // selection; a new bufferRev re-reads the preset (not with the tuner on:
  // the pedal ignores preset reads then).
  useEffect(() => {
    if (!state) return;
    if (!state.connected) { setPreset(null); rev.current = -1; return; }
    const key = `${state.setlist}:${state.slot}`;
    if (key !== presetKey.current) { presetKey.current = key; setSel(null); }
    if (state.bufferRev !== rev.current && !state.tuner?.on) { rev.current = state.bufferRev; void refetchPreset(); }
  }, [state, refetchPreset]);

  const patchPreset = useCallback((fn: (p: PresetView) => PresetView) => setPreset((p) => (p ? fn(p) : p)), []);

  const value: Editor = { state, link, meta, models, preset, presetError, sel, select: setSel, refetchPreset, patchPreset, toast, dismissToast, toasts };
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
