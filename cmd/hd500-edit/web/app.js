// POD HD500 Edit — UI logic for the top-left module (SEND/RECEIVE + INPUTS).
// State comes from the Go server: GET /api/meta once, then Server-Sent Events
// on /api/events for every change (from this UI or from the pedal itself).
"use strict";

const $ = (id) => document.getElementById(id);
// The inputs are edited in the Signal Flow (input block, flow.js); no selects here.
const selects = {};
let meta = null;
let state = null;
const busy = new Set(); // selects with a request in flight: don't overwrite them

// toast shows a message; action = { label, run } adds a button to it.
function toast(msg, ok = false, action = null) {
  const t = $("toast");
  const text = document.createElement("span");
  text.textContent = msg;
  t.replaceChildren(text);
  if (action) {
    const b = document.createElement("button");
    b.className = "toast-action";
    b.type = "button";
    b.textContent = action.label;
    b.addEventListener("click", () => { t.hidden = true; action.run(); });
    t.append(b);
  }
  t.classList.toggle("ok", ok);
  t.hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => { t.hidden = true; }, action ? 12000 : msg.length > 120 ? 9000 : 5000);
}

function fillOptions(sel, options) {
  sel.replaceChildren(...options.map((o) => new Option(o.label, String(o.value))));
}

function flash(sel) {
  sel.classList.add("flash");
  setTimeout(() => sel.classList.remove("flash"), 450);
}

function render(st) {
  const prev = state;
  state = st;
  const online = !!st.connected;
  document.body.classList.toggle("online", online);

  const conn = $("conn");
  // red, like offline: connected but the pedal stopped taking data (API state.stalled)
  const stalled = online && !!st.stalled;
  conn.dataset.state = stalled ? "stalled" : online ? "online" : "offline";
  conn.setAttribute("aria-label", stalled ? "Not responding" : online ? "Connected" : "Offline");
  conn.title = stalled ? "The pedal stopped accepting data.\nPower it off and on, run: sudo ./bin/l6-rebind release, then restart the editor"
    : online ? "Connected" : `Offline${st.error ? `: ${st.error}` : ""}\nTo connect: sudo ./bin/l6-rebind release and restart the editor`;

  const nameEl = $("now-name");
  const unsaved = online && !!st.dirty; // italic while there are unsaved changes (the * is only in the list)
  nameEl.textContent = online ? st.presetName || "—" : "No pedal";
  nameEl.classList.toggle("dirty", unsaved);
  nameEl.title = online ? `${st.slotLabel || ""} · Setlist ${st.setlist + 1}${st.setlistName ? " · " + st.setlistName : ""}` : "";
  $("preset-prev").disabled = $("preset-next").disabled = !online || transferRunning;

  renderTempo(st, prev);
  renderHeaderPresetMenu();
  if (prev && st.dspAlert && st.dspAlert !== prev.dspAlert) showDSPAlert();
  $("midi-btn").disabled = !st.connected;
  renderPresets(st, prev);
  renderFlowState(st);
  renderTuner(st);
  // The input block (flow.js) shows the inputs: redraw when they change.
  if (prev && (prev.input1 !== st.input1 || prev.input2 !== st.input2 || prev.inz !== st.inz || prev.inputsSetup !== st.inputsSetup || prev.variaxControl !== st.variaxControl || prev.midiChannel !== st.midiChannel) && flow.view) renderFlow();
  renderTransferButtons();

  for (const [key, sel] of Object.entries(selects)) {
    sel.disabled = !online;
    if (busy.has(key)) continue;
    const v = String(st[key]);
    if (sel.value !== v) {
      sel.value = v;
      if (prev) flash(sel); // changed by the pedal: draw attention
    }
  }
}

// ---------- Tap tempo ----------
const bpmInput = $("bpm");
const tempo = { sending: false, queued: null, taps: [], wheelTimer: 0, dragging: false };
const fmtBPM = (v) => (Math.round(v * 10) / 10).toFixed(1);
const clampBPM = (v) => Math.min(meta.tempo.max, Math.max(meta.tempo.min, v));

const TS_ICONS = { 0: "<svg viewBox=\"0 0 24 24\" aria-hidden=\"true\"><path d=\"M3 6h2M3 12h2M3 18h2M8 6h13M8 12h13M8 18h13\"/></svg>", 1: "<svg viewBox=\"0 0 24 24\" aria-hidden=\"true\"><circle cx=\"12\" cy=\"12\" r=\"9\"/><path d=\"M3 12h18M12 3c2.6 2.6 3.8 5.6 3.8 9s-1.2 6.4-3.8 9c-2.6-2.6-3.8-5.6-3.8-9S9.4 5.6 12 3z\"/></svg>" };

// ---------- Tempo Sync (POST /api/tempo-sync, state.tempoSync) ----------
function renderTempoSync(st) {
  const online = !!(st && st.connected);
  const btn = $("tsync-btn");
  btn.disabled = !online;
  const global = online && st.tempoSync === 1;
  // the button shows the mode in use (HX Edit): list = Per Preset, globe = Global
  const ic = $("tsync-icon"), want = TS_ICONS[global ? 1 : 0];
  if (ic.innerHTML !== want) ic.innerHTML = want;
  btn.title = `Tempo Sync: ${global ? "Global (one tempo for every preset)" : "Per Preset"}`;
  for (const b of $("tsync-menu").querySelectorAll("[data-ts]")) b.classList.toggle("cur", online && Number(b.dataset.ts) === st.tempoSync);
}

async function setTempoSync(value) {
  try {
    const res = await fetch("/api/tempo-sync", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value }),
    });
    if (!res.ok) throw await apiError(res);
  } catch (e) {
    toast(`Couldn't change Tempo Sync: ${e.message}`);
  }
}

function renderTempo(st, prev) {
  const online = !!st.connected;
  bpmInput.disabled = !online;
  renderTempoSync(st);
  // tempo not read yet (tuner open) or, with Tempo Sync = Global, the
  // pedal-wide tempo not known yet (the pedal can't be asked for it)
  if (!online || st.waitingTuner || !st.tempo) { bpmInput.value = "—"; return; }
  if (document.activeElement === bpmInput || tempo.sending || tempo.dragging) return;
  const v = fmtBPM(st.tempo);
  if (bpmInput.value !== v) bpmInput.value = v;
}

// Sends the newest value only; values typed/tapped while a request is in
// flight are coalesced into one follow-up request.
async function sendTempo(bpm) {
  bpm = Math.round(clampBPM(bpm) * 10) / 10;
  bpmInput.value = fmtBPM(bpm);
  if (tempo.sending) { tempo.queued = bpm; return; }
  tempo.sending = true;
  try {
    const res = await fetch("/api/tempo", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ bpm }),
    });
    if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  } catch (e) {
    toast(`Couldn't change the tempo: ${e.message}`);
    if (state) bpmInput.value = fmtBPM(state.tempo);
  } finally {
    tempo.sending = false;
    if (tempo.queued !== null) {
      const next = tempo.queued;
      tempo.queued = null;
      sendTempo(next);
    }
  }
}

// Tap tempo: left clicks on the BPM box (3+ taps make a tempo); a right-click
// opens it for typing.
function tap() {
  if (bpmInput.disabled) return;
  const now = performance.now();
  const t = tempo.taps;
  if (t.length && now - t[t.length - 1] > 2000) t.length = 0; // pause = new tempo
  t.push(now);
  if (t.length > 5) t.shift();
  const box = bpmInput.parentElement;
  box.classList.add("hit");
  setTimeout(() => box.classList.remove("hit"), 90);
  if (t.length < 3) return;
  const avg = (t[t.length - 1] - t[0]) / (t.length - 1);
  sendTempo(60000 / avg);
}

