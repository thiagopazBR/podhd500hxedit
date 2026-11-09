// POD HD500 Edit — Signal Flow: the blocks of the preset loaded on the pedal
// (its edit buffer), drawn as one SVG chain in a minimal, category-coloured
// style. Icons are original line drawings.
//
// Lanes of FX blocks (byte 6 of a block): 0 before the split, 1/2 path A/B
// before the amp, 3/4 path A/B after the amp, 5 after the mixer.
// Loaded before app.js; uses its helpers ($, toast, apiError) at call time.
"use strict";

const flow = {
  view: null, rev: -1, fetching: false, refetch: false, sel: null, presetKey: "",
  models: [],      // /api/models: [{name, kind, models: [{type, name, basedOn}]}]
  cat: null,       // category shown in the browser (null = the block's own)
  dragging: false, // a slider is being dragged: don't rebuild the parameters
  sliders: {},     // editable sliders on screen, by key (see prmRow)
};
const FS_LABEL = ["", "FS1", "FS2", "FS3", "FS4", "FS5", "FS6", "FS7", "FS8", "EXP"];
const LANE_LABEL = ["Pre", "Path A pre", "Path B pre", "Path A post", "Path B post", "Post"];
const esc = (s) => String(s ?? "").replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

// Category icons in the HX Edit style (32×32 line drawings from the user's
// set, mapped to the HD500's categories). Drawn with currentColor and inline
// styles, so the calibrated category colour (and the list's dimmed tone)
// apply and the editor's generic `svg path` rules can't restyle them.
const S = (w, op = 1) => `style="fill:none;stroke:currentColor;stroke-width:${w};stroke-linecap:round;stroke-linejoin:round${op < 1 ? `;stroke-opacity:${op}` : ""}"`;
const F = (op = 1) => `style="fill:currentColor;stroke:none${op < 1 ? `;fill-opacity:${op}` : ""}"`;
const ICONS32 = {
  distortion: `<path d="M4.6 15.66L4.93 6.54Q7.16 9.21 9.4 6.54L10.7 25.46Q13 22.79 15.3 25.46L16.15 6.54Q18.5 9.21 20.85 6.54L22.15 25.46Q24.45 22.79 26.75 25.46L27.07 17.03" ${S(1.8, 0.85)}/>`, // no halo: it read as bold in the chain
  dynamics: `<path d="M13 19L26 6" ${S(1, 0.45)}/><path d="M13 19L26 17.5" ${S(1, 0.45)}/><path d="M6 26L13 19L26 12" ${S(1.8)}/>`, // thinner: read as bold in the chain
  eq: `<path d="M4.5 15.5C6.2 15.5 7.2 6.3 9.1 6.3C11 6.3 12.1 15.5 13.8 15.5" ${S(2.06, 0.1)}/><path d="M13.8 15.5C16 15.5 17.6 8 20.6 8C23.6 8 25.3 15.5 27.5 15.5" ${S(2.06, 0.1)}/><path d="M13.8 15.5C15.6 15.5 17.5 23.3 20.3 23.3C23 23.3 24.7 17.8 26.5 15.5" ${S(2, 0.08)}/><path d="M2.5 15.5H7.6C10.2 15.5 13.2 25 16 25C18.8 25 21.8 15.5 24.4 15.5H29.5" ${S(2.79, 0.27)}/>` +
    `<path d="M4.5 15.5C6.2 15.5 7.2 6.3 9.1 6.3C11 6.3 12.1 15.5 13.8 15.5" ${S(1.15, 0.34)}/><path d="M13.8 15.5C16 15.5 17.6 8 20.6 8C23.6 8 25.3 15.5 27.5 15.5" ${S(1.15, 0.34)}/><path d="M13.8 15.5C15.6 15.5 17.5 23.3 20.3 23.3C23 23.3 24.7 17.8 26.5 15.5" ${S(1.09, 0.28)}/><path d="M6.5 15.5H25.5" ${S(1.2, 0.4)}/><path d="M2.5 15.5H7.6C10.2 15.5 13.2 25 16 25C18.8 25 21.8 15.5 24.4 15.5H29.5" ${S(1.55)}/>`,
  modulation: `<path class="ic-mod" d="M8 16C9.5 7 12.5 7 14 16C15.5 25 18.5 25 20 16C21.5 7 24.5 7 26 16C27 22 28.5 24 29.5 18" ${S(1.15, 0.45)}/>` +
    `<path class="ic-mod" d="M4.5 16C6 7 9 7 10.5 16C12 25 15 25 16.5 16C18 7 21 7 22.5 16C23.8 24 26.2 24 27.5 16" ${S(1.8)}/>`, // thinner: read as bold in the chain
  delay: `<circle cx="14" cy="16" r="8.5" ${S(1.8)}/><circle cx="17.5" cy="16" r="6.2" ${S(1.3, 0.75)}/><circle cx="20.5" cy="16" r="4.2" ${S(1.1, 0.5)}/><circle cx="23" cy="16" r="2.5" ${S(0.9, 0.3)}/>`, // thinner: read as bold
  reverb: `<path d="M16 14L16 23M6 19L16 14L26 19" ${S(1.3, 0.45)}/><path d="M16 7L26 11.5L26 20.5L16 25L6 20.5L6 11.5Z" ${S(2.2)}/><path d="M6 11.5L16 16L26 11.5M16 16L16 25" ${S(2.2)}/>`,
  pitch: `<path d="M6 7H10.5V18H12.5V25H6Z" ${F()}/><path d="M14.2 7H17.8V18H19.5V25H12.5V18H14.2Z" ${F()}/><path d="M21.5 7H26V25H19.5V18H21.5Z" ${F()}/>` +
    `<path d="M12.5 18V25M19.5 18V25" style="fill:none;stroke:#0c0c0c;stroke-width:1.3"/>`,
  filter: `<path d="M4 9H27" ${S(1.2, 0.5)}/><path d="M4 9H18L24 23H13L7 9" ${S(2)}/><path d="M10.5 9L16.5 23" ${S(1.8)}/><path d="M14.2 9L20.2 23" ${S(1.8)}/><path d="M13 23H27" ${S(2)}/>`,
  wah: `<path class="ic-halo" d="M12.6 2.4H19.4Q21.6 2.4 21.6 4.6V17.36L20.45 20.9V27.4Q20.45 29.6 18.25 29.6H13.75Q11.55 29.6 11.55 27.4V20.9L10.4 17.36V4.6Q10.4 2.4 12.6 2.4Z" style="fill:currentColor;fill-opacity:.13;stroke:currentColor;stroke-opacity:.29;stroke-width:3.3;stroke-linejoin:round"/>` +
    `<path class="ic-main" d="M12.6 2.4H19.4Q21.6 2.4 21.6 4.6V17.36L20.45 20.9V27.4Q20.45 29.6 18.25 29.6H13.75Q11.55 29.6 11.55 27.4V20.9L10.4 17.36V4.6Q10.4 2.4 12.6 2.4Z" ${S(1.9)}/><path class="ic-main" d="M11.8 2.6H20.2" ${S(2.2, 0.91)}/>` +
    `<path class="ic-main" d="M11.3 5.94H20.7" ${S(1.5, 0.92)}/><path class="ic-thin" d="M11.3 9.74H20.7" ${S(1.2, 0.35)}/><path class="ic-thin" d="M11.3 13.55H20.7" ${S(1.2, 0.35)}/><path class="ic-thin" d="M11.3 17.36H20.7" ${S(1.2, 0.35)}/><path class="ic-thin" d="M12.4 21.17H19.6" ${S(1.2, 0.35)}/><path class="ic-thin" d="M12.4 25.25H19.6" ${S(1.2, 0.35)}/>`,
  amp: `<path d="M11 12V10.2C11 9.5 12.5 9 16 9C19.5 9 21 9.5 21 10.2V12" ${S(1.6)}/><rect x="4.5" y="12" width="23" height="10" rx="1.8" ${S(1.8)}/>` +
    `<path d="M4.5 17.5H27.5" ${S(1.45)}/><path d="M8 23.2H10.5M21.5 23.2H24" ${S(1.6)}/>`, // thinner: read as bold
  preamp: `<path d="M8 15V10.5C8 9 9 8 10.2 8C11.4 8 12.4 9 12.4 10.5V15Z" ${F()}/><path d="M13.6 15V10.5C13.6 9 14.6 8 15.8 8C17 8 18 9 18 10.5V15Z" ${F()}/>` +
    `<rect x="5" y="15" width="22" height="6.5" rx="1.5" ${S(2.1)}/><path d="M9 15V21.5M23 15V21.5" ${S(1.6)}/>`,
  cab: `<path d="M9 13H6.5C5.5 13 5 14 5 16C5 18 5.5 19 6.5 19H9Z" ${F()}/><path d="M9 12.5L16 6.5M9 19.5L16 25.5" ${S(1.8)}/>` +
    `<ellipse cx="17.5" cy="16" rx="6.5" ry="9.8" ${S(1.85)}/><ellipse cx="13.5" cy="16" rx="2.4" ry="3.2" ${F()}/>`,
  volume: `<path d="M7 24V20M11.5 24V17M16 24V14M20.5 24V11M25 24V7.5" ${S(2.6)}/>`,
  loop: `<path d="M6 20L18.5 8.5" ${S(2.4)}/><path d="M20.5 6.5L14 8.5L18.5 13Z" ${F()}/><path d="M26 12L13.5 23.5" ${S(2.4)}/><path d="M11.5 25.5L18 23.5L13.5 19Z" ${F()}/>`,
};

// Category → colour and icon: svg = HX-style 32×32 drawing (ICONS32); icon =
// the older 24×24 path, still used by the mixer.
const CATS = {
  Distortion: { color: "#f8932a", /* HX #fc7f30 on screen (Safari shifts orange: #fb8034 showed #ff6d3a) */ svg: ICONS32.distortion, icon: "M2 15h4l2.5-8h3l2 10h3l1.5-5H22" },
  Dynamics: { color: "#dbc239", svg: ICONS32.dynamics, icon: "M3 19c5 0 7-1 9-5s4-9 9-9M3 19h18" },
  "Preamp+EQ": { color: "#dbc239", svg: ICONS32.eq, icon: "M6 4v16M12 4v16M18 4v16M4 14h4M10 8h4M16 12h4" },
  EQ: { color: "#dbc239", svg: ICONS32.eq, icon: "M6 4v16M12 4v16M18 4v16M4 14h4M10 8h4M16 12h4" },
  Modulation: { color: "#0294ec", svg: ICONS32.modulation, icon: "M2 12c2.5-7 5.5-7 8 0s5.5 7 8 0c1-2.8 2-4 4-4" },
  Delay: { color: "#06c535", svg: ICONS32.delay, icon: "M5 12a7 7 0 1 0 14 0a7 7 0 1 0-14 0M9 12a3 3 0 1 0 6 0a3 3 0 1 0-6 0" },
  Reverb: { color: "#fe4924", svg: ICONS32.reverb, icon: "M4 20a16 16 0 0 1 16-16M4 20a11 11 0 0 1 11-11M4 20a6 6 0 0 1 6-6" },
  Pitch: { color: "#a745d6", svg: ICONS32.pitch, icon: "M8 20V5M4 9l4-4 4 4M16 4v15M12 15l4 4 4-4" },
  Filter: { color: "#a745d6", svg: ICONS32.filter, icon: "M2 8h9c4 0 5 3 6 6s2 6 5 6" },
  Wah: { color: "#a142cf", svg: ICONS32.wah, icon: "M3 17l17-7 1 3-17 7zM6 20h14" },
  "Volume/Pan": { color: "#209f8e", svg: ICONS32.volume, icon: "M3 18L21 6v12z" },
  "FX Loop": { color: "#5fb3a0", svg: ICONS32.loop, icon: "M4 12a8 8 0 1 0 3-6.2M4 4v4h4" },
  amp: { color: "#dc0012", svg: ICONS32.amp, fill: "#580005", icon: "M3 5h18v14H3zM3 10h18M6 7.5h.01M9 7.5h.01M12 7.5h.01M15 7.5h.01" },
  preamp: { color: "#dc0012", svg: ICONS32.preamp, fill: "#580005", icon: "" },
  cab: { color: "#dc0012", svg: ICONS32.cab, fill: "#580005", icon: "M4 3h16v18H4zM12 15a4 4 0 1 0 0-.01M12 7.5h.01" },
  mixer: { color: "#c8ccd4", icon: "M6 3v18M12 3v18M18 3v18M4 15h4M10 8h4M16 12h4" },
  none: { color: "#8a8f99", icon: "" },
};
// "None" (empty block) as HX Edit: an X, light grey (#b8b8b8 in the list at 72%).
const NONE_CAT = { color: "#fdfdfd", icon: "M6 6l12 12M18 6L6 18" };
const X_SVG = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" /></svg>';
// The "None" category's only entry: "× (empty block)" (type = the API's none model).
const emptyItem = (type, target, cur) => `<button class="ed-item ed-empty${cur ? " cur" : ""}" type="button" data-type="${type}" data-target="${target}" style="--cat:${NONE_CAT.color}">${X_SVG}<span>(empty block)</span></button>`;
// Parameters panel of an empty block (HX Edit): only the (empty) tab bar.
const EMPTY_BLOCK_HTML = '<div class="ac-tabs"><div class="ac-blank"></div></div>';
const catOf = (it) => (it.kind === "amp" ? (isPreamp(it) ? CATS.preamp : CATS.amp) : it.kind === "cab" ? CATS.cab : CATS[it.group] || CATS.none);

async function fetchFlow() {
  if (flow.fetching) { flow.refetch = true; return; }
  flow.fetching = true;
  try {
    const res = await fetch("/api/preset");
    if (!res.ok) throw await apiError(res);
    flow.view = await res.json();
    if (flow.sel === null) flow.sel = defaultSel();
    // A cab is only shown while its amp exists: amp set to None → select the amp.
    if ((flow.sel === 1 && flow.view.items[0].empty) || (flow.sel === 3 && flow.view.items[2].empty)) flow.sel -= 1;
    renderFlow();
  } catch (e) {
    $("flow-stage").innerHTML = `<span class="flow-empty">${esc(e.message)}</span>`;
  } finally {
    flow.fetching = false;
    if (flow.refetch) { flow.refetch = false; fetchFlow(); }
  }
}

