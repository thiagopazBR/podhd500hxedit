// Left panel (Helix style): setlists, the 64 presets of the viewed setlist
// (01A–16D) and the pedal's SEND / RECEIVE. Ported from the classic UI
// (index.html section.presets.lp + app.js); same markup and classes, so the
// classic stylesheet applies. State and actions: ../../presets.tsx.
import { useEffect, useLayoutEffect, useRef, useState, type DragEvent, type KeyboardEvent, type MouseEvent } from "react";
import { MenuWrap, useMenu } from "../Menu";
import { useEditor } from "../../store";
import { FILE_RE, MAX_NAME, slotLabel, toPedalChars, usePresets, type PresetAct, type Scope } from "../../presets";

// Folder icons in the HX style (own drawing): closed for the other setlists,
// open (back panel + slanted front flap) for the current one.
const Folder = () => <path className="fb" d="M3.5 18.5V5.5h5l1.6 1.8h10.4v11.2z" />;
const FolderOpen = () => <><path className="fb" d="M4.5 18.5V5.5h5l1.6 1.8h7.4v11.2z" /><path className="ff" d="M2.5 10.5h15.2l2.3 8H4.8z" /></>;
const MODE_LABEL = { move: "move here", copy: "copy here", swap: "swap" } as const;
const hasFiles = (e: DragEvent) => !!e.dataTransfer && [...e.dataTransfer.types].includes("Files");

function RenameInput({ slot, initial, onDone }: { slot: number; initial: string; onDone: (value: string, commit: boolean) => void }) {
  const [v, setV] = useState(initial);
  const done = useRef(false);
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => { ref.current?.focus(); ref.current?.select(); }, []);
  const finish = (commit: boolean) => { if (done.current) return; done.current = true; onDone(v, commit); };
  return (
    <input ref={ref} className="rename" maxLength={MAX_NAME} value={v} spellCheck={false} aria-label={`New name for ${slotLabel(slot)}`}
      onChange={(e) => setV(toPedalChars(e.target.value).slice(0, MAX_NAME))}
      onKeyDown={(e) => {
        e.stopPropagation(); // don't trigger list shortcuts / tap tempo
        if (e.key === "Enter") { e.preventDefault(); finish(true); } else if (e.key === "Escape") { e.preventDefault(); finish(false); }
      }}
      onBlur={() => finish(true)}
      onClick={(e) => e.stopPropagation()}
      onDoubleClick={(e) => e.stopPropagation()} />
  );
}