function commitTyped() {
  const v = parseFloat(bpmInput.value.replace(",", "."));
  if (!Number.isFinite(v)) {
    if (state) bpmInput.value = fmtBPM(state.tempo);
    return;
  }
  if (state && Math.abs(v - state.tempo) < 0.05) { bpmInput.value = fmtBPM(state.tempo); return; }
  sendTempo(v);
}

function stepTempo(delta) {
  const cur = parseFloat(bpmInput.value.replace(",", ".")) || (state ? state.tempo : 120);
  const next = clampBPM(Math.round((cur + delta) * 10) / 10);
  bpmInput.value = fmtBPM(next);
  clearTimeout(tempo.wheelTimer); // wait for the user to stop before sending
  tempo.wheelTimer = setTimeout(() => sendTempo(next), 250);
}

function initTempo() {
  bpmInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") { e.preventDefault(); bpmInput.blur(); }
    else if (e.key === "Escape") { if (state) bpmInput.value = fmtBPM(state.tempo); bpmInput.blur(); }
    else if (e.key === "ArrowUp" || e.key === "ArrowDown") {
      e.preventDefault();
      const step = e.altKey ? 0.1 : (e.shiftKey ? 10 : 1);
      stepTempo(e.key === "ArrowUp" ? step : -step);
    }
  });
  bpmInput.addEventListener("blur", commitTyped);
  bpmInput.addEventListener("focus", () => bpmInput.select());
  const field = bpmInput.parentElement;

  // Wheel / trackpad over the whole box. A mouse notch (|deltaY| >= 50) is one
  // step; trackpad deltas are accumulated so a swipe isn't dozens of steps.
  let wheelAcc = 0;
  field.addEventListener("wheel", (e) => {
    if (bpmInput.disabled) return;
    e.preventDefault();
    const fine = e.altKey || e.shiftKey;
    let steps;
    if (e.deltaMode !== 0 || Math.abs(e.deltaY) >= 50) {
      steps = -Math.sign(e.deltaY);
    } else {
      wheelAcc -= e.deltaY;
      steps = Math.trunc(wheelAcc / 25);
      wheelAcc -= steps * 25;
    }
    if (steps) stepTempo(steps * (fine ? 0.1 : 1));
  }, { passive: false });

  // Drag up/down on the value (like a plugin knob): 1 BPM every 4 px,
  // 0.1 BPM with Shift. A press without movement opens the field for typing.
  // The pedal follows during the drag (throttled), final value on release.
  let drag = null;
  const STEP_PX = 4;
  const endDrag = (e) => {
    if (!drag || e.pointerId !== drag.id) return;
    const d = drag;
    drag = null;
    clearTimeout(d.timer);
    tempo.dragging = false;
    field.classList.remove("dragging");
    document.body.classList.remove("dragging-bpm");
    if (field.hasPointerCapture(e.pointerId)) field.releasePointerCapture(e.pointerId);
    if (!d.moved) return; // a click: tapped on press (pointerdown)
    sendTempo(d.value);
  };
  field.addEventListener("pointerdown", (e) => {
    if (bpmInput.disabled || e.button !== 0 || document.activeElement === bpmInput) return;
    if (e.ctrlKey) return; // Ctrl+click = right-click on the Mac: typing, not a tap
    e.preventDefault(); // a gesture, not a text click: no caret yet
    field.setPointerCapture(e.pointerId);
    const start = parseFloat(bpmInput.value.replace(",", ".")) || (state ? state.tempo : 120);
    drag = { id: e.pointerId, lastY: e.clientY, acc: 0, value: start, moved: false, timer: 0 };
    tap(); // the beat lands on the press; a drag that follows doesn't complete a tap tempo
  });
  // right-click: type a value (a left click is a tap)
  field.addEventListener("contextmenu", (e) => {
    e.preventDefault();
    if (bpmInput.disabled) return;
    tempo.taps.length = 0;
    bpmInput.focus();
    bpmInput.select();
  });
  field.addEventListener("pointermove", (e) => {
    if (!drag || e.pointerId !== drag.id) return;
    drag.acc += drag.lastY - e.clientY; // up = positive
    drag.lastY = e.clientY;
    if (!drag.moved) {
      if (Math.abs(drag.acc) < 3) return;
      drag.moved = true;
      tempo.dragging = true;
      tempo.taps.length = 0; // a drag, not taps
      field.classList.add("dragging");
      document.body.classList.add("dragging-bpm");
    }
    const steps = Math.trunc(drag.acc / STEP_PX);
    if (!steps) return;
    drag.acc -= steps * STEP_PX;
    const next = clampBPM(Math.round((drag.value + steps * (e.shiftKey ? 0.1 : 1)) * 10) / 10);
    if (next === drag.value) return;
    drag.value = next;
    bpmInput.value = fmtBPM(next);
    if (!drag.timer) { // live update to the pedal, at most every 120 ms
      const d = drag;
      d.timer = setTimeout(() => { d.timer = 0; if (drag === d) sendTempo(d.value); }, 120);
    }
  });
  // Belt and braces: some browsers still focus the input on mousedown even
  // when pointerdown was prevented; focus is given on pointerup instead.
  field.addEventListener("mousedown", (e) => {
    if (document.activeElement !== bpmInput) e.preventDefault();
  });
  field.addEventListener("pointerup", endDrag);
  field.addEventListener("pointercancel", endDrag);
  // "T" taps anywhere, except while typing in a field.
  document.addEventListener("keydown", (e) => {
    const typing = e.target instanceof HTMLInputElement || e.target instanceof HTMLSelectElement;
    if (!typing && !e.metaKey && !e.ctrlKey && (e.key === "t" || e.key === "T")) tap();
  });
}

// ---------- Presets (setlist selector + 64 slots) ----------
const setlistSel = $("setlist");
const slotsEl = $("slots");
const pres = {
  view: null,        // setlist shown in the list (may differ from the pedal's)
  names: [],         // stored names of the viewed setlist ("" = not read yet)
  sel: null,         // anchor of the selection (last plain/cmd click)
  selected: new Set(), // highlighted slots (click, Shift+click range, Cmd+click toggle)
  edited: [],        // slots changed in the editor and not sent yet
  selectedAt: 0,     // when the current single selection was made
  renameTimer: 0,
  renaming: null,    // slot whose name is being edited
  rev: -1,           // last libraryRev fetched
  setlistsLoaded: false,
  setlists: [],      // /api/setlists: {index, name (editor), stored (pedal), edited}
  fetching: false, refetch: false,
  rows: [],
  lastCurrent: null,
};
const slotLabel = (i) => String(Math.floor(i / 4) + 1).padStart(2, "0") + "ABCD"[i % 4];

function buildSlots() {
  slotsEl.replaceChildren(...Array.from({ length: 64 }, (_, i) => {
    const li = document.createElement("li");
    li.className = "slot empty";
    li.draggable = true;
    li.dataset.slot = String(i);
    li.innerHTML = `<span class="mark"></span><span class="num">${slotLabel(i)}</span><span class="name"></span><span class="edited"></span>`;
    return li;
  }));
  pres.rows = [...slotsEl.children];
}

function renderSlots() {
  const st = state;
  const online = !!(st && st.connected);
  pres.rows.forEach((li, i) => {
    const isCur = online && !st.waitingTuner && st.setlist === pres.view && st.slot === i;
    // The loaded preset shows the edit buffer's name (reflects live renames),
    // unless the editor holds a pending copy/move for that slot.
    const name = isCur && !pres.edited[i] ? st.presetName : pres.names[i] || "";
    // "…" = being read; nothing while waiting for the tuner (the notice says why)
    if (pres.renaming !== i) li.querySelector(".name").textContent = name || (online && !st.waitingTuner ? "…" : "");
    li.classList.toggle("empty", !name);
    li.classList.toggle("current", isCur);
    li.querySelector(".mark").textContent = isCur ? "▶" : "";
    const dirty = (isCur && st.dirty) || !!pres.edited[i];
    li.classList.toggle("dirty", dirty);
    li.querySelector(".edited").textContent = dirty ? "*" : "";
    li.classList.toggle("selected", pres.selected.has(i));
  });
}

