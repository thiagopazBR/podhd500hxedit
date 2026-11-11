// Presets state and actions shared by the presets panel and the header's
// "…" menu: the setlist shown in the list, the slots' names, the selection,
// the clipboard, renames, copy/move/swap, SEND/RECEIVE, import/export.
// Ported from the classic UI (app.js "pres" and its functions); the pedal
// logic stays in the API. The state lives in a mutable object (as in the
// classic UI) and a version counter re-renders the consumers.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { apiError, post, type State } from "./api";
import { useEditor } from "./store";

export interface SetlistInfo { index: number; name: string; stored: string; edited: boolean }
type Clip = { setlist: number; slots: number[] };
export type Scope = "selected" | "setlist" | "all";
export type PresetAct = "load" | "rename" | "copy" | "paste" | "import" | "export" | "send" | "receive";

interface Pres {
  view: number | null; // setlist shown in the list (may differ from the pedal's)
  names: string[]; // stored names of the viewed setlist ("" = not read yet)
  edited: boolean[]; // slots changed in the editor and not sent yet
  sel: number | null; // anchor of the selection (last plain/cmd click)
  selected: Set<number>; // highlighted slots
  renaming: number | null; // slot whose name is being edited
  rev: number; // last libraryRev fetched
  setlistsLoaded: boolean;
  setlists: SetlistInfo[];
  fetching: boolean;
  refetch: boolean;
  clipboard: Clip | null;
  transferRunning: boolean;
  filesRunning: boolean;
  lastCurrent: string | null;
}

export const MAX_NAME = 16;
export const slotLabel = (i: number) => String(Math.floor(i / 4) + 1).padStart(2, "0") + "ABCD"[i % 4];
// The pedal's LCD only has plain ASCII: strip accents (Ação -> Acao) and drop
// anything else that isn't printable ASCII.
export const toPedalChars = (s: string) => s.normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/[^\x20-\x7E]/g, "");
export const FILE_RE = /\.(h5e|h5s|h5b|5xe|5xs|5xb)$/i;

interface PresetsApi {
  p: Pres;
  version: number;
  state: State | null;
  setlistName: (i: number) => string;
  showSetlist: (n: number) => void;
  render: () => void;
  select: (i: number, mod: { shift?: boolean; meta?: boolean }) => void;
  selectOnly: (i: number) => void;
  loadPreset: (slot: number) => Promise<void>;
  startRename: (i: number) => void;
  finishRename: (i: number, value: string, commit: boolean) => Promise<void>;
  arrange: (op: "move" | "copy" | "swap", from: { setlist: number; slot: number }[], to: { setlist: number; slot: number }) => Promise<void>;
  copySelection: () => void;
  pasteClipboard: () => void;
  pendingInScope: (scope: Scope) => string[];
  runTransfer: (kind: "send" | "receive", scope: Scope) => Promise<void>;
  canImport: () => boolean;
  exportFiles: (kind: "preset" | "setlist" | "bundle") => void;
  importFiles: (files: File[], target: number | null | undefined) => void;
  pickFiles: (accept: string) => void;
  backup: () => void;
  renameSetlist: (target: number, name: string) => Promise<void>;
  presetAction: (act: PresetAct, one: number) => void;
  /** The header's "…" menu: act on the loaded preset through the list. */
  headerAction: (act: PresetAct, openPanel: () => void) => void;
  registerFileInput: (el: HTMLInputElement | null) => void;
}

const Ctx = createContext<PresetsApi | null>(null);
export function usePresets(): PresetsApi {
  const c = useContext(Ctx);
  if (!c) throw new Error("usePresets outside PresetsProvider");
  return c;
}

async function download(url: string): Promise<string> {
  const res = await fetch(url);
  if (!res.ok) throw await apiError(res);
  const cd = res.headers.get("Content-Disposition") || "";
  const m = /filename\*=UTF-8''([^;]+)/.exec(cd);
  const name = m ? decodeURIComponent(m[1]) : "preset.h5e";
  const blob = await res.blob();
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = name;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 10000);
  return name;
}

const fileKind = async (f: File) => {
  const magic = await f.slice(0, 4).text();
  return ({ H5EP: "preset", H5ES: "setlist", H5EB: "bundle" } as Record<string, string>)[magic] || null;
};