export function PresetsPanel() {
  const pr = usePresets();
  const { toast } = useEditor();
  const { p, state } = pr;
  const online = !!state?.connected;
  const waiting = !!state?.waitingTuner;
  const off = !online || p.transferRunning || p.filesRunning;
  const noView = p.view === null;
  const nsel = p.selected.size;

  const sendMenu = useMenu(), receiveMenu = useMenu(), exportMenu = useMenu(), listMenu = useMenu();
  const panelRef = useRef<HTMLElement>(null);
  const slotsRef = useRef<HTMLOListElement>(null);
  const rows = useRef<(HTMLLIElement | null)[]>([]);
  const ctxRef = useRef<HTMLDivElement>(null);
  const [ctx, setCtx] = useState<{ x: number; y: number } | null>(null);
  const [drag, setDrag] = useState<number | null>(null); // slot being dragged
  const [drop, setDrop] = useState<{ slot: number; label: string } | null>(null);
  const [fileOver, setFileOver] = useState(false);
  const [setRename, setSetRename] = useState<{ set: number; value: string } | null>(null);
  const setRenameInput = useRef<HTMLInputElement>(null);
  const editBtn = useRef<HTMLButtonElement>(null);

  // a running transfer / files job closes the menus
  useEffect(() => { if (off) { sendMenu.setOpen(false); receiveMenu.setOpen(false); exportMenu.setOpen(false); } }, [off]); // eslint-disable-line react-hooks/exhaustive-deps

  // the loaded preset scrolls into view when it changes
  const cur = state ? `${state.setlist}:${state.slot}` : "";
  useEffect(() => {
    if (!state || cur === p.lastCurrent) return;
    if (state.setlist === p.view) rows.current[state.slot]?.scrollIntoView({ block: "nearest" });
    p.lastCurrent = cur;
  }, [cur, p.view]); // eslint-disable-line react-hooks/exhaustive-deps

  // context menu: placed inside the panel, closes on an outside click / Escape
  useLayoutEffect(() => {
    if (!ctx || !ctxRef.current || !panelRef.current) return;
    const m = ctxRef.current, r = panelRef.current.getBoundingClientRect();
    const x = Math.min(ctx.x - r.left, r.width - m.offsetWidth - 6), y = Math.min(ctx.y - r.top, r.height - m.offsetHeight - 6);
    m.style.left = `${Math.max(6, x)}px`;
    m.style.top = `${Math.max(6, y)}px`;
    m.querySelector<HTMLButtonElement>("button:not(:disabled)")?.focus({ preventScroll: true });
  }, [ctx]);
  useEffect(() => {
    if (!ctx) return;
    const onDown = (e: globalThis.MouseEvent) => { if (!ctxRef.current?.contains(e.target as Node)) setCtx(null); };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [ctx]);

  // dropping a file outside the list must not navigate away from the editor
  useEffect(() => {
    const prevent = (e: globalThis.DragEvent) => { if (e.dataTransfer && [...e.dataTransfer.types].includes("Files")) e.preventDefault(); };
    window.addEventListener("dragover", prevent);
    window.addEventListener("drop", prevent);
    return () => { window.removeEventListener("dragover", prevent); window.removeEventListener("drop", prevent); };
  }, []);

  const closeAll = () => { sendMenu.setOpen(false); receiveMenu.setOpen(false); exportMenu.setOpen(false); listMenu.setOpen(false); setCtx(null); };

  // ---- setlist picker
  const viewN = waiting ? null : p.view;
  const curSet = viewN === null ? null : p.setlists[viewN];
  const pickerName = viewN === null ? "—" : `${pr.setlistName(viewN)}${curSet?.edited ? " *" : ""}`;

  function startSetRename() {
    if (setRename) { void commitSetRename(); return; }
    if (p.view === null) return;
    setSetRename({ set: p.view, value: pr.setlistName(p.view) });
    setTimeout(() => { setRenameInput.current?.focus(); setRenameInput.current?.select(); }, 0);
  }
  async function commitSetRename() {
    const r = setRename;
    if (!r) return;
    setSetRename(null);
    await pr.renameSetlist(r.set, r.value);
  }

  // ---- slots
  const transferItem = (kind: "send" | "receive", scope: Scope, label: string, menu: ReturnType<typeof useMenu>) => (
    <button type="button" role="menuitem" disabled={off || (scope === "selected" && nsel === 0) || (scope !== "all" && noView)}
      onClick={() => { menu.setOpen(false); void pr.runTransfer(kind, scope); }}>{label}</button>
  );
  const pendingAll = pr.pendingInScope("all").length;

  function onSlotsKey(e: KeyboardEvent<HTMLOListElement>) {
    if ((e.target as HTMLElement).closest("input")) return; // keys typed in the rename field
    if (e.metaKey || e.ctrlKey) {
      const key = e.key.toLowerCase();
      if (key === "c" && nsel) { e.preventDefault(); pr.copySelection(); } else if (key === "v" && p.clipboard) { e.preventDefault(); pr.pasteClipboard(); }
      return;
    }
    if (e.key === "F2" && nsel === 1) {
      e.preventDefault();
      pr.startRename([...p.selected][0]);
    } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const from = p.sel ?? (state && state.setlist === p.view ? state.slot : -1);
      const to = Math.max(0, Math.min(63, from + (e.key === "ArrowDown" ? 1 : -1)));
      pr.selectOnly(to);
      rows.current[to]?.scrollIntoView({ block: "nearest" });
    } else if (e.key === "Enter" && p.sel !== null) {
      e.preventDefault();
      void pr.loadPreset(p.sel);
    }
  }

  function onContext(e: MouseEvent, i: number) {
    if (!online || p.renaming !== null) return;
    e.preventDefault();
    if (!p.selected.has(i)) pr.selectOnly(i); // right-click outside the selection selects that row
    closeAll();
    setCtx({ x: e.clientX, y: e.clientY });
  }
  const one = nsel === 1;
  const busy = p.transferRunning || p.filesRunning;
  const ctxEnable: Record<PresetAct, boolean> = { load: one, rename: one, copy: true, paste: !!p.clipboard, export: !busy, import: !busy && pr.canImport(), send: !busy, receive: !busy };
  const ctxItem = (act: PresetAct, label: string, title?: string) => (
    <button type="button" role="menuitem" title={title} disabled={!ctxEnable[act]} onClick={() => { setCtx(null); pr.presetAction(act, [...p.selected][0]); }}>{label}</button>
  );

  // drag & drop: move (plain), copy (Option), swap (Shift); files from the Finder import
  const dropMode = (e: DragEvent) => (e.altKey ? "copy" : e.shiftKey ? "swap" : "move") as keyof typeof MODE_LABEL;
  function onDragOver(e: DragEvent<HTMLOListElement>) {
    const li = (e.target as HTMLElement).closest<HTMLLIElement>(".slot");
    if (drag !== null) {
      if (!li) return;
      e.preventDefault();
      const mode = dropMode(e);
      e.dataTransfer.dropEffect = mode === "copy" ? "copy" : "move";
      const slot = Number(li.dataset.slot);
      const want = slot !== drag ? { slot, label: MODE_LABEL[mode] } : null;
      if (want?.slot !== drop?.slot || want?.label !== drop?.label) setDrop(want);
      return;
    }
    if (!hasFiles(e) || !pr.canImport()) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
    setFileOver(true);
    if (li && drop?.slot !== Number(li.dataset.slot)) setDrop({ slot: Number(li.dataset.slot), label: "import here" });
  }
  function onDrop(e: DragEvent<HTMLOListElement>) {
    const li = (e.target as HTMLElement).closest<HTMLLIElement>(".slot");
    if (drag !== null) {
      e.preventDefault();
      const from = drag;
      setDrop(null);
      if (!li || p.view === null) return;
      const to = Number(li.dataset.slot);
      if (to === from) return;
      void pr.arrange(dropMode(e), [{ setlist: p.view, slot: from }], { setlist: p.view, slot: to });
      return;
    }
    if (!hasFiles(e)) return;
    e.preventDefault();
    setFileOver(false);
    setDrop(null);
    if (!pr.canImport()) return;
    const files = [...e.dataTransfer.files].filter((f) => FILE_RE.test(f.name));
    if (!files.length) { toast("Drag .h5e, .h5s or .h5b files."); return; }
    pr.importFiles(files, li ? Number(li.dataset.slot) : (nsel ? Math.min(...p.selected) : p.sel));
  }

  const progress = state?.loading
    ? { w: `${(100 * state.loading.done) / state.loading.total}%`, text: `${state.loading.label || "Receiving presets"} ${state.loading.done}/${state.loading.total}` }
    : waiting ? { w: "0", text: "Close the tuner to load presets" } : null;

  return (
    <section className="panel presets lp" id="presets-panel" aria-label="Presets" ref={panelRef}>
      <div className="lp-tabs">
        <button className="lp-tab" type="button" aria-current="page">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" /></svg>
          Presets
        </button>
        {/* pedal transfer, where HX Edit has its IRs / Favorites tabs */}
        <MenuWrap menu={sendMenu} className="lp-tabwrap">
          <button type="button" className="lp-tabbtn lp-send" disabled={off || noView} aria-haspopup="menu" aria-expanded={sendMenu.open}
            title={pendingAll ? `${pendingAll} unsaved change${pendingAll > 1 ? "s" : ""}: save to the pedal's memory` : "Save changes to the pedal's memory"}
            onClick={(e) => { e.stopPropagation(); setCtx(null); sendMenu.toggle(); }}>
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15V4M7.5 8.5 12 4l4.5 4.5M5 14v5h14v-5" /></svg>
            Send
          </button>
          {sendMenu.open && (
            <div className="menu" role="menu">
              <span className="menu-title">Send</span>
              {transferItem("send", "selected", "Selected", sendMenu)}
              {transferItem("send", "setlist", "Set List", sendMenu)}
              {transferItem("send", "all", "All", sendMenu)}
            </div>
          )}
        </MenuWrap>
        <MenuWrap menu={receiveMenu} className="lp-tabwrap">
          <button type="button" className="lp-tabbtn" disabled={off || noView} aria-haspopup="menu" aria-expanded={receiveMenu.open}
            title="Read again from the pedal (discards changes)" onClick={(e) => { e.stopPropagation(); setCtx(null); receiveMenu.toggle(); }}>
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 4v11M7.5 10.5 12 15l4.5-4.5M5 14v5h14v-5" /></svg>Receive
          </button>
          {receiveMenu.open && (
            <div className="menu" role="menu">
              <span className="menu-title">Receive</span>
              {transferItem("receive", "selected", "Selected", receiveMenu)}
              {transferItem("receive", "setlist", "Set List", receiveMenu)}
              {transferItem("receive", "all", "All", receiveMenu)}
            </div>
          )}
        </MenuWrap>
      </div>

      <div className="lp-head">
        <h2 className="lp-title">Setlists</h2>
        <div className="lp-actions">
          <button type="button" ref={editBtn} disabled={off || noView} title="Rename the setlist (pending * until SEND › Set List)" onClick={startSetRename}>{setRename ? "OK" : "Rename"}</button>
          <button type="button" disabled={off || noView} title="Import an .h5s into this setlist, or an .h5b (all setlists). Pending (*) until SEND." onClick={() => pr.pickFiles(".h5s,.h5b,.5xs,.5xb")}>Import</button>
          <MenuWrap menu={exportMenu}>
            <button type="button" disabled={off || noView} aria-haspopup="menu" aria-expanded={exportMenu.open} title="Export in POD HD500 Edit format" onClick={(e) => { e.stopPropagation(); setCtx(null); exportMenu.toggle(); }}>Export</button>
            {exportMenu.open && (
              <div className="menu" role="menu">
                <button type="button" role="menuitem" onClick={() => { exportMenu.setOpen(false); pr.exportFiles("setlist"); }}>This setlist <span>.h5s</span></button>
                <button type="button" role="menuitem" onClick={() => { exportMenu.setOpen(false); pr.exportFiles("bundle"); }}>All setlists <span>.h5b</span></button>
              </div>
            )}
          </MenuWrap>
        </div>
      </div>
      <MenuWrap menu={listMenu} className="lp-setlist">
        {!setRename && (
          <button type="button" className={`lp-picker${curSet?.edited ? " dirty" : ""}`} disabled={!online} aria-haspopup="listbox" aria-expanded={listMenu.open}
            title={curSet?.edited ? `Renamed in the editor; on the pedal: "${curSet.stored}"` : ""}
            onClick={(e) => { e.stopPropagation(); setCtx(null); listMenu.toggle(); }}>
            <svg className="lp-folder" viewBox="0 0 24 24" aria-hidden="true"><path className="fb" d="M4.5 18.5V5.5h5l1.6 1.8h7.4v11.2z" /><path className="ff" d="M2.5 10.5h15.2l2.3 8H4.8z" /></svg>
            <span className="lp-picker-name">{pickerName}</span>
            <span className="lp-picker-count">{viewN === null ? "" : `${viewN + 1}/8`}</span>
            <svg className="lp-chevron" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
          </button>
        )}
        {setRename && (
          <input ref={setRenameInput} className="setlist-rename" type="text" maxLength={16} spellCheck={false} autoComplete="off" aria-label="New setlist name"
            value={setRename.value}
            onChange={(e) => setSetRename({ ...setRename, value: toPedalChars(e.target.value).slice(0, MAX_NAME) })}
            onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); void commitSetRename(); } else if (e.key === "Escape") { e.preventDefault(); setSetRename(null); } }}
            // clicking the OK button blurs the input first: let the click commit
            onBlur={(e) => { if (e.relatedTarget !== editBtn.current) void commitSetRename(); }} />
        )}
        {listMenu.open && (
          <ul className="lp-setlists" role="listbox" aria-label="Pedal setlists"
            onKeyDown={(e) => {
              const items = [...e.currentTarget.querySelectorAll<HTMLButtonElement>(".lp-setlist-item")];
              const i = items.indexOf(document.activeElement as HTMLButtonElement);
              if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); items[Math.max(0, Math.min(items.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)))]?.focus(); }
            }}>
            {p.setlists.map((s) => (
              <li key={s.index} role="option" aria-selected={s.index === viewN}>
                <button type="button" className={`lp-setlist-item${s.index === viewN ? " cur" : ""}${s.edited ? " dirty" : ""}`}
                  ref={(el) => { if (el && s.index === viewN && document.activeElement?.closest(".lp-setlists") == null) el.focus({ preventScroll: true }); }}
                  onClick={() => { listMenu.setOpen(false); if (s.index !== p.view) pr.showSetlist(s.index); }}>
                  <svg viewBox="0 0 24 24" aria-hidden="true">{s.index === viewN ? <FolderOpen /> : <Folder />}</svg>
                  <span className="nm">{`${pr.setlistName(s.index)}${s.edited ? " *" : ""}`}</span>
                  <span className="st">{s.edited ? `pedal: ${s.stored}` : ""}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </MenuWrap>

      <div className="lp-head lp-head-presets">
        <h2 className="lp-title">Presets</h2>
        <div className="lp-actions">
          <button type="button" disabled={off || noView || !nsel} title="Copy the selected presets (Cmd+C)" onClick={pr.copySelection}>Copy</button>
          <button type="button" disabled={off || noView || !nsel || !p.clipboard} title="Paste starting at the selected slot (Cmd+V)" onClick={pr.pasteClipboard}>Paste</button>
          <button type="button" disabled={off || noView || (!nsel && p.sel === null)} title="Import .h5e starting at the selected slot (you can also drag files from the Finder onto the list). Pending (*) until SEND." onClick={() => pr.pickFiles(".h5e,.5xe")}>Import</button>
          <button type="button" disabled={off || noView || !nsel} title="Export the selected presets (.h5e)" onClick={() => pr.exportFiles("preset")}>Export</button>
        </div>
      </div>
      {progress && (
        <div className="progress">
          <div className="progress-bar" style={{ width: progress.w }} />
          <span className="progress-text">{progress.text}</span>
        </div>
      )}
      <ol className={`slots${fileOver ? " file-over" : ""}`} ref={slotsRef} tabIndex={0} aria-label="Setlist presets"
        title="Click: select (Shift/Cmd: multiple) · Double-click or Enter: load to pedal · Right-click: actions · F2: rename · Drag: move · Option+drag: copy · Shift+drag: swap"
        onKeyDown={onSlotsKey}
        onDragOver={onDragOver}
        onDragLeave={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node)) { setDrop(null); setFileOver(false); } }}
        onDrop={onDrop}>
        {Array.from({ length: 64 }, (_, i) => {
          const isCur = online && !waiting && state!.setlist === p.view && state!.slot === i;
          const name = isCur && !p.edited[i] ? state!.presetName || "" : p.names[i] || "";
          const dirty = (isCur && !!state!.dirty) || !!p.edited[i];
          const cls = ["slot", !name && "empty", isCur && "current", dirty && "dirty", p.selected.has(i) && "selected", drag === i && "dragging", drop?.slot === i && "drop-target"].filter(Boolean).join(" ");
          return (
            <li key={i} ref={(el) => { rows.current[i] = el; }} className={cls} draggable data-slot={i} data-label={drop?.slot === i ? drop.label : undefined}
              onClick={(e) => pr.select(i, { shift: e.shiftKey, meta: e.metaKey || e.ctrlKey })}
              onDoubleClick={(e) => { if (!(e.target as HTMLElement).closest("input")) void pr.loadPreset(i); }}
              onContextMenu={(e) => onContext(e, i)}
              onDragStart={(e) => {
                if (p.renaming !== null || !online) { e.preventDefault(); return; }
                setDrag(i);
                e.dataTransfer.setData("text/plain", String(i));
                e.dataTransfer.effectAllowed = "copyMove";
              }}
              onDragEnd={() => { setDrag(null); setDrop(null); }}>
              <span className="mark">{isCur ? "▶" : ""}</span>
              <span className="num">{slotLabel(i)}</span>
              <span className="name">
                {p.renaming === i
                  ? <RenameInput slot={i} initial={name} onDone={(v, commit) => { void pr.finishRename(i, v, commit); slotsRef.current?.focus(); }} />
                  : name || (online && !waiting ? "…" : "")}
              </span>
              <span className="edited">{dirty ? "*" : ""}</span>
            </li>
          );
        })}
      </ol>

      <input type="file" multiple hidden accept=".h5e,.h5s,.h5b,.5xe,.5xs,.5xb" ref={pr.registerFileInput}
        onChange={(e) => { const files = [...(e.target.files || [])]; pr.importFiles(files, nsel ? Math.min(...p.selected) : p.sel); }} />
      {ctx && (
        <div className="menu ctx-menu" role="menu" ref={ctxRef}
          onKeyDown={(e) => {
            const items = [...e.currentTarget.querySelectorAll<HTMLButtonElement>("button:not(:disabled)")];
            const i = items.indexOf(document.activeElement as HTMLButtonElement);
            if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); items[(i + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length]?.focus(); }
            else if (e.key === "Escape") { setCtx(null); slotsRef.current?.focus(); }
          }}>
          {ctxItem("copy", "Copy", "⌘C")}
          {ctxItem("paste", "Paste", "⌘V")}
          {ctxItem("rename", "Rename", "F2")}
          <hr />
          {ctxItem("import", "Import...")}
          {ctxItem("export", "Export...")}
          <hr />
          {ctxItem("load", "Load from Pedal", "Enter")}
          {ctxItem("send", "Save to Pedal")}
          {ctxItem("receive", "Discard Changes")}
        </div>
      )}
    </section>
  );
}