function renderFlowState(st) {
  const online = !!st.connected;
  if (!online) {
    flow.view = null;
    flow.rev = -1;
    $("flow-stage").innerHTML = '<span class="flow-empty">No pedal connected</span>';
    $("flow-detail").textContent = "";
    $("ed-cats").textContent = "";
    $("ed-models").textContent = "";
    return;
  }
  const key = `${st.setlist}:${st.slot}`; // a different preset: drop the selection
  if (key !== flow.presetKey) { flow.presetKey = key; flow.sel = null; flow.cat = null; flow.sub = null; flow.subOpen = false; }
  // Not with the tuner open (the pedal ignores preset reads): flow.rev stays
  // behind, so the preset is fetched as soon as the tuner closes.
  if (st.bufferRev !== flow.rev && !st.tuner?.on) { flow.rev = st.bufferRev; fetchFlow(); }
}

// FX blocks of a lane, in signal order, as the API's chain lists them.
// While an FX block is dragged, flow.preview holds the chain as it would be
// after the drop, so the blocks make room live.
const chainNow = () => flow.preview || flow.view.chain;
const laneOf = (n) => (chainNow()?.lanes[n]?.blocks || []).map((id) => flow.view.items[id]);
const laneName = (n) => chainNow()?.lanes[n]?.name;

// ---- layout: columns on the main row (y1) and, for path B, a lower row (y2)
const COL = 58, PAD = 28, Y1 = 34, Y2 = 100, BOX = 24; // BOX: block frame, snug around the icon (HX)

// The cab is drawn this many columns after its amp: close, since on the LCD
// they are one block, but apart so each can be clicked.
const CAB_GAP = 0.62;
// A preamp's label ("Preamp") is wider: a bit more room, so the "+" between
// the labels doesn't touch it.
const cabGap = (amp) => (isPreamp(amp) ? 0.8 : CAB_GAP);

function layout() {
  const it = flow.view.items;
  const ch = flow.view.chain || {};
  const dragging = !!flow.drag;
  const ampB = !it[2].empty;
  // Where the amp sits (API chain.ampPos): "pre" (before the split), "a" /
  // "b" (on a path), "post" (after the mixer). Two amps: one on each path.
  const pos = ampB ? "a" : ch.ampPos || "a";
  const aPre = laneOf(1), bPre = laneOf(2), aPost = laneOf(3), bPost = laneOf(4);
  // On the paths the pedal always has both amp slots: amp B is shown even as
  // Amp Disabled (empty), so path B is always drawn then.
  const fxDrag = dragging && !flow.drag.amp, ampDrag = dragging && !!flow.drag.amp;
  const dual = fxDrag || ch.dual || pos === "a" || pos === "b" || bPre.length > 0 || bPost.length > 0;
  const nodes = []; // {x, y, it | special | zone}
  let col = 0;
  const at = (c, y, n) => nodes.push({ ...n, x: PAD + c * COL, y });
  // Drop zones (end of each lane, only while dragging) take no column: they
  // are small markers in the gap after the lane's last block, so starting a
  // drag doesn't shift any block.
  const zone = (lane, c, y) => { if (fxDrag) at(c - 0.5, y, { zone: laneName(lane) }); };
  // Free columns of a path row also take a dragged FX block (end of that
  // lane), right where it is shown, so a block starting an empty path is
  // accepted where the eye expects it, not only at the lane's end marker.
  const free = (lane, c0, c1, y) => { if (fxDrag) for (let c = c0; c < c1; c++) at(c, y, { zone: laneName(lane) }); };
  // While the amp is dragged: where it can go (not where it already is).
  const ampZone = (to, c, y) => { if (ampDrag && to !== pos) at(c - 0.5, y, { ampZone: to }); };
  // amp i (0 = A, 2 = B) with its cab right after it (none without an amp)
  const ampW = (i) => (it[i].empty ? 1 : 1 + cabGap(it[i]));
  const amp = (i, c, y) => { at(c, y, { it: it[i] }); if (!it[i].empty) at(c + cabGap(it[i]), y, { it: it[i + 1] }); };

  at(col++, Y1, { special: "in" });
  for (const f of laneOf(0)) at(col++, Y1, { it: f });
  zone(0, col, Y1);
  ampZone("pre", col, Y1);
  if (pos === "pre") { amp(0, col, Y1); col += ampW(0); } // end of the pre section
  const split = col - 0.5;
  let w;
  if (pos === "a" || pos === "b") {
    // Both paths share the amp's columns (as on the LCD): the pre-amp blocks
    // of A and B to its left, the post-amp ones to its right.
    const bp = dual ? bPre : [], bq = dual ? bPost : [];
    const preW = Math.max(aPre.length, bp.length);
    const aw = Math.max(ampW(0), ampW(2));
    const postW = Math.max(aPost.length, bq.length);
    const ampCol = col + preW, postCol = ampCol + aw;
    aPre.forEach((f, i) => at(col + i, Y1, { it: f }));
    aPost.forEach((f, i) => at(postCol + i, Y1, { it: f }));
    bp.forEach((f, i) => at(col + i, Y2, { it: f }));
    bq.forEach((f, i) => at(postCol + i, Y2, { it: f }));
    amp(0, ampCol, Y1);
    amp(2, ampCol, Y2); // Amp Disabled = empty slot
    free(1, col + aPre.length, col + preW, Y1);
    free(3, postCol + aPost.length, postCol + postW, Y1);
    if (dual) {
      free(2, col + bp.length, col + preW, Y2);
      free(4, postCol + bq.length, postCol + postW, Y2);
    }
    zone(1, col + aPre.length, Y1); // end of path A pre (before the amp column)
    zone(3, postCol + aPost.length, Y1); // end of path A
    if (dual) {
      zone(2, col + bp.length, Y2);
      zone(4, postCol + bq.length, Y2);
    }
    w = preW + aw + postW;
  } else {
    // No amp on the paths: each path's first lane from the left, its second
    // lane against the mixer (as the LCD draws them).
    const wA = aPre.length + aPost.length, wB = dual ? bPre.length + bPost.length : 0;
    w = Math.max(wA, wB);
    const W = Math.max(1, w); // while dragging the paths have at least one column
    free(1, col + aPre.length, col + W - aPost.length, Y1);
    if (dual) free(2, col + bPre.length, col + W - bPost.length, Y2);
    aPre.forEach((f, i) => at(col + i, Y1, { it: f }));
    aPost.forEach((f, i) => at(col + w - aPost.length + i, Y1, { it: f }));
    zone(3, col + Math.max(w, 1), Y1);
    if (dual) {
      bPre.forEach((f, i) => at(col + i, Y2, { it: f }));
      bPost.forEach((f, i) => at(col + w - bPost.length + i, Y2, { it: f }));
      zone(4, col + Math.max(w, 1), Y2);
    }
  }
  // Empty paths with the amp in Pre/Post take no column (the cab sits as
  // close to the mixer as on the paths), except while dragging: the drop
  // zones need the room.
  col += w === 0 && !dragging && !dual ? 0 : Math.max(1, w);
  const merge = col;
  ampZone("a", col, Y1); // onto the paths (end of path A)
  ampZone("post", col + 1, Y1);
  at(col++, Y1, { special: "mixer" });
  if (pos === "post") { amp(0, col, Y1); col += ampW(0); } // start of the post section
  for (const f of laneOf(5)) at(col++, Y1, { it: f });
  zone(5, col, Y1);
  at(col++, Y1, { special: "out" });
  return { nodes, dual, split: PAD + split * COL, merge: PAD + merge * COL, width: PAD * 2 + (col - 1) * COL, height: dual ? Y2 + 46 : Y1 + 62 }; // room for "Amp" + a footswitch line
}

// What to open when a preset is loaded: the pedal doesn't report its LCD
// cursor, so the first block in signal order (the first empty slot if the
// preset has no blocks).
function defaultSel() {
  const nodes = layout().nodes.filter((n) => n.it).sort((a, b) => a.x - b.x || a.y - b.y);
  const first = nodes.find((n) => !n.it.empty) || nodes[0];
  return first ? first.it.id : "input";
}

function nodeSVG(n) {
  const x = n.x, y = n.y, h = BOX / 2;
  if (n.ampZone) { // where the dragged amp can go
    const tip = { pre: "Pre, before the split: 1 path (amp B is removed)", a: "On the paths: 2 paths (amp A on path A, amp B on path B)", post: "Post, after the mixer: 1 path (amp B is removed)" }[n.ampZone];
    // an amp-sized dashed box with the amp icon (instead of the FX "+")
    const h = BOX / 2;
    return `<g class="drop-zone amp-zone" data-amp="${n.ampZone}"><title>Move the amp here: ${tip}</title>
      <rect x="${x - h}" y="${y - h}" width="${BOX}" height="${BOX}" rx="3" />
      <svg class="dz-icon" x="${x - 11}" y="${y - 11}" width="22" height="22" viewBox="0 0 32 32">${CATS.amp.svg}</svg>
      <text class="dz-label" x="${x}" y="${y - h - 6}">${{ pre: "1 PATH", a: "2 PATHS", post: "POST" }[n.ampZone]}</text></g>`;
  }
  // End-of-lane targets of an FX drag are not drawn: the live preview shows
  // where the block goes. They still count as targets (drag.snapshotTargets),
  // so an empty path (or a lane end) can still take a block.
  if (n.zone) return "";
  if (n.special === "in") { // the input block: click to edit the inputs
    const src = inputLabel("input1", state && state.input1);
    const ic = inputIcon(src);
    const sel = flow.sel === "input" ? " sel" : "";
    // Input 2 only when it isn't "Same" (value 0): a second, smaller line.
    const src2 = state && state.input2 !== 0 ? inputLabel("input2", state.input2) : "";
    let ty = y + 30;
    let lines = `<text class="io-label" x="${x}" y="${ty}">${esc(inputShortName(src))}</text>`;
    if (src2) { ty += 10; lines += `<text class="io-label2" x="${x}" y="${ty}">${esc(inputShortName(src2))}</text>`; }
    if (state && state.inputsSetup === 1) { ty += 10; lines += `<text class="io-tag" x="${x}" y="${ty}">GLOBAL</text>`; }
    const tip = `Input: ${src}${src2 ? " · Input 2: " + src2 : ""}${state && state.inputsSetup === 1 ? " (Global)" : ""}`;
    return `<g class="blk io io-in${sel}" data-id="input"><title>${esc(tip)}</title>
      <circle class="io-ring" cx="${x}" cy="${y}" r="18" /><circle cx="${x}" cy="${y}" r="14" />
      <svg class="io-icon" x="${x - 10}" y="${y - 10}" width="20" height="20" viewBox="0 0 24 24"><path d="${ic.d}" transform="${ic.tf}" /></svg>
      ${lines}</g>`;
  }
  if (n.special === "out") {
    return `<g class="io"><title>Output</title>
      <circle cx="${x}" cy="${y}" r="14" /><path d="M${x - 7} ${y}h13M${x + 1} ${y - 5}l5 5-5 5" /></g>`;
  }
  if (n.special === "mixer") {
    return blockSVG(x, y, { id: "mixer", kind: "mixer", model: "Mixer", active: true }, CATS.mixer, "Mixer A/B");
  }
  const it = n.it;
  if (it.empty && it.kind === "amp") {
    // No amp (None): keep the amp's place visible, as the LCD does (a full-size
    // dotted outline with a faint amp icon), not a small FX slot.
    const sel = flow.sel === it.id ? " sel" : "";
    return `<g class="blk empty amp-empty${sel}" data-id="${it.id}"><title>None (empty block): click to choose an amp</title>
      <rect class="blk-sel" x="${x - h - 5}" y="${y - h - 5}" width="${BOX + 10}" height="${BOX + 10}" rx="9" />
      <rect class="amp-empty-box" x="${x - h}" y="${y - h}" width="${BOX}" height="${BOX}" rx="3" />
      <svg class="blk-icon ic32" x="${x - 11}" y="${y - 11}" width="22" height="22" viewBox="0 0 32 32">${CATS.amp.svg}</svg>
      <text class="blk-fs blk-ac" x="${x}" y="${y + h + 13}">Amp</text></g>`;
  }
  if (it.kind === "cab") { // always shown after its amp; None = disabled (dim), not removed
    // The cab depends on its amp: it is shown off while the amp is off.
    const amp = flow.view.items[it.id === 1 ? 0 : 2];
    const tip = it.empty ? "Cab: None (disabled)" : `${it.model}${it.basedOn ? " — " + it.basedOn : ""}${amp.active ? "" : " (amp off)"}`;
    return blockSVG(x, y, { ...it, active: !it.empty && amp.active }, CATS.cab, tip);
  }
  if (it.dspOver) { // model loaded but not running (DSP limit): drawn off, "DSP OVER" under it (LCD)
    const tip = `${it.model}: DSP limit reached, not running`;
    return blockSVG(x, y, { ...it, active: false, short: "DSP OVER", assign: "" }, catOf(it), tip).replace('class="blk', 'class="blk dsp-over');
  }
  if (it.empty) {
    const sel = flow.sel === it.id ? " sel" : "";
    return `<g class="blk empty slot-empty${sel}" data-id="${it.id}"><title>${it.kind === "amp" ? "No amp" : "Empty slot: click to choose an effect"}</title>
      <rect class="blk-sel" x="${x - 14}" y="${y - 14}" width="28" height="28" rx="7" />
      <rect class="slot-box" x="${x - 8}" y="${y - 8}" width="16" height="16" rx="4" /></g>`;
  }
  const tip = `${it.model}${it.basedOn ? " — " + it.basedOn : ""}`;
  return blockSVG(x, y, it, catOf(it), tip);
}

