// Category colours and icons, from the classic UI (cmd/hd500-edit/web/flow.js).
// HX Edit-style 32×32 line drawings (the user's own set, mapped to the
// HD500's categories), drawn with currentColor and inline styles, so the
// category colour applies and generic `svg path` rules can't restyle them.
// They are static strings rendered with dangerouslySetInnerHTML.
import type { Item, ModelGroup } from "../api";
// STROKE: the drawings are shown larger than in the classic UI (26 vs 22 units,
// HX proportions); their lines are thinned to keep HX Edit's weight.
const STROKE = 0.8;
const S = (w: number, op = 1) => `style="fill:none;stroke:currentColor;stroke-width:${+(w * STROKE).toFixed(2)};stroke-linecap:round;stroke-linejoin:round${op < 1 ? `;stroke-opacity:${op}` : ""}"`;
const F = (op = 1) => `style="fill:currentColor;stroke:none${op < 1 ? `;fill-opacity:${op}` : ""}"`;
export const ICONS32: Record<string, string> = {
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
  // amp head: taller than the classic drawing (fills the block like HX), handle,
  // control panel with knobs, speaker cloth (faint fill), feet
  amp: `<path d="M11.5 9.2V7.6C11.5 6.9 13 6.4 16 6.4C19 6.4 20.5 6.9 20.5 7.6V9.2" ${S(1.6)}/><rect x="4" y="9.2" width="24" height="15" rx="2" ${S(1.8)}/>` +
    `<path d="M4 14.4H28" ${S(1.45)}/><circle cx="8.6" cy="11.8" r="0.95" ${F()}/><circle cx="12.2" cy="11.8" r="0.95" ${F()}/><circle cx="15.8" cy="11.8" r="0.95" ${F()}/><circle cx="19.4" cy="11.8" r="0.95" ${F()}/><circle cx="23" cy="11.8" r="0.95" ${F(0.55)}/>` +
    `<rect x="5.6" y="15.9" width="20.8" height="6.9" rx="0.8" ${F(0.16)}/><path d="M8 25.6H10.5M21.5 25.6H24" ${S(1.6)}/>`,
  // preamp: a bit larger than the classic drawing (fills the block like HX)
  preamp: `<g transform="translate(16 16) scale(1.2) translate(-16 -14.8)">` + `<path d="M8 15V10.5C8 9 9 8 10.2 8C11.4 8 12.4 9 12.4 10.5V15Z" ${F()}/><path d="M13.6 15V10.5C13.6 9 14.6 8 15.8 8C17 8 18 9 18 10.5V15Z" ${F()}/>` +
    `<rect x="5" y="15" width="22" height="6.5" rx="1.5" ${S(2.1)}/><path d="M9 15V21.5M23 15V21.5" ${S(1.6)}/>` + `</g>`,
  cab: `<path d="M9 13H6.5C5.5 13 5 14 5 16C5 18 5.5 19 6.5 19H9Z" ${F()}/><path d="M9 12.5L16 6.5M9 19.5L16 25.5" ${S(1.8)}/>` +
    `<ellipse cx="17.5" cy="16" rx="6.5" ry="9.8" ${S(1.85)}/><ellipse cx="13.5" cy="16" rx="2.4" ry="3.2" ${F()}/>`,
  volume: `<path d="M7 24V20M11.5 24V17M16 24V14M20.5 24V11M25 24V7.5" ${S(2.6)}/>`,
  loop: `<path d="M6 20L18.5 8.5" ${S(2.4)}/><path d="M20.5 6.5L14 8.5L18.5 13Z" ${F()}/><path d="M26 12L13.5 23.5" ${S(2.4)}/><path d="M11.5 25.5L18 23.5L13.5 19Z" ${F()}/>`,
};