async function fetchSetlist() {
  if (pres.view === null) return;
  if (pres.fetching) { pres.refetch = true; return; }
  pres.fetching = true;
  try {
    const want = pres.view;
    const res = await fetch(`/api/setlist?n=${want}`);
    if (!res.ok) throw new Error((await res.text()).trim());
    const v = await res.json();
    if (v.setlist === pres.view) { pres.names = v.names; pres.edited = v.edited || []; renderSlots(); }
  } catch (e) {
    toast(`Couldn't read the setlist: ${e.message}`);
  } finally {
    pres.fetching = false;
    if (pres.refetch) { pres.refetch = false; fetchSetlist(); }
  }
}

// The editor's name of a setlist (a pending rename included).
const setlistName = (i) => pres.setlists[i]?.name || `Setlist ${i + 1}`;

async function loadSetlistNames() {
  pres.setlistsLoaded = true;
  try {
    const res = await fetch("/api/setlists");
    if (!res.ok) throw new Error((await res.text()).trim());
    const list = await res.json();
    pres.setlists = list;
    setlistSel.replaceChildren(...list.map((s) => {
      const o = new Option(`${setlistName(s.index)}${s.edited ? " *" : ""}`, String(s.index));
      if (s.edited) o.title = `On the pedal: "${s.stored}"`;
      return o;
    }));
    if (pres.view !== null) setlistSel.value = String(pres.view);
    setlistSel.classList.toggle("dirty", !!list[pres.view]?.edited);
    renderSetlistPicker();
  } catch (e) {
    pres.setlistsLoaded = false;
    toast(`Couldn't read the setlists: ${e.message}`);
  }
}

function showSetlist(n) {
  pres.view = n;
  pres.sel = null;
  pres.selected.clear();
  pres.names = [];
  pres.edited = [];
  setlistSel.value = String(n);
  setlistSel.classList.toggle("dirty", !!pres.setlists[n]?.edited);
  renderSetlistPicker();
  renderSlots();
  fetchSetlist();
}

// ---------- Setlist picker (Helix style: the list opens under the button) ----------
// Folder icons in the HX style (own drawing): closed for the other setlists,
// open (back panel + slanted front flap) for the current one.
const FOLDER = '<path class="fb" d="M3.5 18.5V5.5h5l1.6 1.8h10.4v11.2z"/>';
const FOLDER_OPEN = '<path class="fb" d="M4.5 18.5V5.5h5l1.6 1.8h7.4v11.2z"/><path class="ff" d="M2.5 10.5h15.2l2.3 8H4.8z"/>';

function renderSetlistPicker() {
  // Waiting for the tuner: the pedal's setlist isn't known yet ("—", not a
  // made-up "Setlist 1").
  const n = state?.waitingTuner ? null : pres.view;
  const cur = n === null ? null : pres.setlists[n];
  $("setlist-btn-name").textContent = n === null ? "—" : `${setlistName(n)}${cur?.edited ? " *" : ""}`;
  $("setlist-btn-count").textContent = n === null ? "" : `${n + 1}/8`;
  $("setlist-btn").classList.toggle("dirty", !!cur?.edited);
  $("setlist-btn").title = cur?.edited ? `Renamed in the editor; on the pedal: "${cur.stored}"` : "";
  $("setlist-list").replaceChildren(...pres.setlists.map((s) => {
    const li = document.createElement("li");
    li.setAttribute("role", "option");
    li.setAttribute("aria-selected", String(s.index === n));
    const b = document.createElement("button");
    b.type = "button";
    b.className = "lp-setlist-item" + (s.index === n ? " cur" : "") + (s.edited ? " dirty" : "");
    b.dataset.index = String(s.index);
    b.innerHTML = `<svg viewBox="0 0 24 24" aria-hidden="true">${s.index === n ? FOLDER_OPEN : FOLDER}</svg><span class="nm"></span><span class="st"></span>`;
    b.querySelector(".nm").textContent = `${setlistName(s.index)}${s.edited ? " *" : ""}`;
    b.querySelector(".st").textContent = s.edited ? `pedal: ${s.stored}` : "";
    li.append(b);
    return li;
  }));
}

function closeSetlistList() {
  $("setlist-list").hidden = true;
  $("setlist-btn").setAttribute("aria-expanded", "false");
}

function initSetlistPicker() {
  const btn = $("setlist-btn"), list = $("setlist-list");
  btn.addEventListener("click", (e) => {
    e.stopPropagation();
    const open = list.hidden;
    closeMenus();
    if (!open) return;
    list.hidden = false;
    btn.setAttribute("aria-expanded", "true");
    list.querySelector(".cur")?.focus({ preventScroll: true });
  });
  list.addEventListener("click", (e) => {
    const b = e.target.closest(".lp-setlist-item");
    if (!b) return;
    closeSetlistList();
    const n = Number(b.dataset.index);
    if (n !== pres.view) showSetlist(n);
    btn.focus();
  });
  list.addEventListener("keydown", (e) => {
    const items = [...list.querySelectorAll(".lp-setlist-item")];
    const i = items.indexOf(document.activeElement);
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      items[Math.max(0, Math.min(items.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)))]?.focus();
    } else if (e.key === "Escape") {
      closeSetlistList();
      btn.focus();
    }
  });
}

function renderPresets(st, prev) {
  const online = !!st.connected;
  setlistSel.disabled = !online;
  $("setlist-btn").disabled = !online;
  if (!online) { renderSlots(); return; }
  // The pedal doesn't answer setlist/preset reads with the tuner open: wait
  // (closing it bumps libraryRev and everything is fetched then).
  const tunerOn = !!st.tuner?.on;
  if (!pres.setlistsLoaded && !tunerOn) loadSetlistNames();
  // Follow the pedal when it changes setlist (front panel or our own load).
  if (pres.view === null || (prev && prev.connected && prev.setlist !== st.setlist)) {
    showSetlist(st.setlist);
  } else if (st.libraryRev !== pres.rev) {
    fetchSetlist();
    if (pres.setlistsLoaded) loadSetlistNames(); // renames, imports, SEND/RECEIVE
  }
  pres.rev = st.libraryRev;

  const prog = $("progress");
  if (st.loading) {
    prog.hidden = false;
    $("progress-bar").style.width = `${(100 * st.loading.done) / st.loading.total}%`;
    $("progress-text").textContent =
      `${st.loading.label || "Receiving presets"} ${st.loading.done}/${st.loading.total}`;
  } else if (st.waitingTuner) {
    prog.hidden = false;
    $("progress-bar").style.width = "0";
    $("progress-text").textContent = "Close the tuner to load presets";
  } else {
    prog.hidden = true;
  }

  renderSlots();
  const cur = `${st.setlist}:${st.slot}`;
  if (cur !== pres.lastCurrent && st.setlist === pres.view && pres.rows[st.slot]) {
    pres.rows[st.slot].scrollIntoView({ block: "nearest" });
  }
  pres.lastCurrent = cur;
}

async function loadPreset(slot) {
  if (!state || !state.connected || pres.view === null) return;
  const same = state.setlist === pres.view && state.slot === slot;
  if (state.dirty) {
    const what = same ? "Reloading this preset" : "Loading another preset";
    if (!window.confirm(`${what} discards the unsaved changes of "${state.presetName}" (${state.slotLabel}). Continue?`)) return;
  }
  try {
    const res = await fetch("/api/select", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ setlist: pres.view, slot }),
    });
    if (!res.ok) throw new Error((await res.text()).trim());
  } catch (e) {
    toast(`Couldn't load the preset: ${e.message}`);
  }
}

// ---------- Rename (inline, like the original editor) ----------
const MAX_NAME = 16;