function blockSVG(x, y, it, cat, tip) {
  const h = BOX / 2, on = it.active;
  const iz = cat.svg ? 22 : 18; // icon size: HX glyphs 22, the mixer's older drawing 18
  const sel = flow.sel === it.id ? " sel" : "";
  // Amp / Preamp / Cab blocks are labelled under the box (a footswitch label,
  // if any, goes on a second line).
  // FX: the category, abbreviated by the API (Dist, Dyn, EQ / Preamp, Vol / Pan…), as HX Edit
  const kindLabel = it.kind === "amp" ? (isPreamp(it) ? "Preamp" : "Amp") : it.kind === "cab" ? "Cab" : it.kind === "mixer" ? "Mixer" : it.short || "";
  const fsY = y + h + (kindLabel ? 21 : 12); // footswitch: smaller, under the name
  const fs = (kindLabel ? `<text class="blk-fs blk-ac" x="${x}" y="${y + h + 13}">${kindLabel}</text>` : "") +
    // footswitch and expression pedals (API: "FS3", "EXP 1 · Toe"…)
    // an amp's line goes under "Amp + Cab" (centred between the amp and its cab)
    (it.assign ? `<text class="blk-fs blk-fsw" x="${it.kind === "amp" ? x + (cabGap(it) * COL) / 2 : x}" y="${fsY}">${esc(it.assign)}</text>` : "");
  // Hover controls above the block (HX Edit): power (on/off) and × (empty the
  // block: None / Amp Disabled / No Cab). A cab has no on/off; the mixer none.
  const cy = y - h - 9;
  const power = (cx) => `<g class="blk-ctl" data-toggle="1"><title>On/off</title><rect x="${cx - 6}" y="${cy - 6}" width="12" height="12" />
      <path d="M${cx} ${cy - 4.6}v4.2M${cx - 3.3} ${cy - 2.7}a4.5 4.5 0 1 0 6.6 0" /></g>`;
  const remove = (cx) => `<g class="blk-ctl" data-remove="1"><title>Remove (None)</title><rect x="${cx - 6}" y="${cy - 6}" width="12" height="12" />
      <path d="M${cx - 3} ${cy - 3}l6 6M${cx + 3} ${cy - 3}l-6 6" /></g>`;
  const led = it.kind === "mixer" ? "" : `<g class="blk-hover"><rect class="blk-hover-hit" x="${x - h}" y="${cy - 7}" width="${BOX}" height="16" />${
    it.kind === "cab" || it.dspOver ? remove(x) : power(x - 7) + remove(x + 7)}</g>`; // DSP over: only × (delete)
  return `<g class="blk${on ? " on" : ""}${sel}" data-id="${it.id}" style="--cat:${cat.color}">
    <title>${esc(tip)}</title>
    <rect class="blk-sel" x="${x - h - 5}" y="${y - h - 5}" width="${BOX + 10}" height="${BOX + 10}" rx="9" />
    <rect class="blk-box" x="${x - h}" y="${y - h}" width="${BOX}" height="${BOX}" rx="3" />
    <svg class="blk-icon${catGlyph(cat).cls}" x="${x - iz / 2}" y="${y - iz / 2}" width="${iz}" height="${iz}" viewBox="${catGlyph(cat).vb}">${catGlyph(cat).inner}</svg>
    ${led}${fs}</g>`;
}

function renderFlow() {
  if (!flow.view) return;
  const L = layout();
  const r = 12; // corner radius of the path B branch
  let wires = `<path d="M${PAD} ${Y1}H${L.width - PAD}" />`;
  if (L.dual) {
    wires += `<path d="M${L.split} ${Y1}V${Y2 - r}q0 ${r} ${r} ${r}H${L.merge - COL / 2 - r}q${r} 0 ${r} -${r}V${Y1}" />`;
  }
  let links = "";
  // The wire between an amp and its cab in the amp's colour (as their
  // frames), to show they are one unit; plain when the amp is off.
  for (const [ai, ci] of [[0, 1], [2, 3]]) {
    const na = L.nodes.find((n) => n.it && n.it.id === ai), nc = L.nodes.find((n) => n.it && n.it.id === ci);
    if (!na || !nc || na.it.empty) continue;
    const linked = !nc.it.empty && na.it.active;
    // "+" between the "Amp" and "Cab" labels, coloured like the link
    const ampHalf = isPreamp(na.it) ? 16 : 9, cabHalf = 9; // half label widths (8.5px bold)
    links += `<text class="amp-plus${linked ? " on" : ""}" x="${(na.x + ampHalf + nc.x - cabHalf) / 2}" y="${na.y + BOX / 2 + 13}">+</text>`;
    if (!linked) continue;
    wires += `<path class="amp-link" style="stroke:${CATS.amp.color}" d="M${na.x + BOX / 2} ${na.y}H${nc.x - BOX / 2}" />`;
  }
  const svg = `<svg class="chain" viewBox="0 0 ${L.width} ${L.height}" style="max-width:${L.width * 1.5}px" role="img" aria-label="Block chain">
    <g class="wires">${wires}</g>${L.nodes.map(nodeSVG).join("")}${links}</svg>`;
  $("flow-stage").innerHTML = svg;
  renderEditor();
}

// Selection changes only classes: rebuilding the SVG between the two clicks
// of a double-click would swallow the dblclick.
function selectBlock(id) {
  if (flow.sel !== id) { flow.cat = null; flow.sub = null; flow.subOpen = false; }
  flow.sel = id;
  for (const g of $("flow-stage").querySelectorAll(".blk")) {
    g.classList.toggle("sel", String(g.dataset.id) === String(id));
  }
  renderEditor();
}

// ---- parameter values (all stored normalized 0..1)
// The pedal rounds for display (Gain 0.8753 shows 88, Low Cut 344.6 Hz 345).
const pct = (v) => Math.round(v * 100);
const listIndex = (p) => Math.round(p.value * Math.max(1, p.options.length - 1));

// ▲▼ / wheel steps of a range in its unit: 0.1 dB, 1 ms, 1 Hz, 0.1 kHz (or
// the catalog step); 0 = the default 1 %.
function rangeSteps(p) {
  const lo = p.min ?? 0, hi = p.max ?? 0;
  if (p.kind !== "range" || !(hi > lo) || !p.unit) return 0;
  const st = p.step || (p.unit === "dB" || p.unit === "kHz" ? 0.1 : 1);
  return Math.round((hi - lo) / st) + 1;
}

function fmtParam(p) {
  const v = p.value;
  if (!Number.isFinite(v)) return "—";
  if (p.kind === "list" && p.options && p.options.length) return p.options[listIndex(p)] ?? "—";
  const lo = p.min ?? 0, hi = p.max ?? 0; // the API omits a 0 bound
  if (p.kind === "range" && hi > lo) {
    const x = lo + v * (hi - lo);
    // dB with one decimal (EQs "-11.5 dB"); a range with a step shows it in
    // those steps (Hard Gate: 1 dB, 0.32 of -96..0 = "-65.0")
    // the pedal steps 0.1 dB: Studio EQ Gain sends exactly k/360 of -18..18
    // (front-panel log 2026-11-08); shown rounded to 0.1, or to the catalog step
    // without a step, dB and kHz are cut toward zero like the LCD (webcam
    // check 2026-11-08: Studio EQ -2.86 → "-2.8", -4.68 → "-4.6"; Vintage Pre
    // LPF 10.55 → "10.5 kHz"); Hz rounds (197.6 → "198 Hz")
    const cut = (u) => Math.trunc(u * 10 + Math.sign(u) * 1e-6) / 10 || 0;
    const xs = p.step ? (Math.trunc(x / p.step + Math.sign(x) * 1e-6) * p.step || 0) : // Hard Gate -35.52 → "-35.0" (LCD)
      p.unit === "dB" || p.unit === "kHz" ? cut(x) : x;
    const n = p.unit === "dB" ? xs.toFixed(p.whole ? 0 : 1) : p.unit === "ms" || p.unit === "Hz" ? String(Math.round(x)) : p.unit === "kHz" ? x.toFixed(1) : Math.abs(x) >= 100 ? x.toFixed(0) : x.toFixed(1);
    // dB above 0 with a "+" ("+3.5 dB"), as the mixer shows it
    // also signed ranges without a unit (Pitch Glide "+6.2", as the LCD)
    const sign = (p.unit === "dB" || (!p.unit && lo < 0)) && Number(n) > 0 ? "+" : "";
    return p.unit ? `${sign}${n} ${p.unit}` : n; // the unit from the API ("-96.0 dB")
  }
  return `${pct(v)} %`; // HX Edit: a space before %
}

// "+3.5 dB", "MUTE" at the minimum; pan "< 50 %" (HX: a space before %),
// "50 % >", centre "< 0 % >".
const fmtDB = (x) => (x <= MIXER_MIN + 1e-3 ? "MUTE" : `${x > 0 ? "+" : ""}${x.toFixed(1)} dB`);
const fmtPan = (x) => {
  const p = Math.round(Math.abs(x) * 100);
  return p === 0 ? "< 0 % >" : x < 0 ? `< ${p} %` : `${p} % >`;
};

// One parameter row: name, value and a slider. With a key, the slider is
// editable through the descriptor flow.sliders[key]:
//   { value 0..1, steps (options for lists, else 0), fmt(v), send(v) }.
function prmRow(name, value, frac, extra = "", key = "", after = "") {
  const f = Math.max(0, Math.min(1, frac));
  const ro = key ? "" : " ro";
  const focus = key ? ` data-s="${esc(key)}" tabindex="0" role="slider" aria-valuemin="0" aria-valuemax="100"` : "";
  // Up/down steppers next to the value (one step: an option, 0.5 dB, 1%…).
  // HX: 7×4 filled triangles, 6px apart; the buttons' hit area is larger (CSS).
  const arrow = (dir, label, d) => `<button type="button" data-step="${dir}" data-for="${esc(key)}" aria-label="${label} ${esc(name)}">` +
    `<svg viewBox="0 0 7 4" aria-hidden="true"><path d="${d}" /></svg></button>`;
  const steps = key ? `<span class="prm-step">${arrow(1, "Increase", "M0 4L3.5 0L7 4Z")}${arrow(-1, "Decrease", "M0 0L3.5 4L7 0Z")}</span>` : "";
  // Double-click the value to type one (HX Edit): see editValue().
  // A list (options, note values…): the value opens a dropdown, as HX Edit ("1/16 Dotted ▾")
  const dd = key && flow.sliders[key] && flow.sliders[key].options;
  const val = dd ? `<span class="prm-val prm-dd" data-dd="${esc(key)}">${esc(value)}<svg viewBox="0 0 8 5" aria-hidden="true"><path d="M0 0h8L4 5z" /></svg></span>`
    : key ? `<span class="prm-val" data-edit="${esc(key)}">${esc(value)}</span>` : `<span class="prm-val">${esc(value)}</span>`;
  return `<div class="prm${ro}"><span class="prm-name">${esc(name)}${extra}</span><span class="prm-valbox">${val}${steps}${after}</span>
    <span class="prm-bar"${focus} aria-label="${esc(name)}" aria-valuenow="${Math.round(f * 100)}" style="--v:${f}"><i></i><b></b></span></div>`;
}

function slider(key, name, d, extra = "", after = "") {
  flow.sliders[key] = d;
  return prmRow(name, d.fmt(d.value), d.value, extra, key, after);
}

// Tempo parameters (delay Time in ms, modulation/filter Speed in Hz) as HX
// Edit: the value in its unit, or — when synced to the tap tempo — the note
// value, stepping through the pedal's note list; the clock button toggles.
const CLOCK_SVG = '<svg viewBox="0 0 14 14" aria-hidden="true"><circle cx="7" cy="7" r="5.5" /><path d="M7 3.8V7l2.2 1.4" /></svg>';
const fmtTempo = (p, v) => {
  const x = p.min + v * (p.max - p.min);
  return p.max > 100 ? `${Math.round(x)} ms` : `${x.toFixed(2)} Hz`;
};
function tempoSlider(blockId, k, p, extra) {
  const notes = (meta && meta.syncNotes) || [];
  const n = notes.length - 2; // note codes 2..
  const key = `b${blockId}.${k}`;
  const clock = `<button type="button" class="prm-sync${p.sync ? " on" : ""}" data-sync="${key}" aria-pressed="${!!p.sync}"
    title="${p.sync ? `Synced to tempo (click for ${p.max > 100 ? "ms" : "Hz"})` : "Sync to tempo"}">${CLOCK_SVG}</button>`;
  if (p.sync >= 2 && n > 1) {
    // HX: a synced time reads "Note Sync" ("Left Time" → "Left Note Sync")
    const syncName = /Time$/.test(p.name) ? p.name.replace(/Time$/, "Note Sync") : p.name;
    return slider(key, syncName, {
      value: (p.sync - 2) / (n - 1),
      steps: n, options: notes.slice(2, 2 + n),
      fmt: (f) => notes[2 + Math.round(f * (n - 1))] ?? "—",
      send: (f) => { p.sync = 2 + Math.round(f * (n - 1)); queueSend("/api/param/sync", { id: blockId, slot: p.tempoSlot, sync: p.sync }); },
    }, extra, clock);
  }
  const unit = p.max > 100 ? 1 : 0.01; // arrows: 1 ms or 0.01 Hz
  return slider(key, p.name, {
    value: p.value,
    steps: Math.round((p.max - p.min) / unit) + 1,
    fmt: (v) => fmtTempo(p, v),
    send: (v) => { p.value = v; queueSend("/api/param", { id: blockId, param: p.id, value: v }); },
  }, extra, clock);
}

// Two-option parameters (Dimension SW1–SW4 OFF/ON, Rotary Slow/Fast, Phase
// 0/180…) as HX Edit: a two-button switch instead of a fader. The value is
// the option index normalized like any list (0 or 1).
function toggleRow(blockId, k, p, extra) {
  const key = `b${blockId}.${k}`;
  const idx = listIndex(p);
  const n = p.options.length; // 2 to 4 buttons (Particle Verb Condition; Multi-Head Heads)
  flow.sliders[key] = {
    value: idx, steps: n, fmt: (i) => p.options[Math.round(i)] ?? "—",
    send: (i) => { const v = i / (n - 1); p.value = v; queueSend("/api/param", { id: blockId, param: p.id, value: v }); },
  };
  // "SW1 OFF" → "OFF": the switch name is already the row's label
  const label = (o) => o.replace(/^SW\d+\s+/i, ""); // as the option reads (HX: "Chorus", "OFF")
  const btns = p.options.map((o, i) => `<button type="button" class="${i === idx ? "cur" : ""}" data-switch="${key}" data-i="${i}" aria-pressed="${i === idx}">${esc(label(o))}</button>`).join("");
  return `<div class="prm prm-switch"><span class="prm-name">${esc(p.name)}${extra}</span><span class="seg" role="group" aria-label="${esc(p.name)}">${btns}</span></div>`;
}

// The clock: free → the note nearest to the current time/speed at the
// current BPM; synced → back to free (the stored time/speed is kept).
function toggleSync(key) {
  const [b, k] = key.slice(1).split(".").map(Number);
  const p = flow.view?.items[b]?.params?.[k];
  if (!p || !p.tempoSlot) return;
  let note = 0;
  if (!p.sync) {
    const beatsOf = (meta && meta.syncBeats) || [];
    const bpm = (state && state.tempo) || 120;
    const x = p.min + p.value * (p.max - p.min);
    const beats = p.max > 100 ? (x * bpm) / 60000 : bpm / 60 / x; // ms → beats; Hz → beats per cycle
    let best = Infinity;
    beatsOf.forEach((bt, i) => {
      if (i < 2 || !bt) return;
      const d = Math.abs(Math.log(beats / bt));
      if (d < best) { best = d; note = i; }
    });
  }
  p.sync = note;
  renderParams();
  queueSend("/api/param/sync", { id: b, slot: p.tempoSlot, sync: note });
}

