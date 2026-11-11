// Root layout (HX Edit): presets panel on the left; header, signal flow and
// block editor on the right. The MIDI window opens from the header.
import { useEffect, useState } from "react";
import { post } from "./api";
import { EditorProvider, useEditor } from "./store";
import { Header } from "./components/Header";
import { SignalFlow } from "./components/SignalFlow";
import { Tuner } from "./components/Tuner";
import { BlockEditor } from "./components/editor/BlockEditor";
import { MidiDialog } from "./components/MidiDialog";
import { PresetsPanel } from "./components/presets/PresetsPanel";
import { PresetsProvider } from "./presets";

function DSPAlert() {
  const { state } = useEditor();
  const [shown, setShown] = useState(false);
  const [last, setLast] = useState<number | undefined>(undefined);
  // a new state.dspAlert = the pedal just reported "DSP limit reached"
  useEffect(() => {
    const a = state?.dspAlert;
    if (a === last) return;
    if (last !== undefined && a) setShown(true);
    setLast(a);
  }, [state?.dspAlert, last]);
  useEffect(() => {
    if (!shown) return;
    const t = setTimeout(() => setShown(false), 4500);
    return () => clearTimeout(t);
  }, [shown]);
  if (!shown) return null;
  return (
    <div className="dsp-alert" role="alert" onClick={() => setShown(false)}>
      <b>DSP LIMIT REACHED</b><span>Reduce usage to free up additional DSP resources.</span>
    </div>
  );
}

function Toasts() {
  const { toasts, dismissToast } = useEditor();
  const t = toasts[toasts.length - 1];
  if (!t) return null;
  return (
    <div className={`toast${t.ok ? " ok" : ""}`} role="status">
      <span>{t.msg}</span>
      {t.action && <button type="button" className="toast-action" onClick={() => { dismissToast(t.id); t.action!.run(); }}>{t.action.label}</button>}
    </div>
  );
}

function Layout() {
  const { state } = useEditor();
  const [panelOpen, setPanelOpen] = useState(() => {
    try { return localStorage.getItem("lpCollapsed") !== "1"; } catch { return true; }
  });
  const setPanel = (open: boolean) => {
    try { localStorage.setItem("lpCollapsed", open ? "0" : "1"); } catch { /* private window */ }
    setPanelOpen(open);
  };
  const togglePanel = () => setPanel(!panelOpen);
  const [midiOpen, setMidiOpen] = useState(false);
  // Escape closes the tuner (as in the classic UI)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && (e.target as HTMLElement).id !== "tuner-ref" && state?.tuner?.on) void post("/api/tuner", { on: false }).catch(() => {});
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [state?.tuner?.on]);

  return (
    <main className={`layout${panelOpen ? "" : " lp-collapsed"}`}>
      <Header panelOpen={panelOpen} onTogglePanel={togglePanel} onOpenPanel={() => setPanel(true)} onOpenMidi={() => setMidiOpen(true)} />
      <section className="panel flow-view" aria-label="Signal Flow">
        <Tuner />
        <SignalFlow />
        <DSPAlert />
      </section>
      <PresetsPanel />
      <BlockEditor />
      {midiOpen && <MidiDialog onClose={() => setMidiOpen(false)} />}
      <Toasts />
    </main>
  );
}

export function App() {
  return <EditorProvider><PresetsProvider><Layout /></PresetsProvider></EditorProvider>;
}
