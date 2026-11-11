// Header (HX Edit layout, POD HD500 features): presets-panel toggle, preset
// stepper + name + "…" menu, snapshots (a dimmed placeholder: the HD500 has none),
// app name with the connection
// LED, MIDI, tuner, Tempo Sync, tap tempo / BPM.
import { useLayoutEffect, useRef, useState } from "react";
import { post } from "../api";
import { useEditor } from "../store";
import { MenuWrap, useMenu } from "./Menu";
import { Tempo } from "./Tempo";
import { usePresets, type PresetAct } from "../presets";

const CAMERA = "M2.5 4.8h4.6l1.7-2.6h6.4l1.7 2.6h4.6a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1h-19a1 1 0 0 1-1-1v-10a1 1 0 0 1 1-1z";
const ListIcon = () => <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h2M3 12h2M3 18h2M8 6h13M8 12h13M8 18h13" /></svg>;
const GlobeIcon = () => <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3c2.6 2.6 3.8 5.6 3.8 9s-1.2 6.4-3.8 9c-2.6-2.6-3.8-5.6-3.8-9S9.4 5.6 12 3z" /></svg>;
const CameraIcon = () => <svg viewBox="0 0 24 18" aria-hidden="true"><path d={CAMERA} /><circle cx="12" cy="10.8" r="4.9" /></svg>;