function paramsHTML(blockId, params) {
  if (!params || !params.length) return '<span class="dt-meta">No parameters.</span>';
  // HX Edit: if any parameter has a tempo-sync clock, every row narrows
  // and the clocks get a column of their own on the right.
  const sync = params.some((p) => p.kind === "tempo" && p.tempoSlot);
  return `<div class="params${sync ? " has-sync" : ""}">${params.map((p, k) => {
    const ctl = p.ctl ? ` <span class="prm-ctl" title="Controlled by expression pedal ${p.ctl}">EXP${p.ctl}</span>` : "";
    if (p.kind === "tempo" && p.tempoSlot) return tempoSlider(blockId, k, p, ctl);
    if (p.kind === "list" && p.options?.length >= 2 && p.options.length <= 4) return toggleRow(blockId, k, p, ctl); // HX: buttons, not a fader (Multi-Head: 4)
    return slider(`b${blockId}.${k}`, p.name, {
      value: p.value,
      steps: p.kind === "list" && p.options ? p.options.length : rangeSteps(p), options: p.kind === "list" ? p.options : undefined,
      fmt: (v) => fmtParam({ ...p, value: v }),
      send: (v) => { p.value = v; queueSend("/api/param", { id: blockId, param: p.id, value: v }); },
    }, ctl);
  }).join("")}</div>`;
}

// Cab of path A (0) or B (1): mic and early reflections (preset settings),
// then low cut, res level, thump and decay (the cab block's parameters).
function cabParamsHTML(cabIdx) {
  const v = flow.view;
  const cab = v.items[cabIdx === 0 ? 1 : 3];
  const cv = v.cabs[cabIdx];
  const mics = (meta && meta.mics) || [];
  const set = (param) => (x) => queueSend("/api/cab", { cab: cabIdx, param, value: x });
  const rows = [
    slider(`c${cabIdx}.mic`, "Mic", {
      value: mics.length > 1 ? cv.mic / (mics.length - 1) : 0,
      steps: mics.length, options: mics,
      fmt: (x) => mics[Math.round(x * (mics.length - 1))] ?? "—",
      send: (x) => { cv.mic = Math.round(x * (mics.length - 1)); set("mic")(cv.mic); },
    }),
    slider(`c${cabIdx}.er`, "Early Reflections", {
      value: cv.er, steps: 0, fmt: (x) => `${pct(x)} %`,
      send: (x) => { cv.er = x; set("er")(x); },
    }),
  ];
  const names = ["lowcut", "res", "thump", "decay"];
  (cab.params || []).slice(0, 4).forEach((p, k) => {
    rows.push(slider(`c${cabIdx}.${names[k]}`, p.name, {
      value: p.value, steps: 0,
      fmt: (x) => (k === 0 ? `${Math.round(20 + x * 480)} Hz` : `${pct(x)} %`),
      send: (x) => { p.value = x; set(names[k])(x); },
    }));
  });
  return `<div class="params">${rows.join("")}</div>`;
}

// Mixer: level in dB (MIXER_MIN..+12, 0.5 dB steps) and pan -1..+1 (0.01 steps),
// stored as such (factory presets).
const MIXER_MIN = -60, MIXER_MAX = 12;
function mixerRow(m, key, name) {
  const level = key.endsWith("Level");
  const [lo, hi, step] = level ? [MIXER_MIN, MIXER_MAX, 0.5] : [-1, 1, 0.01];
  const toReal = (f) => Math.round((lo + f * (hi - lo)) / step) * step;
  return slider(`mix.${key}`, name, {
    value: (m[key] - lo) / (hi - lo),
    steps: Math.round((hi - lo) / step) + 1,
    fmt: (f) => (level ? fmtDB(toReal(f)) : fmtPan(toReal(f))),
    send: (f) => { m[key] = toReal(f); queueSend("/api/mixer", { param: key, value: m[key] }); },
  });
}

// A category glyph: the HX-style 32×32 drawing when there is one (class ic32:
// coloured through `color`), else the older 24×24 path.
const catGlyph = (cat) => (cat.svg ? { vb: "0 0 32 32", inner: cat.svg, cls: " ic32" } : { vb: "0 0 24 24", inner: `<path d="${cat.icon}" />`, cls: "" });
function iconSVG(cat, cls = "dt-icon") {
  const g = catGlyph(cat);
  return `<svg class="${cls}${g.cls}" viewBox="${g.vb}" style="--cat:${cat.color}">${g.inner}</svg>`;
}

// ---- the editor: categories · models · parameters
function selectedItem() {
  const v = flow.view;
  return v && typeof flow.sel === "number" ? v.items[flow.sel] : null;
}

// Categories offered for the selected block and the one it belongs to.
// Amp and cab are separate blocks (the cab always right after its amp):
//   amp: None (disables the amp), Amp, Preamp
//   cab: None ("No Cab": the cab block stays, disabled), Cab
function editorCats(it) {
  if (it.kind === "amp") {
    const list = [{ name: "None", group: "Amps" }, { name: "Amp", group: "Amps" }, { name: "Preamp", group: "Preamps" }];
    return { list, own: it.empty ? "None" : isPreamp(it) ? "Preamp" : "Amp" };
  }
  if (it.kind === "cab") {
    const list = [{ name: "None", group: "Cabs" }, { name: "Cab", group: "Cabs" }];
    return { list, own: it.empty ? "None" : "Cab" };
  }
  const list = [{ name: "None", kind: "fx", models: [] }, ...flow.models.filter((g) => g.kind === "fx")];
  return { list, own: it.empty ? "None" : it.group };
}

function catStyle(name) {
  if (name === "Preamp" || name === "Preamps") return CATS.preamp;
  if (name === "Amp" || name === "Amps") return CATS.amp;
  if (name === "Cab" || name === "Cabs") return CATS.cab;
  return CATS[name] || CATS.none;
}

function renderEditor() {
  const catsEl = $("ed-cats"), modelsEl = $("ed-models");
  const v = flow.view;
  if (!v || flow.sel === null) {
    catsEl.innerHTML = '<span class="ed-hint">Select a block or an empty slot in the chain.</span>';
    modelsEl.innerHTML = "";
    renderParams();
    return;
  }
  if (flow.sel === "input") { // the input block: one "Input" entry, Input 1 sources in the middle
    const grid = '<svg viewBox="0 0 16 16" aria-hidden="true">' +
      [1, 6.3, 11.6].flatMap((y) => [1, 6.3, 11.6].map((x) => `<rect x="${x}" y="${y}" width="3.4" height="3.4" rx=".6" />`)).join("") + "</svg>";
    // same column headers as the other blocks (HX): "Input" | "Input 1" (the sources listed)
    catsEl.innerHTML = `<div class="ed-colhead ed-cathead ed-inhead" style="--cat:#f2f3f5">${INPUT_SVG}<span>Input</span><button class="ed-grid" type="button" aria-label="Grid view" title="Grid view">${grid}</button></div>
      <button class="ed-item cur" type="button" style="--cat:#f2f3f5">${INPUT_SVG}<span>Input</span></button>`;
    const cur = state ? state.input1 : null;
    modelsEl.innerHTML = `<div class="ed-colhead ed-colhead-dark" title="Input 1 Source"><span>Input 1</span></div>` +
      ((meta && meta.input1) || []).map((o) => {
        const ic = inputIcon(o.label);
        return `<button class="ed-item${o.value === cur ? " cur" : ""}" type="button" data-src="${o.value}" style="--cat:#f2f3f5">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="${ic.d}" transform="${ic.tf}" /></svg><span>${esc(o.label)}</span></button>`;
      }).join("");
    renderParams();
    return;
  }
  if (flow.sel === "mixer") {
    // same column headers as the other blocks
    catsEl.innerHTML = catHead(CATS.mixer, "Mixer") + `<button class="ed-item cur" type="button" style="--cat:${CATS.mixer.color}">${iconSVG(CATS.mixer, "")}<span>Mixer</span></button>`;
    modelsEl.innerHTML = `<div class="ed-colhead ed-colhead-dark"><span>Mixer</span></div><button class="ed-item cur" type="button" style="--cat:${CATS.mixer.color}">${iconSVG(CATS.mixer, "")}<span>Mixer</span></button>`;
    renderParams();
    return;
  }
  const it = selectedItem();
  const { list, own } = editorCats(it);
  const show = list.some((g) => g.name === flow.cat) ? flow.cat : own;
  catsEl.innerHTML = catHead(show === "None" ? NONE_CAT : catStyle(show), show) + list.map((g) => {
    const st = g.name === "None" ? NONE_CAT : catStyle(g.name);
    return `<button class="ed-item${g.name === show ? " cur" : ""}" type="button" data-cat="${esc(g.name)}" style="--cat:${st.color}">
      ${g.name === "None" ? X_SVG : iconSVG(st, "")}<span>${esc(g.name)}</span></button>`;
  }).join("");
  if (it.kind === "amp" || it.kind === "cab") renderAmpCabModels(it, list, show, modelsEl);
  else renderFxModels(it, list, show, modelsEl);
  modelsEl.querySelector(".ed-item.cur")?.scrollIntoView({ block: "nearest" });
  renderParams();
}

function renderFxModels(it, list, show, modelsEl) {
  const grp = list.find((g) => g.name === show);
  if (show === "None") {
    modelsEl.innerHTML = `<div class="ed-colhead ed-colhead-blank"></div>` + emptyItem(0x020dffff, it.id, it.empty);
  } else if (grp) {
    modelsEl.innerHTML = `<div class="ed-colhead"><span>${esc(show)}</span></div>` + grp.models.map((m) =>
      `<button class="ed-item${!it.empty && it.type === m.type ? " cur" : ""}" type="button" data-type="${m.type}" data-target="${it.id}"
        title="${esc(m.basedOn || m.name)}" style="--cat:${catStyle(show).color}"><span class="dot"></span><span>${esc(m.name)}</span></button>`).join("");
  } else {
    modelsEl.innerHTML = "";
  }
}

// Amp / Preamp / Cab models. None: only "(empty block)" — the API's none
// model of the group ("Amp Disabled" for amps, "No Cab" for cabs). Otherwise
// a Guitar / Bass dropdown (the API's "bass" flag; default: the current
// model's) over the list.
function renderAmpCabModels(it, list, show, modelsEl) {
  const group = list.find((g) => g.name === show).group;
  const models = flow.models.find((g) => g.name === group)?.models || [];
  if (show === "None") {
    const off = models.find((m) => m.none);
    modelsEl.innerHTML = `<div class="ed-colhead ed-colhead-blank"></div>` + (off ? emptyItem(off.type, it.id, it.empty) : "");
    return;
  }
  const color = catStyle(show).color;
  const curBass = !it.empty && !!models.find((m) => m.type === it.type)?.bass;
  const sub = flow.sub || (curBass ? "bass" : "guitar");
  const shown = models.filter((m) => !m.none && !!m.bass === (sub === "bass"));
  modelsEl.innerHTML = subHead(sub, show) + shown.map((m) =>
    `<button class="ed-item${!it.empty && it.type === m.type ? " cur" : ""}" type="button" data-type="${m.type}" data-target="${it.id}"
      title="${esc(m.basedOn || m.name)}" style="--cat:${color}">${m.style ? ampIconSVG(m.style) : '<span class="dot"></span>'}<span>${esc(m.name)}</span></button>`).join("");
}

// ---- amp thumbnails: original drawings built from the API's style (data/
// gear-styles.json): combo or head, tolex, control panel with its knobs and
// the grille cloth's texture. No logos, no copied art.
const KNOB_COLORS = { white: "#f0f0f0", black: "#141414", chrome: "#c9ccd1", cream: "#eadfbf" };
let ampIconSeq = 0;
// shade("#rrggbb", f): f < 1 darker, f > 1 lighter.
function shade(hex, f) {
  const n = parseInt(String(hex).replace("#", ""), 16);
  if (Number.isNaN(n)) return hex;
  const c = (v) => Math.max(0, Math.min(255, Math.round(v * f)));
  return `rgb(${c(n >> 16)},${c((n >> 8) & 255)},${c(n & 255)})`;
}
// Grille cloth textures (pattern in a 3×3 tile), by gear-styles grilleType.
function grillePattern(id, type, base) {
  const ink = shade(base, 0.62), hi = shade(base, 1.25);
  // inline styles: the list CSS restyles every <path> in an .ed-item
  const ln = (d, color, w) => `<path d="${d}" style="fill:none;stroke:${color};stroke-width:${w}"/>`;
  const lines = {
    tweed: ln("M0 3L3 0M-1 1L1 -1M2 4L4 2", ink, 0.7),
    silver: ln("M0 .75H3M0 2.25H3", hi, 0.55),
    diamond: ln("M0 0L3 3M3 0L0 3", ink, 0.5),
    "diamond-plate": `<path d="M1.5 .4L2.6 1.5L1.5 2.6L.4 1.5Z" style="fill:${hi};stroke:none"/>`,
    basket: ln("M0 0L3 3M3 0L0 3", ink, 0.9),
  }[type] || ln("M0 1.5H3", ink, 0.5);
  return `<pattern id="${id}" width="3" height="3" patternUnits="userSpaceOnUse"><rect width="3" height="3" fill="${base}"/>${lines}</pattern>`;
}
function ampIconSVG(s, cls = "amp-ico") {
  const id = `ag${++ampIconSeq}`;
  const tolex = s.tolex || "#2a2a2a", panel = s.panel || "#202020", grille = s.grille || "#3a3a3a";
  const trim = s.trim || shade(tolex, 0.6), knob = KNOB_COLORS[s.knob] || KNOB_COLORS.chrome;
  // knobs get a contrasting rim so they show on any panel colour
  const rim = s.knob === "black" ? "rgba(255,255,255,.4)" : "rgba(0,0,0,.55)";
  const knobs = (y, x0, x1, n, r) => Array.from({ length: n }, (_, i) =>
    `<circle cx="${(x0 + ((x1 - x0) * i) / (n - 1)).toFixed(2)}" cy="${y}" r="${r}" fill="${knob}" stroke="${rim}" stroke-width=".35"/>`).join("");
  let body;
  if (s.layout === "head") { // short, wide box with a handle; front = control panel + cloth band
    body = `<rect x="11" y="1.6" width="8" height="1.6" rx=".8" fill="${shade(tolex, 0.45)}"/>
      <rect x="1" y="3.6" width="28" height="13" rx="1.4" fill="${tolex}" stroke="${trim}" stroke-width=".6"/>
      <rect x="2.6" y="5.2" width="24.8" height="4" fill="url(#${id})"/>
      <rect x="2.6" y="10" width="24.8" height="5" fill="${panel}"/>${knobs(12.5, 6, 24, 6, 0.85)}`;
  } else { // combo: panel on top, speaker cloth below
    body = `<rect x="1" y="1" width="28" height="18" rx="1.6" fill="${tolex}" stroke="${trim}" stroke-width=".6"/>
      <rect x="2.6" y="2.6" width="24.8" height="4.2" fill="${panel}"/>${knobs(4.7, 6.5, 23.5, 5, 0.85)}
      <rect x="2.6" y="7.8" width="24.8" height="9.6" fill="url(#${id})" stroke="${trim}" stroke-width=".4"/>`;
  }
  return `<svg class="${cls}" viewBox="0 0 30 20" aria-hidden="true"><defs>${grillePattern(id, s.grilleType, grille)}</defs>${body}</svg>`;
}

