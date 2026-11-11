// Connects the MIDI window (purely visual) to the editor: reads preset.midi and
// meta.midiMessages / midiChannels, writes through POST /api/midi-assign.
// Writes use the shared parameter sender (one request in flight, newest wins);
// the preset is re-read once the changes stop, so a slider is not reset mid-drag.
import { useCallback, useEffect, useRef } from "react";
import { useEditor } from "../store";
import { queueSend } from "./editor/sender";
import { MidiWindow, type MidiChangeField } from "./MidiWindow";

const REFETCH_AFTER_MS = 300;

export function MidiDialog({ onClose }: { onClose: () => void }) {
  const { preset, meta, refetchPreset, toast } = useEditor();
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current); }, []);

  const onChange = useCallback((control: number, patch: Partial<Record<MidiChangeField, number>>) => {
    if (timer.current) clearTimeout(timer.current);
    void queueSend("/api/midi-assign", { control, ...patch }, (msg) => toast(msg)).then(() => {
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => { void refetchPreset(); }, REFETCH_AFTER_MS);
    });
  }, [refetchPreset, toast]);

  if (!preset?.midi) return null;
  return (
    <MidiWindow
      items={preset.midi}
      messages={meta?.midiMessages}
      channels={meta?.midiChannels}
      onChange={onChange}
      onClose={onClose}
    />
  );
}