// The pedal's LCD only has plain ASCII: strip accents (Ação -> Acao) and
// drop anything else that isn't printable ASCII.
const toPedalChars = (s) =>
  s.normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/[^\x20-\x7E]/g, "");

function startRename(i) {
  if (!state || !state.connected || pres.renaming !== null || transferRunning) return;
  const li = pres.rows[i];
  const span = li.querySelector(".name");
  const before = span.textContent === "…" ? "" : span.textContent;
  if (!before && !(state.setlist === pres.view && state.slot === i)) {
    toast("Wait for the setlist to finish loading before renaming.");
    return;
  }
  pres.renaming = i;
  const input = document.createElement("input");
  input.className = "rename";
  input.maxLength = MAX_NAME;
  input.value = before;
  input.spellcheck = false;
  input.setAttribute("aria-label", `New name for ${slotLabel(i)}`);
  span.replaceChildren(input);
  input.focus();
  input.select();

  let done = false;
  const finish = async (commit) => {
    if (done) return;
    done = true;
    const name = toPedalChars(input.value).trim();
    pres.renaming = null;
    span.textContent = commit && name ? name : before;
    slotsEl.focus();
    if (!commit || !name || name === before) { renderSlots(); return; }
    try {
      const res = await fetch("/api/rename", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ setlist: pres.view, slot: i, name }),
      });
      if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
    } catch (e) {
      span.textContent = before;
      toast(`Couldn't rename: ${e.message}`);
    }
    fetchSetlist();
  };
  input.addEventListener("input", () => {
    const clean = toPedalChars(input.value).slice(0, MAX_NAME);
    if (clean !== input.value) input.value = clean;
  });
  input.addEventListener("keydown", (e) => {
    e.stopPropagation(); // don't trigger list shortcuts / tap tempo
    if (e.key === "Enter") { e.preventDefault(); finish(true); }
    else if (e.key === "Escape") { e.preventDefault(); finish(false); }
  });
  input.addEventListener("blur", () => finish(true));
  input.addEventListener("click", (e) => e.stopPropagation());
}

// ---------- Copy / move / swap (drag & drop, Cmd+C / Cmd+V) ----------
let clipboard = null; // { setlist, slots: [...] }
const MODE_LABEL = { move: "move here", copy: "copy here", swap: "swap" };

async function arrange(op, from, to) {
  try {
    const res = await fetch("/api/arrange", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ op, from, to }),
    });
    if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
    // Select where the preset(s) ended up.
    if (to.setlist === pres.view) {
      const n = op === "copy" ? from.length : 1;
      pres.selected = new Set(Array.from({ length: n }, (_, k) => to.slot + k));
      pres.sel = to.slot;
    }
    const verb = op === "move" ? "Moved" : op === "swap" ? "Swapped" : `Copied ${from.length}`;
    toast(`${verb}. Pending in the editor (*): SEND to save or RECEIVE to undo.`, true);
    fetchSetlist();
    renderTransferButtons();
  } catch (e) {
    toast(e.message);
  }
}

function clearDropTarget() {
  for (const li of slotsEl.querySelectorAll(".drop-target")) {
    li.classList.remove("drop-target");
    delete li.dataset.label;
  }
}

function initArrange() {
  slotsEl.addEventListener("dragstart", (e) => {
    const li = e.target.closest(".slot");
    if (!li || pres.renaming !== null || !state || !state.connected) { e.preventDefault(); return; }
    pres.drag = { from: Number(li.dataset.slot) };
    e.dataTransfer.setData("text/plain", li.dataset.slot);
    e.dataTransfer.effectAllowed = "copyMove";
    li.classList.add("dragging");
  });
  slotsEl.addEventListener("dragover", (e) => {
    if (!pres.drag) return;
    const li = e.target.closest(".slot");
    if (!li) return;
    e.preventDefault();
    const mode = e.altKey ? "copy" : e.shiftKey ? "swap" : "move";
    e.dataTransfer.dropEffect = mode === "copy" ? "copy" : "move";
    if (!li.classList.contains("drop-target") || li.dataset.label !== MODE_LABEL[mode]) {
      clearDropTarget();
      if (Number(li.dataset.slot) !== pres.drag.from) {
        li.classList.add("drop-target");
        li.dataset.label = MODE_LABEL[mode];
      }
    }
  });
  slotsEl.addEventListener("dragleave", (e) => {
    if (!slotsEl.contains(e.relatedTarget)) clearDropTarget();
  });
  slotsEl.addEventListener("drop", (e) => {
    if (!pres.drag) return;
    e.preventDefault();
    const li = e.target.closest(".slot");
    const from = pres.drag.from;
    clearDropTarget();
    if (!li) return;
    const to = Number(li.dataset.slot);
    if (to === from) return;
    const op = e.altKey ? "copy" : e.shiftKey ? "swap" : "move";
    arrange(op, [{ setlist: pres.view, slot: from }], { setlist: pres.view, slot: to });
  });
  slotsEl.addEventListener("dragend", () => {
    pres.drag = null;
    clearDropTarget();
    for (const li of slotsEl.querySelectorAll(".dragging")) li.classList.remove("dragging");
  });

  // Cmd+C / Cmd+V: copy the selected presets, paste them (consecutively)
  // from the selected slot — also into another setlist.
  slotsEl.addEventListener("keydown", (e) => {
    if (e.target.closest("input") || !(e.metaKey || e.ctrlKey)) return;
    const key = e.key.toLowerCase();
    if (key === "c" && pres.selected.size) {
      e.preventDefault();
      copySelection();
    } else if (key === "v" && clipboard) {
      e.preventDefault();
      pasteClipboard();
    }
  });
}

// copySelection / pasteClipboard: Cmd+C / Cmd+V, the Copy / Paste buttons
// and the context menu.
function copySelection() {
  if (!pres.selected.size) return;
  clipboard = { setlist: pres.view, slots: [...pres.selected].sort((a, b) => a - b) };
  toast(`Copied: ${clipboard.slots.map(slotLabel).join(", ")}. Select the destination and paste (Paste or Cmd+V).`, true);
  renderFileButtons();
}

function pasteClipboard() {
  if (!clipboard) return;
  const target = pres.selected.size ? Math.min(...pres.selected) : pres.sel;
  if (target === null || target === undefined) { toast("Select the destination slot."); return; }
  const from = clipboard.slots.map((slot) => ({ setlist: clipboard.setlist, slot }));
  const names = clipboard.slots.length === 1 ? slotLabel(target)
    : `${slotLabel(target)}–${slotLabel(Math.min(63, target + clipboard.slots.length - 1))}`;
  if (!window.confirm(`Paste ${clipboard.slots.length} preset(s) over ${names} in this setlist?`)) return;
  arrange("copy", from, { setlist: pres.view, slot: target });
}

// ---------- Preset actions: header buttons and context menu ----------
function initPresetActions() {
  $("preset-copy").addEventListener("click", copySelection);
  $("preset-paste").addEventListener("click", pasteClipboard);
  $("preset-rename")?.addEventListener("click", () => {
    if (pres.selected.size === 1) startRename([...pres.selected][0]);
  });

  const menu = $("ctx-menu");
  const panel = menu.closest(".lp");
  slotsEl.addEventListener("contextmenu", (e) => {
    const li = e.target.closest(".slot");
    if (!li || !state || !state.connected || pres.renaming !== null) return;
    e.preventDefault();
    const i = Number(li.dataset.slot);
    if (!pres.selected.has(i)) { // right-click outside the selection selects that row
      pres.selected = new Set([i]);
      pres.sel = i;
      renderSlots();
      renderTransferButtons();
    }
    closeMenus();
    const one = pres.selected.size === 1;
    const busy = transferRunning || filesRunning;
    const enable = { load: one, rename: one, copy: true, paste: !!clipboard, export: !busy, import: !busy && canImport(), send: !busy, receive: !busy };
    for (const b of menu.querySelectorAll("[data-act]")) b.disabled = !enable[b.dataset.act];
    menu.hidden = false;
    const r = panel.getBoundingClientRect();
    const x = Math.min(e.clientX - r.left, r.width - menu.offsetWidth - 6);
    const y = Math.min(e.clientY - r.top, r.height - menu.offsetHeight - 6);
    menu.style.left = `${Math.max(6, x)}px`;
    menu.style.top = `${Math.max(6, y)}px`;
    menu.querySelector("button:not(:disabled)")?.focus({ preventScroll: true });
  });
  menu.addEventListener("click", (e) => {
    const b = e.target.closest("[data-act]");
    if (!b || b.disabled) return;
    closeMenus();
    presetAction(b.dataset.act, [...pres.selected][0]);
  });

  menu.addEventListener("keydown", (e) => {
    const items = [...menu.querySelectorAll("button:not(:disabled)")];
    const i = items.indexOf(document.activeElement);
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      items[(i + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length]?.focus();
    } else if (e.key === "Escape") {
      closeMenus();
      slotsEl.focus();
    }
  });
}