// Column headers as HX Edit (25px, #191919). Categories: the current one in
// its colour. Models (amp / preamp / cab): a Guitar / Bass dropdown.
function catHead(cat, name) {
  return `<div class="ed-colhead ed-cathead" style="--cat:${cat.color}">${iconSVG(cat, "")}<span>${esc(name)}</span></div>`;
}
function subHead(sub, word) {
  const opts = [["guitar", `Guitar ${word}`], ["bass", `Bass ${word}`]];
  const label = opts.find(([s]) => s === sub)[1];
  const menu = flow.subOpen ? `<div class="ed-submenu" role="listbox">${opts.map(([s, l]) => {
    const on = s === sub;
    return `<button class="ed-subopt${on ? " cur" : ""}" type="button" role="option" aria-selected="${on}" data-sub="${s}">${l}</button>`;
  }).join("")}</div>` : "";
  return `<div class="ed-colhead ed-subhead"><button class="ed-subbtn" type="button" data-subtoggle aria-haspopup="listbox" aria-expanded="${!!flow.subOpen}">
    <span>${label}</span><svg viewBox="0 0 12 8" aria-hidden="true"><path d="M1.5 1.5 6 6l4.5-4.5" /></svg></button>${menu}</div>`;
}

// A preamp model (the API's "Preamps" group) rather than a full amp.
function isPreamp(it) {
  if (!it || it.empty) return false;
  const g = flow.models && flow.models.find((x) => x.name === "Preamps");
  return g ? g.models.some((m) => !m.none && m.type === it.type) : / Preamp$/.test(it.model);
}

// Block header as HX Edit: one bar with the model (icon + name, left) and
// the power button; details (where, footswitch, based on) in its tooltip.
function blockHead(it, cat, tip, power = true) {
  const pw = power ? `<button class="power${it.active ? " on" : ""}" type="button" data-toggle-id="${it.id}" title="${it.active ? "Turn off" : "Turn on"} (Space)">
      <svg viewBox="0 0 24 24"><path d="M12 3v9M6.3 6.3a8 8 0 1 0 11.4 0" /></svg></button>` : "";
  return `<div class="ac-tabs"><div class="ac-title" style="--cat:${cat.color}" title="${esc(tip)}">${iconSVG(cat, "")}<span>${esc(it.model)}</span></div>${pw}</div>`;
}
const tipOf = (it, where) => [it.model, where, FS_LABEL[it.fs] ? `footswitch ${FS_LABEL[it.fs]}` : "", it.basedOn ? `based on ${it.basedOn}` : ""].filter(Boolean).join(" · ");

function renderParams() {
  renderParamsPage();
  renderAuto();
  if (typeof renderMidi === "function") renderMidi();
}

function renderParamsPage() {
  if (flow.dragging) return; // rebuilt when the drag ends
  flow.sliders = {};
  const d = $("flow-detail");
  const v = flow.view;
  d.style.removeProperty("--cat");
  d.style.removeProperty("--cat-fill");
  if (!v || flow.sel === null) {
    d.innerHTML = '<span class="dt-meta">The parameters of the selected block appear here.</span>';
    return;
  }
  if (flow.sel === "input") {
    d.style.setProperty("--cat", "#c9cbd0"); // neutral, like the input block of the HX
    d.innerHTML = inputParamsHTML();
    return;
  }
  if (flow.sel === "mixer") {
    const m = v.mixer; // level −60..+12 dB, pan −1..+1
    d.style.setProperty("--cat", CATS.mixer.color);
    d.innerHTML = `<div class="ac-tabs"><div class="ac-title" style="--cat:${CATS.mixer.color}">${iconSVG(CATS.mixer, "")}<span>Mixer</span></div></div>
      <div class="params">${mixerRow(m, "aLevel", "Volume A")}${mixerRow(m, "aPan", "Pan A")}${mixerRow(m, "bLevel", "Volume B")}${mixerRow(m, "bPan", "Pan B")}</div>`;
    return;
  }
  const it = selectedItem();
  if (it && it.dspOver) { // the pedal couldn't run this model
    d.style.setProperty("--cat", catOf(it).color);
    d.innerHTML = blockHead(it, catOf(it), it.model, false) +
      '<div class="dsp-note"><b>DSP LIMIT REACHED</b><span>This block is not running. Reduce usage to free up DSP resources.</span></div>';
    return;
  }
  if (!it || it.empty) { // HX: an empty block shows just the blank header bar
    d.innerHTML = EMPTY_BLOCK_HTML;
    return;
  }
  const cat = catOf(it);
  d.style.setProperty("--cat", cat.color);
  if (cat.fill) d.style.setProperty("--cat-fill", cat.fill); // display-calibrated track fill
  if (it.kind === "amp") {
    d.innerHTML = blockHead(it, cat, tipOf(it, it.id === 0 ? "Amp A" : "Amp B")) + paramsHTML(it.id, it.params);
  } else if (it.kind === "cab") { // no on/off: a cab is on while it has a model
    const cabIdx = it.id === 1 ? 0 : 1;
    d.innerHTML = blockHead(it, cat, tipOf(it, cabIdx === 0 ? "Cab A" : "Cab B"), false) + cabParamsHTML(cabIdx);
  } else {
    d.innerHTML = blockHead(it, cat, tipOf(it, `${it.label} · ${LANE_LABEL[it.lane] || ""}`)) + paramsHTML(it.id, it.params);
  }
}

// × on a block: load its "none" model — None for FX, the API's Amp Disabled /
// No Cab for the amp and cab (the cab block stays, disabled).
function removeBlock(id) {
  const it = flow.view && flow.view.items[id];
  if (!it || (it.empty && !it.dspOver)) return; // a DSP-over block can be deleted
  let type = 0x020dffff; // FX: None
  if (it.kind === "amp" || it.kind === "cab") {
    const g = flow.models.find((x) => x.name === (it.kind === "amp" ? "Amps" : "Cabs"));
    const none = g && g.models.find((m) => m.none);
    if (!none) return;
    type = none.type;
  }
  setModel(id, type);
}

async function toggleBlock(id) {
  if (flow.view && flow.view.items[id] && flow.view.items[id].dspOver) return; // not running: no on/off
  const it = flow.view && flow.view.items[id];
  if (!it || it.empty || it.kind === "cab") return; // a cab has no on/off (None disables it)
  const active = !it.active;
  it.active = active; // optimistic; the server bumps bufferRev and we re-read
  renderFlow();
  try {
    const res = await fetch("/api/block", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id, active }),
    });
    if (!res.ok) throw await apiError(res);
  } catch (e) {
    it.active = !active;
    renderFlow();
    toast(`Couldn't turn ${active ? "on" : "off"} ${it.model}: ${e.message}`);
  }
}

async function setModel(target, type) {
  const ed = document.querySelector(".editors");
  ed.classList.add("ed-busy");
  try {
    const res = await fetch("/api/block/model", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id: target, type }),
    });
    if (!res.ok) throw await apiError(res);
    // Keep the category on screen (e.g. Cabs after changing the cab).
    await fetchFlow(); // the pedal filled in the model's defaults
  } catch (e) {
    toast(`Couldn't change the model: ${e.message}`);
  } finally {
    ed.classList.remove("ed-busy");
  }
}

// ---- sliders: drag / arrows send the newest value, one request at a time
const sender = { busy: false, next: null };

// ---- BYPASS/CONTROLLER ASSIGN (HX Edit, pages 39–43 of its guide), with
// what the HD500 has: per block a bypass footswitch (FS1–FS8, EXP Toe
// Switch); per parameter a controller (EXP 1/2, Variax Vol/Tone) moving it
// between a Minimum (heel) and a Maximum (toe). Left: every assignment of
// the preset; right: the selected block, which of its assignments
// (Bypass or a parameter), its controller and range.
const ASSIGN_BLOCKS = [0, 2, 4, 5, 6, 7, 8, 9, 10, 11]; // cabs follow their amp
const optLabel = (key, v) => (((meta && meta[key]) || []).find((o) => o.value === v) || {}).label || "—";

function assignmentsOf(v) {
  const rows = [];
  for (const id of ASSIGN_BLOCKS) {
    const it = v.items[id];
    if (!it || it.empty) continue;
    if (it.fs) rows.push({ id, k: -1, param: "Bypass", source: optLabel("bypassAssign", it.fs) });
    (it.params || []).forEach((p) => { // k: the API's parameter index (storage order)
      if (p.ctl) rows.push({ id, k: p.index, param: p.name, source: optLabel("controllers", p.ctl) });
    });
  }
  return rows;
}

function renderAuto() {
  const el = $("ed-auto");
  const v = flow.view;
  if (!el || flow.dragging) return;
  if (!v) { el.innerHTML = ""; return; }
  const rows = assignmentsOf(v);
  // sorted by the chosen column (HX: click a header; again = reverse)
  const srt = flow.asSort || (flow.asSort = { col: 0, dir: 1 });
  const keyOf = (r) => [v.items[r.id].model, r.param, r.source][srt.col];
  rows.sort((a, b) => srt.dir * keyOf(a).localeCompare(keyOf(b), undefined, { numeric: true, sensitivity: "base" }));
  // the block on the right: the selected one (if it can be assigned), else
  // the first assigned one
  let id = typeof flow.sel === "number" ? (flow.sel === 1 ? 0 : flow.sel === 3 ? 2 : flow.sel) : null;
  if (id === null || !v.items[id] || v.items[id].empty) id = rows.length ? rows[0].id : null;
  const it = id !== null ? v.items[id] : null;
  if (!flow.assign || flow.assign.id !== id) flow.assign = { id, k: -1 };
  const k = it && (it.params || []).some((q) => q.index === flow.assign.k) ? flow.assign.k : -1;
  const list = rows.map((r) => {
    const b = v.items[r.id];
    const cat = catOf(b);
    // rows of the selected block get its colour behind (HX); the open assignment stronger
    const cur = (r.id === id ? " mine" : "") + (r.id === id && r.k === k ? " cur" : "");
    return `<button type="button" class="as-row${cur}" data-as-id="${r.id}" data-as-k="${r.k}" style="--cat:${cat.color}">
      <span class="as-blk">${esc(b.model)}</span><span>${esc(r.param)}</span><span>${esc(r.source)}</span></button>`;
  }).join("") || '<p class="as-none">No bypass or controller assignments in this preset.</p>';
  const arrow = `<svg viewBox="0 0 12 8" aria-hidden="true"><path d="${srt.dir > 0 ? "M1.5 6.5 6 2l4.5 4.5" : "M1.5 1.5 6 6l4.5-4.5"}" /></svg>`;
  const cols = ["Block", "Parameter", "Source"].map((c, i) =>
    `<button type="button" class="${i === srt.col ? "on" : ""}" data-as-sort="${i}" title="Sort by ${c}">${c}${i === srt.col ? arrow : ""}</button>`).join("");
  const left = `<div class="as-list"><div class="as-cols">${cols}</div>${list}</div>`;
  if (!it) { el.innerHTML = `<div class="as-wrap">${left}<div class="as-detail"><div class="ac-tabs"><div class="ac-blank"></div></div></div></div>`; return; }
  const cat = catOf(it);
  // HX-style dropdown: the bar shows the choice; the list opens under it,
  // the current entry with a dot, assigned ones (o.on) in the block's colour.
  const sel = (attr, opts, cur) => {
    const c = opts.find((o) => o.value === cur) || opts[0] || { label: "—" };
    return `<div class="as-dd" data-dd="${attr}"><button type="button" class="as-dd-btn" aria-haspopup="listbox" aria-expanded="false">
      <span>${esc(c.bar ?? c.label)}</span><svg viewBox="0 0 12 8" aria-hidden="true"><path d="M1.5 1.5 6 6l4.5-4.5" /></svg></button>
      <div class="as-dd-list" role="listbox" hidden>${opts.map((o) =>
        `<button type="button" role="option" class="${o.value === cur ? "cur" : ""}${o.on ? " on" : ""}" data-v="${o.value}" aria-selected="${o.value === cur}">${esc(o.label)}</button>`).join("")}</div></div>`;
  };
  // Assignment menu: Bypass + the block's parameters (assigned ones marked)
  const assignOpts = [{ value: -1, label: "Bypass" },
    ...(it.params || []).map((p) => ({ value: p.index, label: p.name }))];
  // Header panel as HX Edit: [icon + block | Assignment ⌄] over
  // [Controller | its source ⌄]; Min/Max below when a parameter has one.
  const p = k >= 0 ? it.params.find((q) => q.index === k) : null;
  // Each source lists, in brackets, what already uses it in this preset
  // (one footswitch / pedal can drive several): "FS3 [Classic Dist, Dual Phaser]",
  // "EXP 1 [Brit P-75 Bright: Mid, Chrome Custom: Position]". The bar shows just the source.
  const users = (opts, isBypass) => opts.map((o) => {
    if (!o.value) return o;
    const who = [];
    for (const bid of ASSIGN_BLOCKS) {
      const b = v.items[bid];
      if (!b || b.empty) continue;
      if (isBypass) { if (b.fs === o.value) who.push(b.model); }
      else (b.params || []).forEach((q) => { if (q.ctl === o.value) who.push(`${b.model}: ${q.name}`); });
    }
    return who.length ? { ...o, label: `${o.label} [${who.join(", ")}]`, bar: o.label } : o;
  });
  const srcOpt = ((meta && meta[p ? "controllers" : "bypassAssign"]) || []).find((o) => o.value === (p ? p.ctl : it.fs));
  const cc = srcOpt && srcOpt.cc;
  // "MIDI CC" (last entry, only while this assignment uses a footswitch / EXP,
  // which have a fixed CC): shows MIDI Channel + the CC; nothing is sent.
  const MIDI_OPT = -100;
  const showMidi = !!(cc && flow.assign.midi);
  const withMidi = (opts) => (cc ? [...opts, { value: MIDI_OPT, label: "MIDI CC" }] : opts);
  const ctlDD = p ? sel("data-as-ctl", withMidi(users((meta && meta.controllers) || [], false)), showMidi ? MIDI_OPT : p.ctl)
    : sel("data-as-fs", withMidi(users((meta && meta.bypassAssign) || [], true)), showMidi ? MIDI_OPT : it.fs);
  const head = `<div class="as-head">
    <div class="as-hrow"><div class="as-blk-title" style="--cat:${cat.color}">${iconSVG(cat, "")}<span>${esc(it.model)}</span></div><div class="as-assign">${sel("data-as-which", assignOpts, k)}</div></div>
    <div class="as-hrow"><span class="as-hlabel">${p ? "Controller" : "Bypass"}</span>${ctlDD}</div></div>`;
  // MIDI In (HX: a CC per assignment). On the HD500 the CC is fixed by the
  // source (API: meta …[].cc): shown, not editable.
  // the ▲▼ arrows as on HX, disabled (the CC can't change on the HD500)
  const fixedSteps = '<span class="prm-step"><button type="button" disabled aria-label="Fixed"><svg viewBox="0 0 7 4" aria-hidden="true"><path d="M0 4L3.5 0L7 4Z" /></svg></button><button type="button" disabled aria-label="Fixed"><svg viewBox="0 0 7 4" aria-hidden="true"><path d="M0 0L3.5 4L7 0Z" /></svg></button></span>';
  // MIDI Channel: pedal-wide (Setup > MIDI/Tempo), the same for every
  // assignment; editable here. Then the assignment's fixed CC.
  const ch = (state && state.midiChannel) || 0;
  const chRow = slider("as.midich", "MIDI Channel", {
    value: ch ? (ch - 1) / 15 : 0, steps: 16,
    fmt: (x) => (ch || x ? `CH ${1 + Math.round(x * 15)}` : "—"),
    send: (x) => queueSend("/api/midi-channel", { channel: 1 + Math.round(x * 15) }),
  });
  const midi = showMidi ? `<div class="params as-midi">${chRow}${prmRow("MIDI CC", `CC ${cc}`, cc / 127, "", "", fixedSteps)}</div>` : "";
  let body;
  if (!p) {
    body = midi;
  } else {
    const rng = p.ctl ? ["min", "max"].map((w) => slider(`as.${id}.${k}.${w}`, `${w === "min" ? "Min" : "Max"} ${p.name}`, {
      value: w === "min" ? p.lo : p.hi,
      steps: p.kind === "list" && p.options ? p.options.length : 0, options: p.kind === "list" ? p.options : undefined,
      fmt: (x) => fmtParam({ ...p, value: x }),
      send: (x) => { if (w === "min") p.lo = x; else p.hi = x; queueSend("/api/assign", { id, param: k, [w]: x }); },
    })).join("") : "";
    body = `${rng ? `<div class="params">${rng}</div>` : ""}${midi}`; // MIDI rows last (under Min/Max)
  }
  el.innerHTML = `<div class="as-wrap">${left}<div class="as-detail" style="--cat:${cat.color}">${head}${body}</div></div>`;
}
const selRow = (name, control) => `<div class="prm prm-switch"><span class="prm-name">${esc(name)}</span>${control}</div>`;