export function Header({ panelOpen, onTogglePanel, onOpenPanel, onOpenMidi }: { panelOpen: boolean; onTogglePanel: () => void; onOpenPanel: () => void; onOpenMidi: () => void }) {
  const { state, link, preset, toast } = useEditor();
  const online = !!state?.connected;
  const ready = online && !state?.waitingTuner;
  // The app name sits in the middle of the header only when it fits between
  // the snapshot box and the right-hand buttons; otherwise it takes the free space
  // between them ("free"), and with too little room only the LED stays ("led").
  const headerRef = useRef<HTMLElement>(null);
  const [brandMode, setBrandMode] = useState<"center" | "free" | "led">("center");
  useLayoutEffect(() => {
    const hdr = headerRef.current;
    if (!hdr) return;
    const measure = () => {
      const left = hdr.querySelector(".hxh-brand")?.previousElementSibling?.getBoundingClientRect();
      const right = hdr.querySelector(".hxh-right")?.getBoundingClientRect();
      const name = hdr.querySelector(".brand-name")?.parentElement;
      if (!left || !right || !name) return;
      const box = hdr.getBoundingClientRect();
      const full = 150; // the brand's width with its name (LED + "POD HD500" + "Edit")
      const gap = 12;
      const mid = box.left + box.width / 2;
      if (mid - full / 2 >= left.right + gap && mid + full / 2 <= right.left - gap) setBrandMode("center");
      else if (right.left - left.right >= full + 2 * gap) setBrandMode("free");
      else setBrandMode("led");
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(hdr);
    return () => ro.disconnect();
  }, []);
  const presetMenu = useMenu();
  // "…" menu of the loaded preset: acts on it through the presets list (classic initHeaderPresetMenu)
  const pr = usePresets();
  const filesBusy = pr.p.transferRunning || pr.p.filesRunning;
  const enable: Record<PresetAct, boolean> = { load: true, rename: true, copy: true, paste: !!pr.p.clipboard, import: !filesBusy && pr.canImport(), export: !filesBusy, send: !filesBusy, receive: !filesBusy };
  const act = (a: PresetAct) => { presetMenu.setOpen(false); pr.headerAction(a, onOpenPanel); };
  const item = (a: PresetAct, label: string) => <button type="button" role="menuitem" disabled={!ready || !enable[a]} onClick={() => act(a)}>{label}</button>;
  const tsMenu = useMenu();

  // connection LED: green connected; red offline, stalled (the pedal stopped
  // taking data) or the editor server unreachable
  const stalled = online && !!state?.stalled;
  const led = link === "down" ? "offline" : stalled ? "stalled" : online ? "online" : "offline";
  const ledTip = link === "down" ? "Editor disconnected (is the server running?)"
    : stalled ? "The pedal stopped accepting data.\nPower it off and on, run: sudo ./bin/l6-rebind release, then restart the editor"
    : online ? "Connected"
    : `Offline${state?.error ? `: ${state.error}` : ""}\nTo connect: sudo ./bin/l6-rebind release and restart the editor`;

  async function stepPreset(d: number) {
    if (!state || !online) return;
    const slot = (state.slot + d + 64) % 64; // wraps inside the setlist, like the pedal
    if (state.dirty && !window.confirm(`Loading another preset discards the unsaved changes of "${state.presetName}" (${state.slotLabel}). Continue?`)) return;
    try { await post("/api/select", { setlist: state.setlist, slot }); } catch (e) { toast(`Couldn't load the preset: ${(e as Error).message}`); }
  }

  async function setTempoSync(value: number) {
    tsMenu.setOpen(false);
    if (!state || value === state.tempoSync) return;
    try { await post("/api/tempo-sync", { value }); } catch (e) { toast(`Couldn't change Tempo Sync: ${(e as Error).message}`); }
  }

  const tunerOn = online && !!state?.tuner?.on;
  const global = online && state?.tempoSync === 1;

  return (
    <header className="hxh" aria-label="Preset" ref={headerRef}>
      <button type="button" className="hxh-lptoggle" aria-expanded={panelOpen} title={panelOpen ? "Hide the presets panel" : "Show the presets panel"} onClick={onTogglePanel}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><path className="ar" d="M8.5 7.5 4 12l4.5 4.5" /><path d="M11 7h9M11 12h9M11 17h9" /></svg>
      </button>

      <div className="hxh-box hxh-preset">
        <div className="hxh-step">
          <button type="button" disabled={!online || pr.p.transferRunning} title="Previous preset" aria-label="Previous preset" onClick={() => stepPreset(-1)}><svg viewBox="0 0 12 8" aria-hidden="true"><path d="M2 6.5 6 2.5l4 4" /></svg></button>
          <button type="button" disabled={!online || pr.p.transferRunning} title="Next preset" aria-label="Next preset" onClick={() => stepPreset(1)}><svg viewBox="0 0 12 8" aria-hidden="true"><path d="M2 1.5l4 4 4-4" /></svg></button>
        </div>
        <span className={`hxh-name${online && state?.dirty ? " dirty" : ""}`} title={online ? `${state?.slotLabel ?? ""} · Setlist ${(state?.setlist ?? 0) + 1}${state?.setlistName ? " · " + state.setlistName : ""}` : ""}>
          {online ? state?.presetName || "—" : "No pedal"}
        </span>
        <MenuWrap menu={presetMenu} className="hxh-pmwrap">
          <button type="button" className="hxh-more" disabled={!ready} aria-haspopup="menu" aria-expanded={presetMenu.open} title="Preset actions" aria-label="Preset actions" onClick={presetMenu.toggle}>
            <svg viewBox="0 0 24 24" aria-hidden="true"><circle className="ring" cx="12" cy="12" r="9" /><circle cx="7.6" cy="12" r="1.25" /><circle cx="12" cy="12" r="1.25" /><circle cx="16.4" cy="12" r="1.25" /></svg>
          </button>
          {presetMenu.open && (
            <div className="menu hxh-pmenu" role="menu">
              {item("rename", "Rename")}
              {item("copy", "Copy")}
              {item("paste", "Paste")}
              <hr />
              {item("import", "Import...")}
              {item("export", "Export...")}
              <hr />
              {item("send", "Save to Pedal")}
              {item("receive", "Discard Changes")}
            </div>
          )}
        </MenuWrap>
      </div>

      {/* Snapshots: the HD500 has none (planned for later): shown dimmed and not clickable */}
      <div className="hxh-snapwrap">
        <button type="button" className="hxh-box hxh-snap" disabled title="Snapshots (not available yet)">
          <span className="snap-num"><CameraIcon /><b>1</b></span><span>SNAPSHOT 1</span>
        </button>
      </div>

      <div className={`brand hxh-brand brand-${brandMode}`}>
        <span className="brand-mark" data-state={led} role="img" aria-label={led === "online" ? "Connected" : led === "stalled" ? "Not responding" : "Offline"} title={ledTip} />
        <span className="brand-name">POD HD500</span>
        <span className="brand-sub">Edit</span>
      </div>

      <div className="hxh-right">
        <button type="button" id="midi-btn" className="hxh-box hxh-icon" disabled={!online || !preset?.midi} title="MIDI Assign" aria-label="MIDI" aria-haspopup="dialog" onClick={onOpenMidi}>
          <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /><circle className="f" cx="7.6" cy="12" r="1.1" /><circle className="f" cx="16.4" cy="12" r="1.1" /><circle className="f" cx="9" cy="8.3" r="1.1" /><circle className="f" cx="15" cy="8.3" r="1.1" /><circle className="f" cx="12" cy="7" r="1.1" /><path d="M10.5 18.5h3" /></svg>
        </button>
        <button type="button" id="tuner-btn" className={`hxh-box hxh-icon hxh-tuner${tunerOn ? " on" : ""}`} disabled={!online} title="Tuner" aria-label="Tuner" aria-pressed={tunerOn}
          onClick={() => post("/api/tuner", { on: !tunerOn }).catch((e: Error) => toast(`Tuner: ${e.message}`))}>
          <svg viewBox="0 0 24 24" aria-hidden="true"><g transform="rotate(45 12 12)"><path className="t" d="M8.5 2.5v7.5a3.5 3.5 0 0 0 7 0V2.5" /><path className="t" d="M12 13.5v7" /><circle cx="12" cy="21" r="2.5" /></g></svg>
        </button>
        {/* Tempo Sync (pedal-wide): Per Preset or Global. Per Snapshot exists only on the HX: shown disabled. */}
        <MenuWrap menu={tsMenu} className="hxh-tswrap">
          <button type="button" id="tsync-btn" className="hxh-box hxh-icon" disabled={!online} aria-haspopup="menu" aria-expanded={tsMenu.open} aria-label="Tempo Sync"
            title={`Tempo Sync: ${global ? "Global (one tempo for every preset)" : "Per Preset"}`} onClick={tsMenu.toggle}>
            <span id="tsync-icon">{global ? <GlobeIcon /> : <ListIcon />}</span>
          </button>
          {tsMenu.open && (
            <div className="menu hxh-tsmenu" role="menu">
              <button type="button" role="menuitem" disabled title="Not available on the POD HD500"><CameraIcon />Per Snapshot</button>
              <button type="button" role="menuitem" className={online && state?.tempoSync === 0 ? "cur" : ""} title="Each preset uses its own tempo" onClick={() => setTempoSync(0)}><ListIcon />Per Preset</button>
              <button type="button" role="menuitem" className={global ? "cur" : ""} title="One tempo for every preset (the pedal's Tempo Sync: Global)" onClick={() => setTempoSync(1)}><GlobeIcon />Global</button>
            </div>
          )}
        </MenuWrap>
        <Tempo />
      </div>
    </header>
  );
}