// presetAction: the preset menus' actions (list right-click and header "…"),
// on the selected slots (one = the single slot for load / rename).
function presetAction(act, one) {
  switch (act) {
    case "load": loadPreset(one); break;
    case "rename": startRename(one); break;
    case "copy": copySelection(); break;
    case "paste": pasteClipboard(); break;
    case "export": exportFiles("preset"); break;
    case "import": $("file-import").click(); break;
    case "send": runTransfer("send", "selected"); break;
    case "receive": runTransfer("receive", "selected"); break;
  }
}

function initPresets() {
  buildSlots();
  setlistSel.addEventListener("change", () => showSetlist(Number(setlistSel.value)));
  slotsEl.addEventListener("click", (e) => {
    const li = e.target.closest(".slot");
    if (!li) return;
    const i = Number(li.dataset.slot);
    if (e.shiftKey && pres.sel !== null) {        // range from the anchor
      const [a, b] = [Math.min(pres.sel, i), Math.max(pres.sel, i)];
      if (!e.metaKey) pres.selected.clear();
      for (let k = a; k <= b; k++) pres.selected.add(k);
    } else if (e.metaKey || e.ctrlKey) {         // toggle one
      pres.selected.has(i) ? pres.selected.delete(i) : pres.selected.add(i);
      pres.sel = i;
    } else {                                      // single
      // One click only selects (HX Edit); renaming is in the context menu / F2.
      pres.selected = new Set([i]);
      pres.sel = i;
    }
    renderSlots();
    renderTransferButtons();
  });
  slotsEl.addEventListener("dblclick", (e) => {
    if (e.target.closest("input")) return; // double-click inside the rename field
    clearTimeout(pres.renameTimer);
    const li = e.target.closest(".slot");
    if (li) loadPreset(Number(li.dataset.slot));
  });
  slotsEl.addEventListener("keydown", (e) => {
    if (e.target.closest("input")) return; // keys typed in the rename field
    if (e.key === "F2" && pres.selected.size === 1) {
      e.preventDefault();
      startRename([...pres.selected][0]);
    } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const from = pres.sel ?? (state && state.setlist === pres.view ? state.slot : -1);
      pres.sel = Math.max(0, Math.min(63, from + (e.key === "ArrowDown" ? 1 : -1)));
      pres.selected = new Set([pres.sel]);
      renderSlots();
      renderTransferButtons();
      pres.rows[pres.sel].scrollIntoView({ block: "nearest" });
    } else if (e.key === "Enter" && pres.sel !== null) {
      e.preventDefault();
      loadPreset(pres.sel);
    }
  });
}

// ---------- SEND / RECEIVE ----------
let transferRunning = false;

function renderTransferButtons() {
  const online = !!(state && state.connected);
  const off = !online || transferRunning || filesRunning;
  $("send-btn").disabled = off || pres.view === null;
  $("receive-btn").disabled = off || pres.view === null;
  for (const b of document.querySelectorAll("[data-kind][data-scope]")) {
    b.disabled = off || (b.dataset.scope === "selected" && pres.selected.size === 0) ||
      (b.dataset.scope !== "all" && pres.view === null);
    const c = b.querySelector("[data-count]");
    if (c) {
      const n = pendingInScope(b.dataset.scope).length;
      c.textContent = n ? `${n} changed` : "none";
    }
  }
  const n = pendingInScope("all").length;
  if ($("pending-count")) $("pending-count").textContent = n ? String(n) : "";
  $("send-btn").title = n ? `${n} unsaved change${n > 1 ? "s" : ""}: save to the pedal's memory` : "Save changes to the pedal's memory";
  if (off) closeMenus();
  renderFileButtons();
}

// ---------- Menus of the left panel (SEND, RECEIVE, Export, context menu) ----------
const MENUS = [["send-btn", "send-menu"], ["receive-btn", "receive-menu"], ["file-export", "export-menu"], ["snap-btn", "snap-menu"], ["tsync-btn", "tsync-menu"], ["preset-more", "preset-menu"]];

// ---------- DSP limit reached (state.dspAlert changes): warning over the flow ----------
let dspTimer = 0;
function showDSPAlert() {
  const el = $("dsp-alert");
  el.hidden = false;
  clearTimeout(dspTimer);
  dspTimer = setTimeout(() => { el.hidden = true; }, 4500);
  el.onclick = () => { el.hidden = true; };
}

// ---------- Header: show / hide the presets panel (remembered per browser) ----------
function setPresetsPanel(open) {
  document.querySelector(".layout").classList.toggle("lp-collapsed", !open);
  const b = $("lp-toggle");
  b.setAttribute("aria-expanded", String(open));
  b.title = open ? "Hide the presets panel" : "Show the presets panel";
  try { localStorage.setItem("lpCollapsed", open ? "0" : "1"); } catch {}
}

function initPresetsPanelToggle() {
  let collapsed = false;
  try { collapsed = localStorage.getItem("lpCollapsed") === "1"; } catch {}
  setPresetsPanel(!collapsed);
  $("lp-toggle").addEventListener("click", () => setPresetsPanel(document.querySelector(".layout").classList.contains("lp-collapsed")));
}

// ---------- Header: "…" menu of the loaded preset (acts on it in the list) ----------
function initHeaderPresetMenu() {
  $("preset-menu").addEventListener("click", (e) => {
    const b = e.target.closest("[data-act]");
    if (!b || b.disabled || !state || !state.connected) return;
    closeMenus();
    if (pres.view !== state.setlist) showSetlist(state.setlist);
    pres.selected = new Set([state.slot]);
    pres.sel = state.slot;
    renderSlots();
    renderTransferButtons();
    if (b.dataset.act === "rename") setPresetsPanel(true); // renaming happens in the list
    presetAction(b.dataset.act, state.slot);
  });
}

function renderHeaderPresetMenu() {
  const online = !!(state && state.connected) && !state.waitingTuner;
  $("preset-more").disabled = !online;
  const busy = transferRunning || filesRunning;
  const enable = { rename: true, copy: true, paste: !!clipboard, import: !busy && canImport(), export: !busy, send: !busy, receive: !busy };
  for (const b of $("preset-menu").querySelectorAll("[data-act]")) b.disabled = !online || !enable[b.dataset.act];
}

// ---------- Header: preset stepper (like the pedal's bank up/down) ----------
async function stepPreset(d) {
  if (!state || !state.connected || transferRunning) return;
  const slot = (state.slot + d + 64) % 64; // wraps inside the setlist, like the pedal
  if (state.dirty && !window.confirm(`Loading another preset discards the unsaved changes of "${state.presetName}" (${state.slotLabel}). Continue?`)) return;
  try {
    const res = await fetch("/api/select", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ setlist: state.setlist, slot }),
    });
    if (!res.ok) throw new Error((await res.text()).trim());
  } catch (e) {
    toast(`Couldn't load the preset: ${e.message}`);
  }
}