async function postAssign(body) {
  try {
    const res = await fetch("/api/assign", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (!res.ok) throw await apiError(res);
    await fetchFlow();
  } catch (e) {
    toast(`Couldn't change the assignment: ${e.message}`);
    renderAuto();
  }
}

async function clearAssign(body) {
  try {
    const res = await fetch("/api/assign/clear", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (!res.ok) throw await apiError(res);
    await fetchFlow();
  } catch (e) {
    toast(`Couldn't clear: ${e.message}`);
  }
}

function initAuto() {
  const el = $("ed-auto");
  // right-click on an assignment: Clear Assignment / Clear All Assignments (HX Edit)
  const menu = $("as-menu");
  const hideMenu = () => { menu.hidden = true; };
  el.addEventListener("contextmenu", (e) => {
    const r = e.target.closest("[data-as-id]");
    if (!r) return;
    e.preventDefault();
    menu.dataset.id = r.dataset.asId;
    menu.dataset.k = r.dataset.asK;
    const box = menu.parentElement.getBoundingClientRect();
    menu.hidden = false;
    menu.style.left = `${Math.min(e.clientX - box.left, box.width - menu.offsetWidth - 4)}px`;
    menu.style.top = `${Math.min(e.clientY - box.top, box.height - menu.offsetHeight - 4)}px`;
  });
  menu.addEventListener("click", (e) => {
    const b = e.target.closest("[data-as-clear]");
    if (!b) return;
    hideMenu();
    clearAssign(b.dataset.asClear === "all" ? { all: true } : { id: Number(menu.dataset.id), param: Number(menu.dataset.k) });
  });
  document.addEventListener("pointerdown", (e) => { if (!menu.hidden && !menu.contains(e.target)) hideMenu(); });
  document.addEventListener("keydown", (e) => { if (e.key === "Escape") hideMenu(); });
  el.addEventListener("click", (e) => {
    const h = e.target.closest("[data-as-sort]");
    if (h) {
      const c = Number(h.dataset.asSort), s = flow.asSort || { col: 0, dir: 1 };
      flow.asSort = s.col === c ? { col: c, dir: -s.dir } : { col: c, dir: 1 };
      renderAuto();
      return;
    }
    const r = e.target.closest("[data-as-id]");
    if (!r) return;
    const id = Number(r.dataset.asId);
    flow.assign = { id, k: Number(r.dataset.asK) };
    selectBlock(id); // also shows it in the chain
  });
  // dropdowns: open / pick
  const closeDD = (except) => {
    for (const d of el.querySelectorAll(".as-dd")) {
      if (d === except) continue;
      d.querySelector(".as-dd-list").hidden = true;
      d.querySelector(".as-dd-btn").setAttribute("aria-expanded", "false");
    }
  };
  el.addEventListener("click", (e) => {
    const btn = e.target.closest(".as-dd-btn");
    if (btn) {
      const dd = btn.parentElement, list = dd.querySelector(".as-dd-list");
      closeDD(dd);
      list.hidden = !list.hidden;
      btn.setAttribute("aria-expanded", String(!list.hidden));
      if (!list.hidden) list.querySelector(".cur")?.scrollIntoView({ block: "nearest" });
      return;
    }
    const opt = e.target.closest(".as-dd-list [data-v]");
    if (!opt) return;
    const which = opt.closest(".as-dd").dataset.dd, v = Number(opt.dataset.v);
    closeDD();
    const id = flow.assign && flow.assign.id;
    if (id === null || id === undefined || opt.classList.contains("cur")) return;
    if ((which === "data-as-fs" || which === "data-as-ctl") && v === -100) { flow.assign.midi = true; renderAuto(); return; }
    if (which === "data-as-fs" || which === "data-as-ctl") flow.assign.midi = false;
    if (which === "data-as-which") { flow.assign.k = v; flow.assign.midi = false; renderAuto(); }
    else if (which === "data-as-fs") postAssign({ id, fs: v });
    else if (which === "data-as-ctl") postAssign({ id, param: flow.assign.k, ctl: v });
  });
  document.addEventListener("pointerdown", (e) => { if (!e.target.closest(".as-dd")) closeDD(); });
  document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeDD(); });
}

// Dropdown of a list parameter (HX Edit): the options under the value, the
// current one with a dot; picking one sends it like the slider would.
function openParamList(span) {
  const key = span.dataset.dd, d = flow.sliders[key];
  if (!d || !d.options) return;
  let m = $("prm-ddmenu");
  if (!m) {
    m = document.createElement("div");
    m.id = "prm-ddmenu";
    m.className = "as-dd-list prm-ddmenu";
    document.body.appendChild(m);
    m.addEventListener("click", (e) => {
      const b = e.target.closest("[data-i]");
      if (!b) return;
      const dd = flow.sliders[m.dataset.key];
      m.hidden = true;
      if (!dd) return;
      const n = dd.options.length, i = Number(b.dataset.i);
      const v = n > 1 ? i / (n - 1) : 0;
      if (Math.round(dd.value * (n - 1)) === i) return;
      dd.value = v;
      dd.send(v);
      renderParams();
    });
    document.addEventListener("pointerdown", (e) => { if (!m.hidden && !m.contains(e.target) && !e.target.closest("[data-dd]")) m.hidden = true; });
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") m.hidden = true; });
  }
  if (!m.hidden && m.dataset.key === key) { m.hidden = true; return; }
  const n = d.options.length, cur = Math.round(d.value * (n - 1));
  m.dataset.key = key;
  m.style.setProperty("--cat", getComputedStyle(span).getPropertyValue("--cat"));
  m.innerHTML = d.options.map((o, i) => `<button type="button" class="${i === cur ? "cur" : ""}" data-i="${i}">${esc(o)}</button>`).join("");
  m.hidden = false;
  const r = span.getBoundingClientRect();
  const w = Math.max(140, m.offsetWidth);
  m.style.left = `${Math.max(8, Math.min(r.right - w + 14, innerWidth - w - 8))}px`;
  const below = innerHeight - r.bottom - 8;
  m.style.maxHeight = `${Math.max(160, Math.min(320, below))}px`;
  m.style.top = `${below >= 160 ? r.bottom + 4 : Math.max(8, r.top - 320)}px`;
  m.querySelector(".cur")?.scrollIntoView({ block: "center" });
}

async function queueSend(url, body) {
  sender.next = { url, body };
  if (sender.busy) return;
  sender.busy = true;
  try {
    while (sender.next) {
      const { url: u, body: b } = sender.next;
      sender.next = null;
      const res = await fetch(u, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(b),
      });
      if (!res.ok) throw await apiError(res);
    }
  } catch (e) {
    sender.next = null;
    toast(`Couldn't change the parameter: ${e.message}`);
  } finally {
    sender.busy = false;
  }
}

// setFromFrac updates one slider and sends the value (lists snap to options).
function setFromFrac(bar, frac) {
  const d = flow.sliders[bar.dataset.s];
  if (!d) return;
  let v = Math.max(0, Math.min(1, frac));
  if (d.steps > 1) v = Math.round(v * (d.steps - 1)) / (d.steps - 1);
  if (Math.abs(v - d.value) < 1e-6) return;
  d.value = v;
  bar.style.setProperty("--v", v); // fill + handle position (CSS: HX geometry)
  bar.setAttribute("aria-valuenow", String(Math.round(v * 100)));
  bar.closest(".prm").querySelector(".prm-val").textContent = d.fmt(v);
  d.send(v);
}

// Typed values (HX Edit: click the value, type, Enter). The typed text is
// matched against what the slider would display: an option by name for lists,
// otherwise the number (with "k" = ×1000) nearest to the typed one.
const num = (s) => {
  const m = String(s).replace(",", ".").match(/-?\d+(\.\d+)?/);
  if (!m) return NaN;
  return parseFloat(m[0]) * (/\d\s*k/i.test(s) ? 1000 : 1);
};
function fracForText(d, text) {
  const t = text.trim().toLowerCase();
  if (!t) return null;
  const n = d.steps > 1 ? d.steps : 2001;
  const cands = Array.from({ length: n }, (_, i) => i / (n - 1));
  const exact = cands.find((c) => String(d.fmt(c)).trim().toLowerCase() === t);
  if (exact !== undefined) return exact;
  const want = num(t);
  if (Number.isNaN(want)) {
    const pre = cands.find((c) => String(d.fmt(c)).toLowerCase().startsWith(t));
    return pre === undefined ? null : pre;
  }
  let best = null, bd = Infinity;
  for (const c of cands) {
    const dv = Math.abs(num(d.fmt(c)) - want);
    if (dv < bd) { bd = dv; best = c; }
  }
  return best;
}

function editValue(span) {
  const key = span.dataset.edit;
  const d = flow.sliders[key];
  const bar = span.closest(".prm")?.querySelector(".prm-bar[data-s]");
  if (!d || !bar || span.querySelector("input")) return;
  const old = span.textContent;
  const input = document.createElement("input");
  input.className = "prm-input";
  input.value = old;
  input.setAttribute("aria-label", `${bar.getAttribute("aria-label")} value`);
  // As wide as its text, so the selection highlight hugs the value (HX).
  const fit = () => { input.style.width = `${Math.max(1, input.value.length) + 0.3}ch`; };
  fit();
  input.addEventListener("input", fit);
  span.textContent = "";
  span.appendChild(input);
  window.getSelection()?.removeAllRanges(); // the double-click's own word selection
  input.focus();
  input.select();
  let done = false;
  const finish = (commit) => {
    if (done) return;
    done = true;
    const f = commit ? fracForText(d, input.value) : null;
    span.textContent = d.fmt(d.value);
    if (f !== null) setFromFrac(bar, f);
    else if (commit && input.value.trim() !== old) toast(`"${input.value.trim()}" isn't a valid value`);
  };
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") { e.preventDefault(); finish(true); }
    else if (e.key === "Escape") { e.preventDefault(); finish(false); }
    e.stopPropagation(); // no Space toggling / arrow stepping while typing
  });
  input.addEventListener("blur", () => finish(true));
}