export interface Cat { color: string; svg?: string; icon: string; fill?: string }
export const CATS: Record<string, Cat> = {
  Distortion: { color: "#f8932a", /* HX #fc7f30 on screen (Safari shifts orange: #fb8034 showed #ff6d3a) */ svg: ICONS32.distortion, icon: "M2 15h4l2.5-8h3l2 10h3l1.5-5H22" },
  Dynamics: { color: "#dbc239", svg: ICONS32.dynamics, icon: "M3 19c5 0 7-1 9-5s4-9 9-9M3 19h18" },
  "Preamp+EQ": { color: "#dbc239", svg: ICONS32.eq, icon: "M6 4v16M12 4v16M18 4v16M4 14h4M10 8h4M16 12h4" },
  EQ: { color: "#dbc239", svg: ICONS32.eq, icon: "M6 4v16M12 4v16M18 4v16M4 14h4M10 8h4M16 12h4" },
  Modulation: { color: "#0294ec", svg: ICONS32.modulation, icon: "M2 12c2.5-7 5.5-7 8 0s5.5 7 8 0c1-2.8 2-4 4-4" },
  Delay: { color: "#06c535", svg: ICONS32.delay, icon: "M5 12a7 7 0 1 0 14 0a7 7 0 1 0-14 0M9 12a3 3 0 1 0 6 0a3 3 0 1 0-6 0" },
  Reverb: { color: "#f24025", svg: ICONS32.reverb, icon: "M4 20a16 16 0 0 1 16-16M4 20a11 11 0 0 1 11-11M4 20a6 6 0 0 1 6-6" },
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
export const NONE_CAT: Cat = { color: "#fdfdfd", icon: "M6 6l12 12M18 6L6 18" };


/** The glyph of a category: HX 32×32 drawing, or the older 24×24 path (mixer). */
export const catGlyph = (cat: Cat) =>
  cat.svg ? { vb: "0 0 32 32", inner: cat.svg, cls: " ic32" } : { vb: "0 0 24 24", inner: `<path d="${cat.icon}" />`, cls: "" };

/** A preamp is an amp model from the "Preamps" group (its block says Preamp). */
export function isPreamp(it: Item | undefined, models: ModelGroup[]): boolean {
  if (!it || it.empty) return false;
  const g = models.find((x) => x.name === "Preamps");
  return g ? g.models.some((m) => !m.none && m.type === it.type) : / Preamp$/.test(it.model);
}

export const catOf = (it: Item, models: ModelGroup[]): Cat =>
  it.kind === "amp" ? (isPreamp(it, models) ? CATS.preamp : CATS.amp) : it.kind === "cab" ? CATS.cab : CATS[it.group ?? ""] || CATS.none;

// ---- the input block's icon, by Input 1 source
export const GUITAR_BODY = "M11 6.5c-1-.8-2.6-.8-3.4.2-.7.9-.4 2.1-1.1 3-.9 1-2 1.6-2 3.4 0 2.9 2.9 5 7.5 5s7.5-2.1 7.5-5c0-1.8-1.1-2.4-2-3.4-.7-.9-.4-2.1-1.1-3-.8-1-2.4-1-3.4-.2zM10.2 12.7h3.6";

export function inputIcon(label: string): { d: string; tf: string } {
  if (/guitar/i.test(label)) return { d: GUITAR_BODY, tf: "rotate(40 12 12)" };
  const d = /same/i.test(label) ? "M5 9h14M5 15h14"
    : /mic/i.test(label) ? "M12 3a3 3 0 0 1 3 3v5a3 3 0 0 1-6 0V6a3 3 0 0 1 3-3zM6 11a6 6 0 0 0 12 0M12 17v4"
    : /mags/i.test(label) ? "M5 6l7 12 7-12M8 4h8"
    : /variax/i.test(label) ? "M5 6l7 12 7-12"
    : "M3 12h11M10 8l4 4-4 4M18 5v14"; // aux
  return { d, tf: "rotate(0 12 12)" };
}

/** The label under the input block, as the HX shows it: a source that combines inputs is "Multi". */
export const inputShortName = (l: string) => (l.includes("+") ? "Multi" : l);