// ---------- Tuner (POST /api/tuner, state.tuner) ----------
function renderTuner(st) {
  const t = (st && st.tuner) || {};
  const online = !!(st && st.connected);
  const on = online && !!t.on;
  const btn = $("tuner-btn");
  btn.disabled = !online;
  btn.classList.toggle("on", on);
  btn.setAttribute("aria-pressed", String(on));
  $("tuner").hidden = !on;
  if (!on) return;
  // Mute / Bypass (the pedal's tuner knob 2)
  const mute = $("tuner-mute");
  mute.classList.toggle("on", !!t.mute);
  mute.setAttribute("aria-pressed", String(!!t.mute));
  $("tuner-mute-label").textContent = t.mute ? "Mute" : "Bypass";
  // Reference (knob 1); not while the user is typing in it
  const ref = $("tuner-ref");
  if (document.activeElement !== ref && t.reference) ref.value = `${t.reference} Hz`;
  // Reading (HX Edit): note between the chevrons, the bars stay still and a
  // 24px marker slides over them (±50 cents), coloured green→red by |cents|.
  const sig = !!t.signal;
  const inTune = sig && Math.abs(t.cents) <= 1;
  const col = sig ? centsColor(t.cents) : "";
  // Note letter + small raised flat (the API sends "Gb", HX Edit style).
  const note = $("tuner-note");
  note.textContent = sig ? t.note[0] : "";
  if (sig && t.note[1] === "b") {
    // Drawn, not the ♭ glyph: the list font lacks it and the fallback is
    // heavy; HX Edit's flat is a thin stroke, top-aligned with the letter.
    const fl = document.createElement("span");
    fl.className = "tuner-flat-sign";
    fl.innerHTML = '<svg viewBox="0 0 20 44" aria-hidden="true"><path d="M3 0V43M3 43C9 38 17 33 17 27C17 20 8 20 3 26"/></svg>';
    note.appendChild(fl);
  }
  const cv = $("tuner-cents");
  const rc = Math.round(t.cents);
  cv.textContent = sig ? (rc > 0 ? `+${rc}` : String(rc)) : "";
  cv.style.color = col;
  $("tuner-cents-box").hidden = !sig;
  $("tuner-flat").classList.toggle("lit", sig && (inTune || t.cents < 0));
  $("tuner-sharp").classList.toggle("lit", sig && (inTune || t.cents > 0));
  const mk = $("tuner-mark");
  mk.hidden = !sig;
  if (sig) {
    const c = Math.max(-52, Math.min(52, t.cents));
    mk.style.left = `calc(50% + ${(c * 0.96).toFixed(2)}%)`;
    mk.style.background = col;
  }
}

// Marker colour by distance from the note, measured on HX Edit:
// 0 #50f44f, 12 #97e44b, 21 #d1d647, 33 #f77f31, 46+ #ff001c.
const CENTS_STOPS = [[0, [0x50, 0xf4, 0x4f]], [12, [0x97, 0xe4, 0x4b]], [21, [0xd1, 0xd6, 0x47]], [33, [0xf7, 0x7f, 0x31]], [46, [0xff, 0x00, 0x1c]]];
function centsColor(cents) {
  const a = Math.min(46, Math.abs(cents));
  let i = 0;
  while (i < CENTS_STOPS.length - 2 && a > CENTS_STOPS[i + 1][0]) i++;
  const [x0, c0] = CENTS_STOPS[i], [x1, c1] = CENTS_STOPS[i + 1];
  const f = Math.max(0, Math.min(1, (a - x0) / (x1 - x0)));
  const ch = c0.map((v, k) => Math.round(v + (c1[k] - v) * f));
  return `rgb(${ch[0]}, ${ch[1]}, ${ch[2]})`;
}

const setTuner = (on) => tunerPost({ on });

function initTunerSettings() {
  $("tuner-mute").addEventListener("click", () => tunerPost({ mute: !(state && state.tuner && state.tuner.mute) }));
  const ref = $("tuner-ref");
  const cur = () => (state && state.tuner && state.tuner.reference) || 440;
  const setRef = (hz) => {
    hz = Math.round(Math.min(455, Math.max(425, hz)));
    ref.value = `${hz} Hz`;
    if (hz !== cur()) tunerPost({ reference: hz });
  };
  $("tuner-ref-up").addEventListener("click", () => setRef(cur() + 1));
  $("tuner-ref-down").addEventListener("click", () => setRef(cur() - 1));
  ref.addEventListener("wheel", (e) => { e.preventDefault(); setRef(cur() + (e.deltaY < 0 ? 1 : -1)); }, { passive: false });
  ref.addEventListener("keydown", (e) => {
    if (e.key === "Enter") ref.blur();
    else if (e.key === "ArrowUp" || e.key === "ArrowDown") { e.preventDefault(); setRef(cur() + (e.key === "ArrowUp" ? 1 : -1)); }
    else if (e.key === "Escape") { ref.value = `${cur()} Hz`; ref.blur(); }
  });
  ref.addEventListener("focus", () => { ref.value = String(cur()); ref.select(); });
  ref.addEventListener("blur", () => {
    const v = parseInt(ref.value, 10);
    if (Number.isFinite(v)) setRef(v); else ref.value = `${cur()} Hz`;
  });
}

async function tunerPost(body) {
  try {
    const res = await fetch("/api/tuner", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error((await res.text()).trim());
  } catch (e) {
    toast(`Tuner: ${e.message}`);
  }
}

// Snapshots: interface only for now (the HD500 has none; planned for later).
function initHeader() {
  $("preset-prev").addEventListener("click", () => stepPreset(-1));
  $("preset-next").addEventListener("click", () => stepPreset(1));
  $("tuner-btn").addEventListener("click", () => setTuner(!(state && state.tuner && state.tuner.on)));
  initTunerSettings();
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && e.target.id !== "tuner-ref" && state && state.tuner && state.tuner.on) setTuner(false);
  });
  $("tsync-menu").addEventListener("click", (e) => {
    const b = e.target.closest("[data-ts]");
    if (!b) return;
    closeMenus();
    if (state && Number(b.dataset.ts) !== state.tempoSync) setTempoSync(Number(b.dataset.ts));
  });
  const menu = $("snap-menu");
  for (let n = 1; n <= 8; n++) {
    const b = document.createElement("button");
    b.type = "button";
    b.setAttribute("role", "menuitem");
    b.dataset.snap = String(n);
    b.innerHTML = `<span class="snap-num"><svg viewBox="0 0 24 18" aria-hidden="true"><path d="M2.5 4.8h4.6l1.7-2.6h6.4l1.7 2.6h4.6a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1h-19a1 1 0 0 1-1-1v-10a1 1 0 0 1 1-1z"/><circle cx="12" cy="10.8" r="4.9"/></svg><b>${n}</b></span>SNAPSHOT ${n}`;
    menu.appendChild(b);
  }
  menu.addEventListener("click", (e) => {
    const b = e.target.closest("[data-snap]");
    if (!b) return;
    $("snap-num").textContent = b.dataset.snap;
    $("snap-label").textContent = `SNAPSHOT ${b.dataset.snap}`;
    for (const x of menu.children) x.classList.toggle("cur", x === b);
    closeMenus();
  });
  menu.firstChild.classList.add("cur");
}

function closeMenus() {
  for (const [b, m] of MENUS) {
    $(m).hidden = true;
    $(b).setAttribute("aria-expanded", "false");
  }
  $("ctx-menu").hidden = true;
  closeSetlistList();
}

function initMenus() {
  for (const [b, m] of MENUS) {
    $(b).addEventListener("click", (e) => {
      e.stopPropagation();
      const open = $(m).hidden;
      closeMenus();
      if (!open) return;
      renderTransferButtons(); // fresh counts and enabled items
      $(m).hidden = false;
      $(b).setAttribute("aria-expanded", "true");
    });
  }
  document.addEventListener("click", (e) => {
    if (!e.target.closest(".menu-wrap, .ctx-menu, .lp-setlist")) closeMenus();
  });
  document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeMenus(); });
}