function initSliders(el = $("flow-detail")) {
  el.addEventListener("click", (e) => { // a list value: open its dropdown
    const v = e.target.closest("[data-dd]");
    if (v) openParamList(v);
  });
  el.addEventListener("dblclick", (e) => { // double-click the value to type one (HX Edit)
    const v = e.target.closest("[data-edit]");
    if (v && !e.target.closest("input")) editValue(v);
  });
  el.addEventListener("click", (e) => { // ▲▼ steppers
    const b = e.target.closest("[data-step]");
    if (!b) return;
    const d = flow.sliders[b.dataset.for];
    const bar = [...el.querySelectorAll(".prm-bar[data-s]")].find((x) => x.dataset.s === b.dataset.for);
    if (!d || !bar) return;
    const one = d.steps > 1 ? 1 / (d.steps - 1) : 0.01;
    setFromFrac(bar, d.value + one * Number(b.dataset.step));
  });
  // Mouse wheel over a parameter row moves its fader (HX Edit). One notch =
  // one option/note for lists, 1% otherwise; Shift = the finest step (1 ms,
  // 0.01 Hz…). Trackpads send many small deltas: they add up to notches.
  let wheelAcc = 0;
  el.addEventListener("wheel", (e) => {
    const bar = e.target.closest(".prm")?.querySelector(".prm-bar[data-s]");
    const d = bar && flow.sliders[bar.dataset.s];
    if (!d) return;
    e.preventDefault(); // the fader moves, not the panel
    const dy = (Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX) * (e.deltaMode === 1 ? 16 : 1);
    // macOS "natural scrolling" inverts the deltas; Safari says so: undo it,
    // so pushing the wheel up always raises the value.
    wheelAcc += e.webkitDirectionInvertedFromDevice ? -dy : dy;
    const notches = Math.trunc(wheelAcc / 40);
    if (!notches) return;
    wheelAcc -= notches * 40;
    const fine = d.steps > 1 ? 1 / (d.steps - 1) : 0.001;
    const one = e.shiftKey || (d.steps > 1 && d.steps <= 201) ? fine : 0.01;
    setFromFrac(bar, d.value - notches * one); // wheel up = higher value
  }, { passive: false });
  el.addEventListener("pointerdown", (e) => {
    const bar = e.target.closest(".prm-bar[data-s]");
    if (!bar || e.button !== 0) return;
    e.preventDefault();
    bar.focus({ preventScroll: true });
    bar.setPointerCapture(e.pointerId);
    flow.dragging = true;
    const at = (ev) => {
      const r = bar.getBoundingClientRect();
      setFromFrac(bar, (ev.clientX - r.left - 6) / (r.width - 12)); // the 12px handle stays inside the track
    };
    at(e);
    const move = (ev) => at(ev);
    const up = () => {
      bar.removeEventListener("pointermove", move);
      bar.removeEventListener("pointerup", up);
      bar.removeEventListener("pointercancel", up);
      flow.dragging = false;
      renderParams(); // catch up with values re-read meanwhile
    };
    bar.addEventListener("pointermove", move);
    bar.addEventListener("pointerup", up);
    bar.addEventListener("pointercancel", up);
  });
  el.addEventListener("keydown", (e) => {
    const bar = e.target.closest(".prm-bar[data-s]");
    const d = bar && flow.sliders[bar.dataset.s];
    if (!d) return;
    const one = d.steps > 1 ? 1 / (d.steps - 1) : 0.01;
    const step = e.shiftKey ? (d.steps > 1 && d.steps <= 20 ? one : Math.max(one, 0.1)) : one;
    const delta = { ArrowRight: step, ArrowUp: step, ArrowLeft: -step, ArrowDown: -step, PageUp: 0.1, PageDown: -0.1 }[e.key];
    if (delta === undefined && e.key !== "Home" && e.key !== "End") return;
    e.preventDefault();
    setFromFrac(bar, e.key === "Home" ? 0 : e.key === "End" ? 1 : d.value + delta);
  });
}

function initFlow() {
  fetch("/api/models").then((r) => r.json()).then((m) => { flow.models = m; renderEditor(); }).catch(() => {});
  const stage = $("flow-stage");
  const pick = (el) => {
    const g = el.closest(".blk");
    if (!g) return null;
    return g.dataset.id === "mixer" || g.dataset.id === "input" ? g.dataset.id : Number(g.dataset.id);
  };
  stage.addEventListener("click", (e) => {
    if (flow.justDragged) return;
    const id = pick(e.target);
    if (id === null) return;
    if (e.target.closest("[data-toggle]")) { toggleBlock(id); return; }
    if (e.target.closest("[data-remove]")) { removeBlock(id); return; }
    selectBlock(id);
    stage.focus({ preventScroll: true });
  });
  stage.addEventListener("dblclick", (e) => {
    const id = pick(e.target);
    if (id !== null && id !== "mixer" && !e.target.closest("[data-toggle], [data-remove]")) toggleBlock(id);
  });
  stage.addEventListener("keydown", (e) => {
    if (e.key === " " && typeof flow.sel === "number") { e.preventDefault(); toggleBlock(flow.sel); }
  });
  // EDIT | BYPASS/CONTROLLER ASSIGN (HX Edit): the second page replaces the
  // three edit columns while it is selected.
  document.querySelector(".ed-tabs").addEventListener("click", (e) => {
    const t = e.target.closest("[data-edtab]");
    if (!t) return;
    for (const b of document.querySelectorAll(".ed-tab")) {
      const on = b === t;
      b.classList.toggle("cur", on);
      b.setAttribute("aria-selected", String(on));
    }
    document.querySelector(".editors").classList.toggle("auto", t.dataset.edtab === "auto");
  });
  $("ed-cats").addEventListener("click", (e) => {
    const b = e.target.closest("[data-cat]");
    if (!b) return;
    flow.cat = b.dataset.cat;
    flow.sub = null; flow.subOpen = false;
    renderEditor();
  });
  $("ed-models").addEventListener("click", (e) => {
    // Guitar / Bass dropdown (amp, preamp, cab)
    if (e.target.closest("[data-subtoggle]")) { flow.subOpen = !flow.subOpen; renderEditor(); return; }
    const o = e.target.closest("[data-sub]");
    if (o) {
      flow.sub = o.dataset.sub;
      flow.subOpen = false;
      renderEditor();
      return;
    }
    if (flow.subOpen) { flow.subOpen = false; renderEditor(); return; } // a click outside closes it
    const b = e.target.closest("[data-type]");
    if (!b || b.classList.contains("cur")) return;
    setModel(Number(b.dataset.target), Number(b.dataset.type));
  });
  $("ed-models").addEventListener("click", (e) => { // Input 1 source
    const b = e.target.closest("[data-src]");
    if (b && !b.classList.contains("cur")) setInput("input1", Number(b.dataset.src));
  });
  $("flow-detail").addEventListener("click", (e) => {
    const sw = e.target.closest("[data-switch]"); // two-option switch
    if (sw) {
      const d = flow.sliders[sw.dataset.switch];
      const i = Number(sw.dataset.i);
      if (d && d.value !== i) {
        d.value = i;
        d.send(i);
        for (const b of sw.parentElement.children) { const on = b === sw; b.classList.toggle("cur", on); b.setAttribute("aria-pressed", String(on)); }
      }
      return;
    }
    const sy = e.target.closest("[data-sync]"); // tempo parameter: ms/Hz ⇄ note
    if (sy) { toggleSync(sy.dataset.sync); return; }
    const b = e.target.closest("[data-toggle-id]");
    if (b) toggleBlock(Number(b.dataset.toggleId));
    const s = e.target.closest("[data-setup]");
    if (s && s.getAttribute("aria-pressed") !== "true") setInputsSetup(Number(s.dataset.setup));
    const t = e.target.closest("[data-intab]");
    if (t) { flow.inTab = t.dataset.intab; renderParams(); }
    const vdi = e.target.closest("[data-vdi]");
    if (vdi) setVdiInput(vdi.dataset.vdi);
    const vc = e.target.closest("[data-vxctl]");
    if (vc && vc.getAttribute("aria-pressed") !== "true") setVariaxControl(Number(vc.dataset.vxctl));
  });
  initDrag();
  initSliders();
  initSliders($("ed-auto"));
  initAuto();
}

// ---- drag a block in the chain: the backend does the move
// (POST /api/chain/move {block, lane, before?}); the UI only says where.
function dropTarget(x, y) {
  const el = document.elementFromPoint(x, y);
  if (!el) return null;
  const z = el.closest(".drop-zone");
  if (z && z.dataset.amp) return flow.drag && flow.drag.amp ? { amp: z.dataset.amp, el: z } : null;
  if (z) return { lane: z.dataset.lane, el: z };
  if (flow.drag && flow.drag.amp) return null; // the amp only goes to its zones
  const g = el.closest(".blk[data-id]");
  if (!g || g.dataset.id === "mixer") return null;
  const it = flow.view.items[Number(g.dataset.id)];
  if (!it || it.kind !== "fx") return null;
  return { lane: flow.view.chain.lanes[it.lane].name, before: it.id, el: g };
}

// The FX drop rule: where block id lands for a target {lane, before?}
// (before = the block it was dropped on; none = the lane's end zone).
// Dropped on a block further right in the same lane it takes that block's
// place (goes after it); otherwise it goes before the target. Returns
// {lane, before} for the API (before undefined = lane end), or null when
// the block would stay where it is.
function fxDrop(chain, id, target) {
  if (!target || target.before === id) return null;
  const lane = chain.lanes.find((l) => l.name === target.lane);
  const ids = lane ? lane.blocks : [];
  let before = target.before;
  const from = ids.indexOf(id), to = ids.indexOf(before);
  if (before !== undefined && from >= 0 && to > from) before = ids.slice(to + 1).find((b) => b !== id);
  const noop = from >= 0 && (before === undefined ? from === ids.length - 1 : before === ids[from + 1]);
  return noop ? null : { lane: target.lane, before };
}

// previewChain: the chain after that drop (what the drag shows live).
function previewChain(chain, id, drop) {
  const lanes = chain.lanes.map((l) => ({ ...l, blocks: l.blocks.filter((b) => b !== id) }));
  const l = lanes.find((x) => x.name === drop.lane);
  if (!l) return chain;
  const at = drop.before === undefined ? -1 : l.blocks.indexOf(drop.before);
  l.blocks.splice(at < 0 ? l.blocks.length : at, 0, id);
  return { ...chain, lanes };
}

async function moveAmp(pos) {
  try {
    const res = await fetch("/api/amp/move", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pos }),
    });
    if (!res.ok) throw await apiError(res);
    await fetchFlow();
  } catch (e) {
    toast(`Couldn't move the amp: ${e.message}`);
  }
}

async function moveBlock(block, lane, before) {
  try {
    const res = await fetch("/api/chain/move", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(before === undefined ? { block, lane } : { block, lane, before }),
    });
    if (!res.ok) throw await apiError(res);
    await fetchFlow();
  } catch (e) {
    toast(`Couldn't move the block: ${e.message}`);
  }
}

function initDrag() {
  const stage = $("flow-stage");
  let start = null; // {id, x, y, pointerId}
  let hover = null;
  const setHover = (t) => {
    if (hover && hover.el) hover.el.classList.remove("drop-hover");
    hover = t;
    if (hover && hover.el) hover.el.classList.add("drop-hover");
  };
  stage.addEventListener("pointerdown", (e) => {
    const g = e.target.closest(".blk[data-id]");
    if (!g || e.button !== 0 || e.target.closest("[data-toggle], [data-remove]")) return;
    const it = flow.view && flow.view.items[Number(g.dataset.id)];
    // FX blocks, empty slots too (the pedal moves them); amp A (even None)
    // or cab A move amp A with its cab. Amp B never moves (as on the pedal).
    const isAmp = it && (it.id === 0 || (it.id === 1 && !flow.view.items[0].empty));
    if (!it || (!isAmp && it.kind !== "fx")) return;
    e.preventDefault(); // no text selection (Safari paints it blue while dragging)
    stage.focus({ preventScroll: true });
    const box = g.querySelector(".blk-box, .amp-empty-box, .slot-box")?.getBoundingClientRect();
    start = { id: it.id, amp: isAmp, x: e.clientX, y: e.clientY, pointerId: e.pointerId,
      offX: box ? e.clientX - (box.left + box.width / 2) : 0, offY: box ? e.clientY - (box.top + box.height / 2) : 0 };
  });
  stage.addEventListener("pointermove", (e) => {
    if (!start) return;
    if (!flow.drag) {
      if (Math.hypot(e.clientX - start.x, e.clientY - start.y) < 6) return;
      flow.drag = { id: start.id, amp: start.amp };
      window.getSelection()?.removeAllRanges();
      stage.setPointerCapture(start.pointerId);
      stage.classList.add("dragging");
      renderFlow(); // shows the drop zones
      stage.querySelector(`.blk[data-id="${start.id}"]`)?.classList.add("drag-src");
      if (!start.amp) snapshotTargets();
    }
    if (start.amp) {
      followPointer(e);
      setHover(dropTarget(e.clientX, e.clientY));
      return;
    }
    // FX: the target is found on the layout as it was when the drag began
    // (stable while the preview moves blocks around), from the dragged
    // block's centre; the chain is redrawn as it would be after the drop.
    const drop = fxDrop(flow.view.chain, start.id, fxTarget(e.clientX - start.offX, e.clientY - start.offY));
    const key = drop ? `${drop.lane}:${drop.before}` : "";
    if (key !== start.key) {
      start.key = key;
      start.drop = drop;
      flow.preview = drop ? previewChain(flow.view.chain, start.id, drop) : null;
      renderFlow();
    }
    followPointer(e);
  });
  const snapshotTargets = () => {
    const svg = stage.querySelector("svg.chain");
    const ctm = svg && svg.getScreenCTM();
    if (!ctm) { start.base = []; return; }
    const laneOfId = (id) => (flow.view.chain.lanes.find((l) => l.blocks.includes(id)) || {}).name;
    start.scale = ctm.a;
    start.base = layout().nodes.filter((n) => n.zone || (n.it && n.it.kind === "fx")).map((n) => {
      const pt = new DOMPoint(n.x, n.y).matrixTransform(ctm);
      return { x: pt.x, y: pt.y, t: n.zone ? { lane: n.zone } : { lane: laneOfId(n.it.id), before: n.it.id } };
    });
  };
  const fxTarget = (x, y) => {
    let best = null, bd = Infinity;
    const rx = (COL / 2) * (start.scale || 1), ry = 32 * (start.scale || 1);
    for (const b of start.base || []) {
      // Targets where the block would stay put (e.g. the end of its own lane
      // right next to it) are skipped, except the block itself: they would
      // swallow the space up to the next real position.
      if (b.t.before !== start.id && !fxDrop(flow.view.chain, start.id, b.t)) continue;
      const dx = Math.abs(x - b.x), dy = Math.abs(y - b.y);
      if (dx < rx && dy < ry && dx + dy < bd) { bd = dx + dy; best = b.t; }
    }
    return best;
  };
  const followPointer = (e) => {
    const g = stage.querySelector(`.blk[data-id="${start.id}"]`);
    const mates = start.amp ? [0, 1].filter((i) => i !== start.id)
      .map((i) => stage.querySelector(`.blk[data-id="${i}"]`)).filter(Boolean) : [];
    const svg = g?.ownerSVGElement;
    const rect = g?.querySelector(".blk-box, .amp-empty-box, .slot-box");
    if (!svg || !rect) return;
    g.classList.add("drag-src"); // again, if the chain was redrawn meanwhile
    // A faded copy stays at the block's place while it is dragged (a redraw
    // drops it with the rest; it is recreated here). No data-id: not a target.
    if (!g.parentNode.querySelector(".blk-ghost")) {
      const ghost = g.cloneNode(true);
      ghost.removeAttribute("data-id");
      ghost.removeAttribute("transform");
      ghost.classList.remove("drag-src", "sel");
      ghost.classList.add("blk-ghost");
      ghost.querySelector(".blk-hover")?.remove();
      g.parentNode.insertBefore(ghost, g);
    }
    if (g.nextSibling) g.parentNode.appendChild(g); // drawn last = on top
    const ctm = svg.getScreenCTM();
    if (!ctm) return;
    const pt = new DOMPoint(e.clientX - start.offX, e.clientY - start.offY).matrixTransform(ctm.inverse());
    const cx = Number(rect.getAttribute("x")) + Number(rect.getAttribute("width")) / 2;
    const cy = Number(rect.getAttribute("y")) + Number(rect.getAttribute("height")) / 2;
    const tf = `translate(${(pt.x - cx).toFixed(2)} ${(pt.y - cy).toFixed(2)})`;
    g.setAttribute("transform", tf);
    for (const m of mates) { // the cab with its amp (or the amp with its cab)
      m.classList.add("drag-src");
      if (!m.dataset.ghosted) {
        const ghost = m.cloneNode(true);
        ghost.removeAttribute("data-id");
        ghost.classList.remove("drag-src", "sel");
        ghost.classList.add("blk-ghost");
        ghost.querySelector(".blk-hover")?.remove();
        m.parentNode.insertBefore(ghost, m);
        m.dataset.ghosted = "1";
      }
      m.parentNode.appendChild(m);
      m.setAttribute("transform", tf);
    }
  };
  const end = (e, cancel) => {
    if (!start) return;
    const dragged = flow.drag;
    const target = !cancel && dragged && dragged.amp ? dropTarget(e.clientX, e.clientY) : null;
    const drop = !cancel && dragged && !dragged.amp ? start.drop : null;
    start = null;
    setHover(null);
    if (!dragged) return;
    flow.drag = null;
    flow.justDragged = true; // swallow the click that follows
    setTimeout(() => { flow.justDragged = false; }, 0);
    stage.classList.remove("dragging");
    if (dragged.amp) { renderFlow(); if (target && target.amp) moveAmp(target.amp); return; }
    if (!drop) { flow.preview = null; renderFlow(); return; }
    // Keep showing the new order until the pedal has it (no flash back).
    renderFlow();
    moveBlock(dragged.id, drop.lane, drop.before).finally(() => { flow.preview = null; renderFlow(); });
  };
  stage.addEventListener("pointerup", (e) => end(e, false));
  stage.addEventListener("pointercancel", (e) => end(e, true));
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && flow.drag) end(e, true);
  });
}

