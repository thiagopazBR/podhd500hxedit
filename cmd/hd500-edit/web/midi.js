// POD HD500 Edit — MIDI window (HX Edit's Command Center, page 46 of its
// guide): the MIDI Assign of the loaded preset, i.e. what each footswitch /
// pedal SENDS on MIDI OUT. Left, the controls laid out as on the pedal's
// MIDI ASSIGN screen; right, the selected control's settings. Data from the
// API (preset.midi, meta.midiMessages / midiChannels); writes go to
// /api/midi-assign. The Snapshot menu is only drawn (not on the HD500).
// Loaded after flow.js; uses its helpers (slider, queueSend, esc, $).
"use strict";

const midiUI = { open: false, sel: 0, ddOpen: "" };
// grid of the pedal's MIDI ASSIGN screen: [control, column, row, span]
// (control ids follow the pedal's storage order = this screen read row by row)
const MIDI_GRID = [[0, 0, 0], [1, 1, 0], [2, 2, 0], [3, 3, 0], [4, 4, 0], [5, 5, 0, 2],
  [6, 0, 1], [7, 1, 1], [8, 2, 1], [9, 3, 1], [10, 4, 1], [11, 5, 1], [12, 6, 1]];
const MIDI_SW_SVG = '<svg viewBox="0 0 24 24" aria-hidden="true"><ellipse cx="12" cy="7" rx="6" ry="2.6"/><path d="M6 7v4c0 1.4 2.7 2.6 6 2.6s6-1.2 6-2.6V7M8 13.2v4.2c0 1 1.8 1.9 4 1.9s4-.9 4-1.9v-4.2"/></svg>';
// EXP 1 / EXP 2: a tall treadle, as large as HX Edit's Command Center pedals
const MIDI_PEDAL_SVG = '<svg class="mc-pedal" viewBox="0 0 28 52" aria-hidden="true"><rect x="3" y="2" width="22" height="48" rx="2.5"/><path d="M7 7h14M7 11h14M7 15h14"/><rect x="9" y="40" width="10" height="5" rx="1"/></svg>';
// EXP Toe: a two-position toggle switch (lever, threaded bushing, body, two
// pins); a line runs from its pins down to EXP 1 and EXP 2
const MIDI_TOE_SVG = '<svg class="mc-toe" viewBox="0 0 24 40" aria-hidden="true"><path class="lever" d="M12 17 15.5 3.5"/><rect x="9" y="13.5" width="6" height="7.5" rx=".8"/><path d="M9 16h6M9 18.5h6"/><rect x="5" y="21" width="14" height="10" rx="1.2"/><path d="M9.5 31v8M14.5 31v8"/></svg>';
const MIDI_TAP_SVG = '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="7.5"/><path d="M12 7.5V12l3 2"/></svg>';
const MIDI_LOOP_SVG = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12a7 7 0 0 1 12-4.9M19 12a7 7 0 0 1-12 4.9"/><path d="M17 3.5V7.5h-4M7 20.5v-4h4"/></svg>';

const midiIcon = (a) => (a.pedal ? MIDI_PEDAL_SVG : a.control === 5 ? MIDI_TOE_SVG : a.control === 10 ? MIDI_TAP_SVG : a.control === 4 ? MIDI_LOOP_SVG : MIDI_SW_SVG);

function midiDD(field, opts, cur) {
  const c = opts.find((o) => o.value === cur) || { label: "—" };
  const open = midiUI.ddOpen === field;
  return `<div class="as-dd mc-dd" data-mdd="${field}"><button type="button" class="as-dd-btn" aria-expanded="${open}">
    <span>${esc(c.label)}</span><svg viewBox="0 0 12 8" aria-hidden="true"><path d="M1.5 1.5 6 6l4.5-4.5" /></svg></button>
    <div class="as-dd-list" role="listbox"${open ? "" : " hidden"}>${opts.map((o) =>
      `<button type="button" role="option" class="${o.value === cur ? "cur" : ""}" data-mv="${o.value}">${esc(o.label)}</button>`).join("")}</div></div>`;
}