// What a SEND in this scope would write, as far as the UI knows (the server
// decides for real): the loaded preset if edited + editor-edited slots.
function pendingInScope(scope) {
  const st = state;
  const inScope = (set, slot) =>
    scope === "all" || (set === pres.view && (scope === "setlist" || pres.selected.has(slot)));
  const out = [];
  if (scope !== "selected") { // setlist renames go with Set List / All
    for (const s of pres.setlists) {
      if (s.edited && (scope === "all" || s.index === pres.view)) out.push(`Setlist ${s.index + 1} name: "${s.stored}" → "${s.name}"`);
    }
  }
  const curDirty = st && st.dirty && inScope(st.setlist, st.slot);
  if (curDirty) out.push(`${st.slotLabel} ${st.presetName}`);
  pres.edited.forEach((e, i) => {
    if (!e || !inScope(pres.view, i)) return;
    // The loaded slot was already listed if its pedal buffer is dirty (the
    // server saves the buffer then). Otherwise its editor change (copy/move)
    // must be listed like any other slot.
    const isCur = st && st.setlist === pres.view && st.slot === i;
    if (isCur && curDirty) return;
    out.push(`${slotLabel(i)} ${pres.names[i] || ""}${isCur ? " (loaded: the pedal reloads it)" : ""}`);
  });
  return out;
}

async function runTransfer(kind, scope) {
  const setName = setlistName(pres.view);
  const where = scope === "all" ? "all setlists"
    : scope === "setlist" ? `setlist "${setName}"`
    : `${pres.selected.size} selected preset(s) in "${setName}"`;

  if (kind === "send") {
    const pending = pendingInScope(scope);
    if (scope !== "all" && pending.length === 0) {
      toast(`Nothing to send: no changed presets in ${where}.`);
      return;
    }
    if (pending.length && !window.confirm(
      `Save to the pedal's memory (overwrites what is stored):\n\n${pending.join("\n")}\n\nContinue?`)) return;
  } else {
    const st = state;
    const curInScope = st && st.dirty && (scope === "all" ||
      (st.setlist === pres.view && (scope === "setlist" || pres.selected.has(st.slot))));
    if (!window.confirm(`Read ${where} again from the pedal?` +
      (curInScope ? `\n\nThe unsaved changes of "${st.presetName}" (${st.slotLabel}) will be discarded.` : ""))) return;
  }

  transferRunning = true;
  renderTransferButtons();
  try {
    const body = { scope, setlist: pres.view ?? 0, slots: [...pres.selected].sort((a, b) => a - b) };
    const res = await fetch(`/api/${kind}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
    const r = await res.json();
    if (r.nothing) toast("Nothing to send: no changed presets.");
    else if (r.failed && r.failed.length) toast(`Finished with errors (${r.done} ok): ${r.failed.join("; ")}`);
    else toast(kind === "send" ? `Saved and verified: ${r.done} preset(s).` : `Received from the pedal: ${r.done} preset(s).`, true);
    fetchSetlist();
  } catch (e) {
    toast(`${kind === "send" ? "SEND" : "RECEIVE"} failed: ${e.message}`);
  } finally {
    transferRunning = false;
    renderTransferButtons();
  }
}

function initTransfer() {
  for (const b of document.querySelectorAll("[data-kind][data-scope]")) {
    b.addEventListener("click", () => { closeMenus(); runTransfer(b.dataset.kind, b.dataset.scope); });
  }
}

// ---------- Files: import / export (.h5e .h5s .h5b) and backup ----------
let filesRunning = false;
const FILE_RE = /\.(h5e|h5s|h5b|5xe|5xs|5xb)$/i;

// canImport: files can be imported now (the target slot comes from the
// selection or, for files dropped from the Finder, from the drop point).
function canImport() {
  return !!(state && state.connected) && !filesRunning && !transferRunning && pres.view !== null;
}

function renderFileButtons() {
  const online = !!(state && state.connected);
  const off = !online || filesRunning || transferRunning;
  const noView = pres.view === null;
  const sel = pres.selected.size;
  $("file-import").disabled = off || noView || (!sel && pres.sel === null);
  $("setlist-import").disabled = off || noView;
  $("file-export").disabled = off || noView;
  $("preset-export").disabled = off || noView || !sel;
  if ($("file-backup")) $("file-backup").disabled = off;
  $("setlist-edit").disabled = off || noView;
  $("preset-copy").disabled = off || noView || !sel;
  $("preset-paste").disabled = off || noView || !sel || !clipboard;
  if ($("preset-rename")) $("preset-rename").disabled = off || noView || sel !== 1;
}

async function withFiles(fn) {
  filesRunning = true;
  renderFileButtons();
  renderTransferButtons();
  try {
    await fn();
  } catch (e) {
    toast(e.message);
  } finally {
    filesRunning = false;
    renderFileButtons();
    renderTransferButtons();
  }
}

async function apiError(res) {
  return new Error((await res.text()).trim() || res.statusText);
}

// download fetches url and saves the response with the server's file name.
async function download(url) {
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

function exportFiles(kind) {
  closeMenus();
  withFiles(async () => {
    const set = pres.view;
    const names = [];
    if (kind === "preset") {
      for (const slot of [...pres.selected].sort((a, b) => a - b)) {
        names.push(await download(`/api/export?kind=preset&setlist=${set}&slot=${slot}`));
      }
    } else {
      if (kind === "bundle") toast("Reading the remaining setlists from the pedal…", true);
      names.push(await download(`/api/export?kind=${kind}&setlist=${set}`));
    }
    toast(`Exported to Downloads: ${names.join(", ")}`, true);
  });
}

function backup() {
  if (!window.confirm("Back up the whole pedal?\n\nThe editor reads all 512 presets and the setlist names again (about 40 s) and saves an .h5b file in Documents › POD HD500 Edit X › Backups.\n\nPending changes (*) are not included in the backup.")) return;
  withFiles(async () => {
    const res = await fetch("/api/backup", { method: "POST" });
    if (!res.ok) throw await apiError(res);
    const r = await res.json();
    const file = r.path.split("/").pop();
    toast(`Backup saved: ${file} (${r.presets} presets).`, true, {
      label: "Show in Finder",
      run: () => fetch("/api/reveal", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: r.path }),
      }),
    });
  });
}

const fileKind = async (f) => {
  const magic = await f.slice(0, 4).text();
  return { H5EP: "preset", H5ES: "setlist", H5EB: "bundle" }[magic] || null;
};

// importFiles sends the files to the editor's copy; target is the slot where
// preset files go (consecutively). Setlists go to the viewed setlist.
function importFiles(list, target) {
  const files = [...list];
  if (!files.length) return;
  withFiles(async () => {
    const kinds = await Promise.all(files.map(fileKind));
    const bad = files.filter((f, i) => !kinds[i]);
    if (bad.length) throw new Error(`Not a POD HD500 Edit file: ${bad.map((f) => f.name).join(", ")}`);
    const kind = kinds[0];
    if (files.length > 1 && kinds.some((k) => k !== "preset")) {
      throw new Error("Import one setlist (.h5s) or bundle (.h5b) at a time; multiple files only for presets (.h5e).");
    }
    const setName = setlistName(pres.view);
    if (kind === "preset") {
      if (target === null || target === undefined) throw new Error("Select the destination slot before importing.");
      if (target + files.length > 64) throw new Error(`Doesn't fit: ${files.length} preset(s) starting at ${slotLabel(target)} go past 16D.`);
      const range = files.length > 1 ? `${slotLabel(target)}–${slotLabel(target + files.length - 1)}` : slotLabel(target);
      const over = [];
      for (let k = 0; k < files.length; k++) if (pres.names[target + k]) over.push(`${slotLabel(target + k)} ${pres.names[target + k]}`);
      if (over.length > 1 && !window.confirm(`Import ${files.length} presets into ${range} of "${setName}", replacing in the editor:\n\n${over.join("\n")}\n\nNothing is saved to the pedal until SEND. Continue?`)) return;
    } else if (kind === "setlist") {
      if (!window.confirm(`Import the setlist "${files[0].name}", replacing the 64 presets and the name of "${setName}" in the editor?\n\nNothing is saved to the pedal until SEND (RECEIVE undoes it).`)) return;
    } else if (!window.confirm(`Import the bundle "${files[0].name}", replacing the 8 setlists (512 presets and names) in the editor?\n\nNothing is saved to the pedal until SEND › All (RECEIVE › All undoes it).`)) return;

    if (kind !== "preset" && pres.edited.some(Boolean) && !window.confirm("There are pending changes (*) that the file will replace. Continue?")) return;

    const fd = new FormData();
    fd.append("setlist", String(pres.view));
    fd.append("slot", String(target ?? 0));
    for (const f of files) fd.append("file", f, f.name);
    if (kind === "bundle") toast("Reading the remaining setlists from the pedal…", true);
    const res = await fetch("/api/import", { method: "POST", body: fd });
    if (!res.ok) throw await apiError(res);
    const r = await res.json();
    if (kind === "preset") {
      pres.selected = new Set(files.map((_, k) => target + k));
      pres.sel = target;
    }
    fetchSetlist();
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

function initFiles() {
  const input = $("file-input");
  const pick = (accept) => { input.accept = accept; input.value = ""; input.click(); };
  $("file-import").addEventListener("click", () => pick(".h5e,.5xe"));            // presets
  $("setlist-import").addEventListener("click", () => pick(".h5s,.h5b,.5xs,.5xb")); // setlist / bundle
  input.addEventListener("change", () => {
    const target = pres.selected.size ? Math.min(...pres.selected) : pres.sel;
    importFiles(input.files, target);
  });
  $("preset-export").addEventListener("click", () => exportFiles("preset"));
  for (const b of $("export-menu").querySelectorAll("[data-export]")) {
    b.addEventListener("click", () => exportFiles(b.dataset.export));
  }
  $("file-backup")?.addEventListener("click", backup);

  // Files dragged from the Finder onto the list: presets go to the slot under
  // the pointer, a setlist/bundle anywhere on the list.
  const hasFiles = (e) => e.dataTransfer && [...e.dataTransfer.types].includes("Files");
  slotsEl.addEventListener("dragover", (e) => {
    if (pres.drag || !hasFiles(e) || !canImport()) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
    slotsEl.classList.add("file-over");
    const li = e.target.closest(".slot");
    if (li && !li.classList.contains("drop-target")) {
      clearDropTarget();
      li.classList.add("drop-target");
      li.dataset.label = "import here";
    }
  });
  slotsEl.addEventListener("dragleave", (e) => {
    if (!slotsEl.contains(e.relatedTarget)) slotsEl.classList.remove("file-over");
  });
  slotsEl.addEventListener("drop", (e) => {
    if (pres.drag || !hasFiles(e)) return;
    e.preventDefault();
    slotsEl.classList.remove("file-over");
    clearDropTarget();
    if (!canImport()) return;
    const files = [...e.dataTransfer.files].filter((f) => FILE_RE.test(f.name));
    if (!files.length) { toast("Drag .h5e, .h5s or .h5b files."); return; }
    const li = e.target.closest(".slot");
    importFiles(files, li ? Number(li.dataset.slot) : (pres.selected.size ? Math.min(...pres.selected) : pres.sel));
  });
  // Dropping a file outside the list must not navigate away from the editor.
  window.addEventListener("dragover", (e) => { if (hasFiles(e)) e.preventDefault(); });
  window.addEventListener("drop", (e) => { if (hasFiles(e)) e.preventDefault(); });
}

async function onChange(key) {
  const sel = selects[key];
  const value = Number(sel.value);
  const before = state ? String(state[key]) : sel.value;
  busy.add(key);
  try {
    // Inputs Setup is a pedal-wide (global) setting; the others belong to the preset.
    const res = key === "inputsSetup"
      ? await fetch("/api/inputs-setup", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ value }),
        })
      : await fetch("/api/input", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ id: meta.setupInput[key], value }),
        });
    if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  } catch (e) {
    sel.value = before;
    toast(`Couldn't change it: ${e.message}`);
  } finally {
    busy.delete(key);
  }
}