// ---- the input block (Input 1/2 sources, Guitar In-Z, Inputs Setup) ----
// In the HD500, In-Z and Inputs Setup are one setting for both inputs, so the
// editor shows a single "Input" entry. Values and option lists come from the
// API (state + /api/meta); writes go to /api/input and /api/inputs-setup.
const GUITAR_BODY = "M11 6.5c-1-.8-2.6-.8-3.4.2-.7.9-.4 2.1-1.1 3-.9 1-2 1.6-2 3.4 0 2.9 2.9 5 7.5 5s7.5-2.1 7.5-5c0-1.8-1.1-2.4-2-3.4-.7-.9-.4-2.1-1.1-3-.8-1-2.4-1-3.4-.2zM10.2 12.7h3.6";
const INPUT_SVG = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="${GUITAR_BODY}" transform="rotate(40 12 12)" /></svg>`;

function inputIcon(label) {
  const l = String(label || "");
  if (/guitar/i.test(l)) return { d: GUITAR_BODY, tf: "rotate(40 12 12)" };
  const d = /same/i.test(l) ? "M5 9h14M5 15h14"
    : /mic/i.test(l) ? "M12 3a3 3 0 0 1 3 3v5a3 3 0 0 1-6 0V6a3 3 0 0 1 3-3zM6 11a6 6 0 0 0 12 0M12 17v4"
    : /mags/i.test(l) ? "M5 6l7 12 7-12M8 4h8"
    : /variax/i.test(l) ? "M5 6l7 12 7-12"
    : "M3 12h11M10 8l4 4-4 4M18 5v14"; // aux
  return { d, tf: "rotate(0 12 12)" };
}

// inputLabel: the option label of an input setting's raw value.
function inputLabel(key, value) {
  const o = ((meta && meta[key]) || []).find((x) => x.value === value);
  return o ? o.label : "—";
}

// inputShortName: the label under the input block, as the HX shows it: a
// source that combines inputs ("Guitar+Aux") is "Multi".
const inputShortName = (l) => (l.includes("+") ? "Multi" : l);

// A pedal-wide Global / Preset setting as an HX two-button switch row.
function setupSwitch(name, opts, cur, attr) {
  const btns = opts.map((o) => `<button type="button" class="${o.value === cur ? "cur" : ""}" ${attr}="${o.value}" aria-pressed="${o.value === cur}">${esc(o.label)}</button>`).join("");
  return `<div class="prm prm-switch"><span class="prm-name">${esc(name)}</span><span class="seg" role="group" aria-label="${esc(name)}">${btns}</span></div>`;
}
const VARIAX_PATH = "M5 6l7 12 7-12";

function inputParamsHTML() {
  const st = state || {};
  const optSlider = (key, name) => {
    const opts = (meta && meta[key]) || [];
    const idx = Math.max(0, opts.findIndex((o) => o.value === st[key]));
    const n = Math.max(1, opts.length - 1);
    return slider(`in.${key}`, name, {
      value: idx / n,
      steps: opts.length, options: opts.map((o) => o.label),
      fmt: (f) => (opts[Math.round(f * n)] || {}).label || "—",
      send: (f) => {
        const o = opts[Math.round(f * n)];
        if (o) queueSend("/api/input", { id: meta.setupInput[key], value: o.value });
      },
    });
  };
  const setupOpts = ((meta && meta.inputsSetup) || []).slice().sort((a, b) => b.value - a.value); // Global, Preset
  const global = st.inputsSetup === 1
    ? '<p class="in-note">Inputs Setup is Global: these inputs apply to all presets and are stored in the pedal settings, not in this preset.</p>' : "";
  // Input | Variax tabs, the Variax one only when an input uses the Variax.
  const variax = usesVariax(st);
  if (!variax) flow.inTab = "input";
  const tab = flow.inTab || "input";
  // Header bar as the other blocks (HX: icon + the source's name). With the
  // Variax in use: two tabs, "Input" and "Variax", as Amp | Cab once were
  // (not the source's name: "Variax | Variax" for the Variax alone).
  const src = inputLabel("input1", st.input1);
  const ic = inputIcon(src);
  const srcIcon = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="${ic.d}" transform="${ic.tf}" /></svg>`;
  const head = variax
    ? `<div class="ac-tabs" role="tablist" style="--cat:#f2f3f5">
        <button type="button" class="ac-tab${tab === "input" ? " cur" : ""}" role="tab" data-intab="input" aria-selected="${tab === "input"}" title="Input 1 Source: ${esc(src)}">${INPUT_SVG}<span>Input</span></button>
        <button type="button" class="ac-tab${tab === "variax" ? " cur" : ""}" role="tab" data-intab="variax" aria-selected="${tab === "variax"}"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="${VARIAX_PATH}" /></svg><span>Variax</span></button></div>`
    : `<div class="ac-tabs"><div class="ac-title" style="--cat:#f2f3f5" title="Input 1 Source: ${esc(src)}">${srcIcon}<span>${esc(src)}</span></div></div>`;
  if (variax && tab === "variax") return head + variaxParamsHTML(st);
  return `${head}
    <div class="params">${optSlider("inz", "Guitar In-Z")}${optSlider("input2", "Input 2 Source")}
      ${setupSwitch("Inputs Setup", setupOpts, st.inputsSetup, "data-setup")}</div>
    ${global}`;
}

// usesVariax: Input 1 or Input 2 has the Variax as (part of) its source.
function usesVariax(st) {
  return /variax/i.test(inputLabel("input1", st.input1)) || /variax/i.test(inputLabel("input2", st.input2));
}

// The Variax tab: Model (list of the 300/600 500/700, the only one known),
// Tone knob (0..127, shown in %) and the pedal-wide Variax Control.
function variaxParamsHTML(st) {
  const vx = (flow.view && flow.view.variax) || { model: 0, tone: 0 };
  // VDI Input: only picks the list of model names (software-only, like the
  // original editor; the pedal stores just the index).
  const vdi = VDI_TYPES.includes(vdiInput()) ? vdiInput() : VDI_TYPES[0];
  const known = (meta && meta.variaxModels && meta.variaxModels[vdi]) || null;
  // A type without a known list (Bass 700/705) shows the raw index, 0..110.
  const models = known || Array.from({ length: 111 }, (_, i) => (i === 0 ? "(Don't Force)" : `Model ${i}`));
  const nm = Math.max(1, models.length - 1);
  const tmax = (meta && meta.variaxToneMax) || 127;
  const nv = VDI_TYPES.length - 1;
  const vdiSlider = slider("vx.vdi", "VDI Input", {
    value: VDI_TYPES.indexOf(vdi) / nv,
    steps: VDI_TYPES.length, options: VDI_TYPES,
    fmt: (f) => VDI_TYPES[Math.round(f * nv)],
    send: (f) => setVdiInput(VDI_TYPES[Math.round(f * nv)], false),
  });
  const model = slider("vx.model", "Model", {
    value: vx.model / nm,
    steps: models.length, options: models,
    fmt: (f) => models[Math.round(f * nm)] || "—",
    send: (f) => { vx.model = Math.round(f * nm); queueSend("/api/variax", { param: "model", value: vx.model }); },
  });
  const tone = slider("vx.tone", "Tone", {
    value: vx.tone / tmax,
    steps: tmax + 1,
    fmt: (f) => `${Math.floor(f * 100 + 1e-6)} %`, // the pedal truncates: 63/127 shows 49%
    send: (f) => { vx.tone = Math.round(f * tmax); queueSend("/api/variax", { param: "tone", value: vx.tone }); },
  });
  // Only the 300/600 500/700 (the user's Variax) is verified on the pedal; for
  // the other types Model and Tone are DISPLAY ONLY for now (user's decision,
  // 2026-11-06: a change with Acoustic 700 selected jammed the USB link).
  if (vdi !== VDI_TYPES[0]) {
    const roModel = prmRow("Model", models[vx.model] || `Model ${vx.model}`, vx.model / nm);
    const roTone = prmRow("Tone", `${Math.floor((vx.tone / tmax) * 100 + 1e-6)} %`, vx.tone / tmax);
    const extraRO = vdi === "Tyler Variax" ? tylerOnlyHTML() : vdi === "Acoustic 700" ? acousticOnlyHTML() : "";
    const unknownRO = known ? "" : '<p class="in-note in-pending">The model list for this type is not known yet: Model shows the number stored in the pedal.</p>';
    return `<div class="params">${vdiSlider}</div>${unknownRO}
    <p class="in-note in-pending"><span class="off-tag">Disabled</span> ${esc(vdi)}: not working yet. Model and Tone only show what is in the preset; nothing is sent to the pedal.</p>
    <div class="params params-pending" aria-disabled="true">${roModel}${roTone}</div>${extraRO}
    <div class="params">${setupSwitch("Variax Control", (meta && meta.variaxControl) || [], st.variaxControl, "data-vxctl")}</div>`;
  }
  const extra = "";
  const unknown = known ? "" : '<p class="in-note in-pending">The model list for this type is not known yet: Model shows the number stored in the pedal.</p>';
  return `<div class="params">${vdiSlider}</div>${unknown}
    <div class="params params-gap">${model}${tone}</div>${extra}
    <div class="params">${setupSwitch("Variax Control", (meta && meta.variaxControl) || [], st.variaxControl, "data-vxctl")}</div>`;
}

async function setVariaxControl(value) {
  try {
    const res = await fetch("/api/variax", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ param: "control", value }),
    });
    if (!res.ok) throw await apiError(res);
  } catch (e) {
    toast(`Couldn't change Variax Control: ${e.message}`);
  }
}

async function setInput(key, value) {
  try {
    const res = await fetch("/api/input", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id: meta.setupInput[key], value }),
    });
    if (!res.ok) throw await apiError(res);
  } catch (e) {
    toast(`Couldn't change the input: ${e.message}`);
  }
}

async function setInputsSetup(value) {
  try {
    const res = await fetch("/api/inputs-setup", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value }),
    });
    if (!res.ok) throw await apiError(res);
  } catch (e) {
    toast(`Couldn't change Inputs Setup: ${e.message}`);
  }
}

// vdiInput: the VDI Input type chosen in the Variax tab, kept in this
// browser (a display preference; the pedal doesn't store it).
function vdiInput() {
  try { return localStorage.getItem("hd500.vdiInput") || "300/600 500/700"; } catch { return "300/600 500/700"; }
}

// setVdiInput stores the choice; while the fader is dragged the screen is
// rebuilt when the drag ends (renderParams waits for it).
function setVdiInput(t, now = true) {
  try { localStorage.setItem("hd500.vdiInput", t); } catch { /* private mode: not remembered */ }
  if (now || !flow.dragging) renderParams();
}

// The VDI Input types of the original editor, in its order.
const VDI_TYPES = ["300/600 500/700", "Tyler Variax", "Acoustic 700", "Bass 700", "Bass 705"];

// Acoustic 700 knobs of the original editor: DISPLAY ONLY (codes unknown).
function acousticOnlyHTML() {
  return `<p class="in-note in-pending"><span class="off-tag">Disabled</span> Acoustic 700 options: not working yet (the codes the pedal uses for them are unknown).</p>
    <div class="params params-pending" aria-disabled="true">${prmRow("Compression", "0 %", 0)}${prmRow("Mic", "0 %", 0)}</div>`;
}

// Tyler Variax options of the original editor (Local Control, Variax Tuning,
// note offset per string). DISPLAY ONLY: how the pedal stores them is still
// unknown (probably among the unnamed setup ids 2..21), so nothing is sent.
const TYLER_LOCAL = ["Unlocked", "Volume Lock", "Tone Lock", "Vol+Tone Lock", "Vol+Tone+Togl Lock"];
const TYLER_STRINGS = [["6", "E"], ["5", "A"], ["4", "D"], ["3", "G"], ["2", "B"], ["1", "E"]];

function tylerOnlyHTML() {
  const rows = [
    prmRow("Local Control", TYLER_LOCAL[0], 0),
    prmRow("Variax Tuning", "Don't Force", 0),
    ...TYLER_STRINGS.map(([n, note]) => prmRow(`String ${n} (${note})`, "—", 0)),
  ];
  return `<p class="in-note in-pending"><span class="off-tag">Disabled</span> Tyler Variax options: not working yet (the codes the pedal uses for them are unknown).</p>
    <div class="params params-pending" aria-disabled="true">${rows.join("")}</div>`;
}