function renderMidi() {
  const modal = $("midi-modal");
  if (!modal || modal.hidden || flow.dragging) return;
  const list = (flow.view && flow.view.midi) || [];
  const grid = $("mc-grid"), insp = $("mc-insp");
  if (!list.length) { grid.innerHTML = '<span class="mc-empty">No preset loaded.</span>'; insp.innerHTML = ""; return; }
  grid.innerHTML = MIDI_GRID.map(([c, col, row, span]) => {
    const a = list[c];
    const on = a.channel !== 16; // None = sends nothing
    return `<button type="button" class="mc-cell${c === midiUI.sel ? " sel" : ""}${on ? " on" : ""}${span ? " wide" : ""}" data-mc="${c}"
      ${a.pedal ? ' data-pedal="1"' : ""} style="grid-column:${col + 1}${span ? ` / span ${span}` : ""};grid-row:${row + 1}" title="${esc(a.name)}">
      <i class="mc-tri"></i>${midiIcon(a)}<span>${esc(a.name)}</span>${c === 5 ? '<i class="mc-stem"></i><i class="mc-fork"></i>' : ""}</button>`;
  }).join("");
  const a = list[midiUI.sel];
  const chans = (meta && meta.midiChannels) || [];
  const set = (field) => (x) => queueSend("/api/midi-assign", { control: a.control, [field]: Math.round(x * 127) });
  const num = (key, name, field) => slider(`mc.${a.control}.${key}`, name, {
    value: a[field] / 127, steps: 128, fmt: (x) => String(Math.round(x * 127)), send: set(field),
  });
  let rows = "";
  if (!a.pedal) rows += `<div class="prm prm-switch"><span class="prm-name">Message</span>${midiDD("message", (meta && meta.midiMessages) || [], a.message)}</div>`;
  rows += `<div class="prm prm-switch"><span class="prm-name">MIDI Channel</span>${midiDD("channel", chans, a.channel)}</div>`;
  if (a.channel !== 16) {
    if (a.pedal) rows += num("n", "CC #", "number") + num("v", "Min Value", "value") + num("m", "Max Value", "max");
    else if (a.message <= 1) rows += num("n", "CC #", "number") + num("v", "Value", "value");
    else if (a.message === 2) rows += num("n", "Program #", "number");
    else if (a.message === 3) rows += num("n", "Bank #", "number");
    else rows += num("n", "Note", "number") + num("v", "Velocity", "value");
  }
  insp.innerHTML = `<div class="mc-ihead">${midiIcon(a)}<span>${esc(a.name)}</span></div><div class="params">${rows}</div>`;
}

function setMidiModal(open) {
  const m = $("midi-modal");
  m.hidden = !open;
  midiUI.ddOpen = "";
  $("midi-btn").setAttribute("aria-expanded", String(open));
  if (open) renderMidi();
}

function initMidi() {
  $("midi-btn").addEventListener("click", () => setMidiModal(true));
  $("mc-done").addEventListener("click", () => setMidiModal(false));
  const m = $("midi-modal");
  m.addEventListener("pointerdown", (e) => { if (e.target === m) setMidiModal(false); });
  document.addEventListener("keydown", (e) => { if (e.key === "Escape" && !m.hidden) { if (midiUI.ddOpen) { midiUI.ddOpen = ""; renderMidi(); } else setMidiModal(false); } });
  m.addEventListener("click", (e) => {
    const cell = e.target.closest("[data-mc]");
    if (cell) { midiUI.sel = Number(cell.dataset.mc); midiUI.ddOpen = ""; renderMidi(); return; }
    const btn = e.target.closest(".mc-dd .as-dd-btn");
    if (btn) { const f = btn.parentElement.dataset.mdd; midiUI.ddOpen = midiUI.ddOpen === f ? "" : f; renderMidi(); return; }
    const opt = e.target.closest(".mc-dd [data-mv]");
    if (opt) {
      const f = opt.closest(".mc-dd").dataset.mdd, v = Number(opt.dataset.mv);
      midiUI.ddOpen = "";
      const a = flow.view.midi[midiUI.sel];
      if (a[f] !== v) { a[f] = v; queueSend("/api/midi-assign", { control: a.control, [f]: v }).then(() => fetchFlow()); }
      renderMidi();
      return;
    }
    if (midiUI.ddOpen && !e.target.closest(".mc-dd")) { midiUI.ddOpen = ""; renderMidi(); }
  });
  initSliders($("mc-insp"));
}