async function init() {
  meta = await (await fetch("/api/meta")).json();
  initTempo();
  initPresets();
  initTransfer();
  initArrange();
  initFiles();
  initSetlistRename();
  initSetlistPicker();
  initPresetActions();
  initMenus();
  initPresetsPanelToggle();
  initHeaderPresetMenu();
  initMidi();
  initHeader();
  initFlow();

  // The pedal doesn't announce global changes made on its front panel:
  // re-read when the user comes back to the editor window.
  window.addEventListener("focus", () => {
    if (state && state.connected) fetch("/api/refresh?globals=1", { method: "POST" }).catch(() => {});
  });
  for (const key of Object.keys(selects)) {
    selects[key].addEventListener("change", () => onChange(key));
  }

  const es = new EventSource("/api/events");
  es.onmessage = (ev) => render(JSON.parse(ev.data));
  // After a server restart the event stream reconnects: if it is another
  // server run (maybe a new build of the UI), reload to get the new page.
  es.onopen = async () => {
    try {
      const m = await (await fetch("/api/meta")).json();
      if (meta.server && m.server !== meta.server) location.reload();
    } catch { /* still restarting: the next reconnect checks again */ }
  };
  es.onerror = () => {
    const conn = $("conn");
    conn.dataset.state = "offline";
    conn.setAttribute("aria-label", "Editor disconnected");
    conn.title = "Editor disconnected (is the server running?)";
  };
}

init().catch((e) => toast(`Failed to start: ${e.message}`));

// ---------- Setlist rename (pending until SEND › Set List / All) ----------
function initSetlistRename() {
  const input = $("setlist-rename");
  const btn = $("setlist-edit");
  let set = null;
  const close = () => {
    input.hidden = true;
    $("setlist-btn").hidden = false;
    btn.textContent = "Rename";
    set = null;
  };
  const commit = async () => {
    if (set === null) return;
    const target = set;
    const name = toPedalChars(input.value).trim().slice(0, MAX_NAME);
    close();
    if (!name || name === setlistName(target)) return;
    try {
      const res = await fetch("/api/setlist-name", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ setlist: target, name }),
      });
      if (!res.ok) throw await apiError(res);
      await loadSetlistNames();
      const s = pres.setlists[target];
      toast(s && s.edited
        ? `Setlist renamed to "${name}" in the editor (*). Use SEND › Set List to save it to the pedal.`
        : `Same name as on the pedal: nothing pending.`, true);
      renderTransferButtons();
    } catch (e) {
      toast(`Couldn't rename the setlist: ${e.message}`);
    }
  };
  btn.addEventListener("click", () => {
    if (set !== null) { commit(); return; }
    if (pres.view === null) return;
    set = pres.view;
    input.value = setlistName(set);
    $("setlist-btn").hidden = true;
    input.hidden = false;
    btn.textContent = "OK";
    input.focus();
    input.select();
  });
  input.addEventListener("input", () => {
    const clean = toPedalChars(input.value).slice(0, MAX_NAME);
    if (clean !== input.value) input.value = clean;
  });
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") { e.preventDefault(); commit(); }
    else if (e.key === "Escape") { e.preventDefault(); close(); }
  });
  // Clicking the OK button blurs the input first: let the click commit.
  input.addEventListener("blur", (e) => { if (e.relatedTarget !== btn) commit(); });
}