export function PresetsProvider({ children }: { children: ReactNode }) {
  const { state, toast } = useEditor();
  const [version, setVersion] = useState(0);
  const render = useCallback(() => setVersion((v) => v + 1), []);
  const ref = useRef<Pres>({
    view: null, names: [], edited: [], sel: null, selected: new Set(), renaming: null, rev: -1,
    setlistsLoaded: false, setlists: [], fetching: false, refetch: false, clipboard: null,
    transferRunning: false, filesRunning: false, lastCurrent: null,
  });
  const p = ref.current;
  const stRef = useRef<State | null>(state);
  stRef.current = state;
  const prevRef = useRef<State | null>(null);
  const fileInput = useRef<HTMLInputElement | null>(null);

  const api = useMemo<PresetsApi>(() => {
    const st = () => stRef.current;
    const setlistName = (i: number) => p.setlists[i]?.name || `Setlist ${i + 1}`;

    async function fetchSetlist() {
      if (p.view === null) return;
      if (p.fetching) { p.refetch = true; return; }
      p.fetching = true;
      try {
        const want = p.view;
        const res = await fetch(`/api/setlist?n=${want}`);
        if (!res.ok) throw await apiError(res);
        const v = (await res.json()) as { setlist: number; names: string[]; edited?: boolean[] };
        if (v.setlist === p.view) { p.names = v.names; p.edited = v.edited || []; render(); }
      } catch (e) {
        toast(`Couldn't read the setlist: ${(e as Error).message}`);
      } finally {
        p.fetching = false;
        if (p.refetch) { p.refetch = false; void fetchSetlist(); }
      }
    }

    async function loadSetlistNames() {
      p.setlistsLoaded = true;
      try {
        const res = await fetch("/api/setlists");
        if (!res.ok) throw await apiError(res);
        p.setlists = (await res.json()) as SetlistInfo[];
        render();
      } catch (e) {
        p.setlistsLoaded = false;
        toast(`Couldn't read the setlists: ${(e as Error).message}`);
      }
    }

    function showSetlist(n: number) {
      p.view = n;
      p.sel = null;
      p.selected = new Set();
      p.names = [];
      p.edited = [];
      render();
      void fetchSetlist();
    }

    function select(i: number, mod: { shift?: boolean; meta?: boolean }) {
      if (mod.shift && p.sel !== null) { // range from the anchor
        const [a, b] = [Math.min(p.sel, i), Math.max(p.sel, i)];
        if (!mod.meta) p.selected.clear();
        for (let k = a; k <= b; k++) p.selected.add(k);
      } else if (mod.meta) { // toggle one
        if (p.selected.has(i)) p.selected.delete(i); else p.selected.add(i);
        p.sel = i;
      } else { // single: one click only selects (HX Edit); renaming is in the context menu / F2
        p.selected = new Set([i]);
        p.sel = i;
      }
      render();
    }
    function selectOnly(i: number) { p.selected = new Set([i]); p.sel = i; render(); }

    async function loadPreset(slot: number) {
      const s = st();
      if (!s || !s.connected || p.view === null) return;
      const same = s.setlist === p.view && s.slot === slot;
      if (s.dirty) {
        const what = same ? "Reloading this preset" : "Loading another preset";
        if (!window.confirm(`${what} discards the unsaved changes of "${s.presetName}" (${s.slotLabel}). Continue?`)) return;
      }
      try {
        await post("/api/select", { setlist: p.view, slot });
      } catch (e) {
        toast(`Couldn't load the preset: ${(e as Error).message}`);
      }
    }

    // the name a slot shows: the loaded preset shows the edit buffer's name
    // (live renames), unless the editor holds a pending copy/move for it
    const shownName = (i: number) => {
      const s = st();
      const isCur = !!s?.connected && !s.waitingTuner && s.setlist === p.view && s.slot === i;
      return isCur && !p.edited[i] ? s!.presetName || "" : p.names[i] || "";
    };

    function startRename(i: number) {
      const s = st();
      if (!s || !s.connected || p.renaming !== null || p.transferRunning) return;
      if (!shownName(i) && !(s.setlist === p.view && s.slot === i)) {
        toast("Wait for the setlist to finish loading before renaming.");
        return;
      }
      p.renaming = i;
      render();
    }

    async function finishRename(i: number, value: string, commit: boolean) {
      if (p.renaming !== i) return;
      const before = shownName(i);
      const name = toPedalChars(value).trim();
      p.renaming = null;
      render();
      if (!commit || !name || name === before) return;
      p.names[i] = name; // shown until the re-read
      render();
      try {
        await post("/api/rename", { setlist: p.view, slot: i, name });
      } catch (e) {
        p.names[i] = before;
        render();
        toast(`Couldn't rename: ${(e as Error).message}`);
      }
      void fetchSetlist();
    }

    async function arrange(op: "move" | "copy" | "swap", from: { setlist: number; slot: number }[], to: { setlist: number; slot: number }) {
      try {
        await post("/api/arrange", { op, from, to });
        if (to.setlist === p.view) { // select where the preset(s) ended up
          const n = op === "copy" ? from.length : 1;
          p.selected = new Set(Array.from({ length: n }, (_, k) => to.slot + k));
          p.sel = to.slot;
        }
        const verb = op === "move" ? "Moved" : op === "swap" ? "Swapped" : `Copied ${from.length}`;
        toast(`${verb}. Pending in the editor (*): SEND to save or RECEIVE to undo.`, true);
        void fetchSetlist();
        render();
      } catch (e) {
        toast((e as Error).message);
      }
    }

    function copySelection() {
      if (!p.selected.size || p.view === null) return;
      p.clipboard = { setlist: p.view, slots: [...p.selected].sort((a, b) => a - b) };
      toast(`Copied: ${p.clipboard.slots.map(slotLabel).join(", ")}. Select the destination and paste (Paste or Cmd+V).`, true);
      render();
    }

    function pasteClipboard() {
      const clip = p.clipboard;
      if (!clip || p.view === null) return;
      const target = p.selected.size ? Math.min(...p.selected) : p.sel;
      if (target === null || target === undefined) { toast("Select the destination slot."); return; }
      const from = clip.slots.map((slot) => ({ setlist: clip.setlist, slot }));
      const names = clip.slots.length === 1 ? slotLabel(target)
        : `${slotLabel(target)}–${slotLabel(Math.min(63, target + clip.slots.length - 1))}`;
      if (!window.confirm(`Paste ${clip.slots.length} preset(s) over ${names} in this setlist?`)) return;
      void arrange("copy", from, { setlist: p.view, slot: target });
    }

    // What a SEND in this scope would write, as far as the UI knows (the
    // server decides for real): the loaded preset if edited + editor-edited slots.
    function pendingInScope(scope: Scope): string[] {
      const s = st();
      const inScope = (set: number | null, slot: number) => scope === "all" || (set === p.view && (scope === "setlist" || p.selected.has(slot)));
      const out: string[] = [];
      if (scope !== "selected") { // setlist renames go with Set List / All
        for (const x of p.setlists) if (x.edited && (scope === "all" || x.index === p.view)) out.push(`Setlist ${x.index + 1} name: "${x.stored}" → "${x.name}"`);
      }
      const curDirty = !!s && !!s.dirty && inScope(s.setlist, s.slot);
      if (curDirty) out.push(`${s!.slotLabel} ${s!.presetName}`);
      p.edited.forEach((e, i) => {
        if (!e || !inScope(p.view, i)) return;
        const isCur = !!s && s.setlist === p.view && s.slot === i;
        if (isCur && curDirty) return;
        out.push(`${slotLabel(i)} ${p.names[i] || ""}${isCur ? " (loaded: the pedal reloads it)" : ""}`);
      });
      return out;
    }

    async function runTransfer(kind: "send" | "receive", scope: Scope) {
      const setName = setlistName(p.view ?? 0);
      const where = scope === "all" ? "all setlists" : scope === "setlist" ? `setlist "${setName}"` : `${p.selected.size} selected preset(s) in "${setName}"`;
      if (kind === "send") {
        const pending = pendingInScope(scope);
        if (scope !== "all" && pending.length === 0) { toast(`Nothing to send: no changed presets in ${where}.`); return; }
        if (pending.length && !window.confirm(`Save to the pedal's memory (overwrites what is stored):\n\n${pending.join("\n")}\n\nContinue?`)) return;
      } else {
        const s = st();
        const curInScope = !!s && !!s.dirty && (scope === "all" || (s.setlist === p.view && (scope === "setlist" || p.selected.has(s.slot))));
        if (!window.confirm(`Read ${where} again from the pedal?` + (curInScope ? `\n\nThe unsaved changes of "${s!.presetName}" (${s!.slotLabel}) will be discarded.` : ""))) return;
      }
      p.transferRunning = true;
      render();
      try {
        const body = { scope, setlist: p.view ?? 0, slots: [...p.selected].sort((a, b) => a - b) };
        const res = await post(`/api/${kind}`, body);
        const r = (await res.json()) as { done: number; failed?: string[]; nothing?: boolean };
        if (r.nothing) toast("Nothing to send: no changed presets.");
        else if (r.failed && r.failed.length) toast(`Finished with errors (${r.done} ok): ${r.failed.join("; ")}`);
        else toast(kind === "send" ? `Saved and verified: ${r.done} preset(s).` : `Received from the pedal: ${r.done} preset(s).`, true);
        void fetchSetlist();
      } catch (e) {
        toast(`${kind === "send" ? "SEND" : "RECEIVE"} failed: ${(e as Error).message}`);
      } finally {
        p.transferRunning = false;
        render();
      }
    }

    const canImport = () => !!st()?.connected && !p.filesRunning && !p.transferRunning && p.view !== null;

    async function withFiles(fn: () => Promise<void>) {
      p.filesRunning = true;
      render();
      try { await fn(); } catch (e) { toast((e as Error).message); } finally { p.filesRunning = false; render(); }
    }

    function exportFiles(kind: "preset" | "setlist" | "bundle") {
      void withFiles(async () => {
        const set = p.view;
        const names: string[] = [];
        if (kind === "preset") {
          for (const slot of [...p.selected].sort((a, b) => a - b)) names.push(await download(`/api/export?kind=preset&setlist=${set}&slot=${slot}`));
        } else {
          if (kind === "bundle") toast("Reading the remaining setlists from the pedal…", true);
          names.push(await download(`/api/export?kind=${kind}&setlist=${set}`));
        }
        toast(`Exported to Downloads: ${names.join(", ")}`, true);
      });
    }

    // importFiles sends the files to the editor's copy; target is the slot
    // where preset files go (consecutively). Setlists go to the viewed setlist.
    function importFiles(files: File[], target: number | null | undefined) {
      if (!files.length) return;
      void withFiles(async () => {
        const kinds = await Promise.all(files.map(fileKind));
        const bad = files.filter((_, i) => !kinds[i]);
        if (bad.length) throw new Error(`Not a POD HD500 Edit file: ${bad.map((f) => f.name).join(", ")}`);
        const kind = kinds[0];
        if (files.length > 1 && kinds.some((k) => k !== "preset")) throw new Error("Import one setlist (.h5s) or bundle (.h5b) at a time; multiple files only for presets (.h5e).");
        const setName = setlistName(p.view ?? 0);
        if (kind === "preset") {
          if (target === null || target === undefined) throw new Error("Select the destination slot before importing.");
          if (target + files.length > 64) throw new Error(`Doesn't fit: ${files.length} preset(s) starting at ${slotLabel(target)} go past 16D.`);
          const range = files.length > 1 ? `${slotLabel(target)}–${slotLabel(target + files.length - 1)}` : slotLabel(target);
          const over: string[] = [];
          for (let k = 0; k < files.length; k++) if (p.names[target + k]) over.push(`${slotLabel(target + k)} ${p.names[target + k]}`);
          if (over.length > 1 && !window.confirm(`Import ${files.length} presets into ${range} of "${setName}", replacing in the editor:\n\n${over.join("\n")}\n\nNothing is saved to the pedal until SEND. Continue?`)) return;
        } else if (kind === "setlist") {
          if (!window.confirm(`Import the setlist "${files[0].name}", replacing the 64 presets and the name of "${setName}" in the editor?\n\nNothing is saved to the pedal until SEND (RECEIVE undoes it).`)) return;
        } else if (!window.confirm(`Import the bundle "${files[0].name}", replacing the 8 setlists (512 presets and names) in the editor?\n\nNothing is saved to the pedal until SEND › All (RECEIVE › All undoes it).`)) return;

        if (kind !== "preset" && p.edited.some(Boolean) && !window.confirm("There are pending changes (*) that the file will replace. Continue?")) return;

        const fd = new FormData();
        fd.append("setlist", String(p.view));
        fd.append("slot", String(target ?? 0));
        for (const f of files) fd.append("file", f, f.name);
        if (kind === "bundle") toast("Reading the remaining setlists from the pedal…", true);
        const res = await fetch("/api/import", { method: "POST", body: fd });
        if (!res.ok) throw await apiError(res);
        const r = (await res.json()) as { presets: number; same?: number; renamed?: number; setNames?: string[]; hd500x?: boolean };
        if (kind === "preset") {
          p.selected = new Set(files.map((_, k) => target! + k));
          p.sel = target!;
        }
        void fetchSetlist();
        let msg = r.presets
          ? `Imported: ${r.presets} preset(s) pending (*). SEND to save to the pedal or RECEIVE to undo.`
          : "Imported: identical to what is on the pedal, nothing to send.";
        if (r.same && r.presets) msg += ` ${r.same} were already the same as on the pedal.`;
        if (r.renamed) msg += kind === "setlist"
          ? ` The setlist is now named "${(r.setNames || [])[0]}" (pending *, until SEND › Set List).`
          : ` ${r.renamed} setlist name(s) pending (*), saved with SEND › All.`;
        if (!r.presets && r.renamed) msg = msg.replace("Imported: identical to what is on the pedal, nothing to send.", "Imported: presets identical to the pedal's.");
        if (r.hd500x) msg += " (HD500X file.)";
        toast(msg, true);
      });
    }

    function pickFiles(accept: string) {
      const input = fileInput.current;
      if (!input) return;
      input.accept = accept;
      input.value = "";
      input.click();
    }

    function backup() {
      if (!window.confirm("Back up the whole pedal?\n\nThe editor reads all 512 presets and the setlist names again (about 40 s) and saves an .h5b file in Documents › POD HD500 Edit X › Backups.\n\nPending changes (*) are not included in the backup.")) return;
      void withFiles(async () => {
        const res = await post("/api/backup");
        const r = (await res.json()) as { path: string; presets: number };
        const file = r.path.split("/").pop();
        toast(`Backup saved: ${file} (${r.presets} presets).`, true, {
          label: "Show in Finder",
          run: () => { void post("/api/reveal", { path: r.path }).catch(() => {}); },
        });
      });
    }

    async function renameSetlist(target: number, raw: string) {
      const name = toPedalChars(raw).trim().slice(0, MAX_NAME);
      if (!name || name === setlistName(target)) return;
      try {
        await post("/api/setlist-name", { setlist: target, name });
        await loadSetlistNames();
        const x = p.setlists[target];
        toast(x && x.edited
          ? `Setlist renamed to "${name}" in the editor (*). Use SEND › Set List to save it to the pedal.`
          : "Same name as on the pedal: nothing pending.", true);
      } catch (e) {
        toast(`Couldn't rename the setlist: ${(e as Error).message}`);
      }
    }

    // the preset menus' actions (list right-click and header "…"), on the
    // selected slots (one = the single slot for load / rename)
    function presetAction(act: PresetAct, one: number) {
      switch (act) {
        case "load": void loadPreset(one); break;
        case "rename": startRename(one); break;
        case "copy": copySelection(); break;
        case "paste": pasteClipboard(); break;
        case "export": exportFiles("preset"); break;
        case "import": pickFiles(".h5e,.5xe"); break;
        case "send": void runTransfer("send", "selected"); break;
        case "receive": void runTransfer("receive", "selected"); break;
      }
    }

    function headerAction(act: PresetAct, openPanel: () => void) {
      const s = st();
      if (!s || !s.connected) return;
      if (p.view !== s.setlist) showSetlist(s.setlist);
      p.selected = new Set([s.slot]);
      p.sel = s.slot;
      render();
      if (act === "rename") openPanel(); // renaming happens in the list
      presetAction(act, s.slot);
    }

    return {
      p, version: 0, state: null, setlistName, showSetlist, render, select, selectOnly, loadPreset, startRename, finishRename,
      arrange, copySelection, pasteClipboard, pendingInScope, runTransfer, canImport, exportFiles, importFiles, pickFiles,
      backup, renameSetlist, presetAction, headerAction,
      registerFileInput: (el) => { fileInput.current = el; },
      // internal, used by the effect below
      ...({ fetchSetlist, loadSetlistNames } as object),
    } as PresetsApi;
  }, [p, render, toast]);

  // Follow the live state (classic renderPresets).
  useEffect(() => {
    const st = state;
    const prev = prevRef.current;
    prevRef.current = st;
    if (!st || !st.connected) { render(); return; }
    const internal = api as unknown as { fetchSetlist: () => Promise<void>; loadSetlistNames: () => Promise<void> };
    // The pedal doesn't answer setlist/preset reads with the tuner open: wait
    // (closing it bumps libraryRev and everything is fetched then).
    const tunerOn = !!st.tuner?.on;
    if (!p.setlistsLoaded && !tunerOn) void internal.loadSetlistNames();
    // Follow the pedal when it changes setlist (front panel or our own load).
    if (p.view === null || (prev && prev.connected && prev.setlist !== st.setlist)) {
      api.showSetlist(st.setlist);
    } else if (st.libraryRev !== p.rev) {
      void internal.fetchSetlist();
      if (p.setlistsLoaded) void internal.loadSetlistNames(); // renames, imports, SEND/RECEIVE
    }
    p.rev = st.libraryRev;
  }, [state, api, p, render]);

  const value = useMemo(() => ({ ...api, version, state }), [api, version, state]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
