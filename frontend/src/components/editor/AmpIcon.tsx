// Amp thumbnails: original drawings built from the API's style (data/
// gear-styles.json): combo or head, tolex, control panel with its knobs and
// the grille cloth's texture. No logos, no copied art. (classic: ampIconSVG)
import { useId } from "react";
import type { AmpStyle } from "../../editor/format";

export const KNOB_COLORS: Record<string, string> = { white: "#f0f0f0", black: "#141414", chrome: "#c9ccd1", cream: "#eadfbf" };

/** shade("#rrggbb", f): f < 1 darker, f > 1 lighter. */
export function shade(hex: string, f: number) {
  const n = parseInt(String(hex).replace("#", ""), 16);
  if (Number.isNaN(n)) return hex;
  const c = (v: number) => Math.max(0, Math.min(255, Math.round(v * f)));
  return `rgb(${c(n >> 16)},${c((n >> 8) & 255)},${c(n & 255)})`;
}

/** Grille cloth textures (pattern in a 3×3 tile), by gear-styles grilleType. */
export function grillePattern(id: string, type: string | undefined, base: string) {
  const ink = shade(base, 0.62), hi = shade(base, 1.25);
  // inline styles: the list CSS restyles every <path> in an .ed-item
  const ln = (d: string, color: string, w: number) => `<path d="${d}" style="fill:none;stroke:${color};stroke-width:${w}"/>`;
  const lines = ({
    tweed: ln("M0 3L3 0M-1 1L1 -1M2 4L4 2", ink, 0.7),
    silver: ln("M0 .75H3M0 2.25H3", hi, 0.55),
    diamond: ln("M0 0L3 3M3 0L0 3", ink, 0.5),
    "diamond-plate": `<path d="M1.5 .4L2.6 1.5L1.5 2.6L.4 1.5Z" style="fill:${hi};stroke:none"/>`,
    basket: ln("M0 0L3 3M3 0L0 3", ink, 0.9),
  } as Record<string, string>)[type ?? ""] || ln("M0 1.5H3", ink, 0.5);
  return `<pattern id="${id}" width="3" height="3" patternUnits="userSpaceOnUse"><rect width="3" height="3" fill="${base}"/>${lines}</pattern>`;
}

// Speakers seen through the cloth (HX thumbnails show them on some combos):
// faint circles; on a combo without cloth (grilleType none) an open, dark port.
const SPEAKERS: Record<number, [number, number, number][]> = {
  1: [[15, 12.6, 4.3]],
  2: [[9.3, 12.6, 3.9], [20.7, 12.6, 3.9]],
  4: [[9.8, 10.3, 2.05], [20.2, 10.3, 2.05], [9.8, 14.9, 2.05], [20.2, 14.9, 2.05]],
};
function speakers(s: AmpStyle, grille: string) {
  const list = SPEAKERS[s.speakers ?? 0];
  if (!list) return "";
  const open = s.grilleType === "none";
  return list.map(([cx, cy, r]) => open
    ? `<circle cx="${cx}" cy="${cy}" r="${r}" fill="#23252f" stroke="${shade(grille, 0.6)}" stroke-width=".5"/>`
    : `<circle cx="${cx}" cy="${cy}" r="${r}" fill="${shade(grille, 0.78)}" fill-opacity=".55" stroke="${shade(grille, 0.6)}" stroke-width=".35" stroke-opacity=".7"/>`).join("");
}

function ampIconInner(s: AmpStyle, id: string) {
  const tolex = s.tolex || "#2a2a2a", panel = s.panel || "#202020", grille = s.grille || "#3a3a3a";
  const trim = s.trim || shade(tolex, 0.6), knob = KNOB_COLORS[s.knob ?? ""] || KNOB_COLORS.chrome;
  // knobs get a contrasting rim so they show on any panel colour
  const rim = s.knob === "black" ? "rgba(255,255,255,.4)" : "rgba(0,0,0,.55)";
  const knobs = (y: number, x0: number, x1: number, n: number, r: number) => Array.from({ length: n }, (_, i) =>
    `<circle cx="${(x0 + ((x1 - x0) * i) / (n - 1)).toFixed(2)}" cy="${y}" r="${r}" fill="${knob}" stroke="${rim}" stroke-width=".35"/>`).join("");
  let body: string;
  if (s.layout === "head") { // low, wide box (HX thumbnails: a strip) with a handle; cloth band over the control panel
    body = `<rect x="11" y="3.6" width="8" height="1.5" rx=".75" fill="${shade(tolex, 0.45)}"/>
      <rect x="1" y="5.4" width="28" height="10.6" rx="1.3" fill="${tolex}" stroke="${trim}" stroke-width=".6"/>
      <rect x="2.4" y="6.8" width="25.2" height="3.4" fill="url(#${id})"/>
      <rect x="2.4" y="10.8" width="25.2" height="3.9" fill="${panel}"/>${knobs(12.75, 6, 24, 6, 0.8)}`;
  } else { // combo: panel on top, speaker cloth below
    body = `<rect x="1" y="1" width="28" height="18" rx="1.6" fill="${tolex}" stroke="${trim}" stroke-width=".6"/>
      <rect x="2.6" y="2.6" width="24.8" height="4.2" fill="${panel}"/>${s.controls === "top" ? `<rect x="10" y="3.9" width="10" height="1.6" rx=".5" fill="${shade(panel, 0.55)}"/>` : knobs(4.7, 6.5, 23.5, 5, 0.85)}
      <rect x="2.6" y="7.8" width="24.8" height="9.6" fill="url(#${id})" stroke="${trim}" stroke-width=".4"/>${speakers(s, grille)}`;
  }
  return `<defs>${grillePattern(id, s.grilleType, grille)}</defs>${body}`;
}

export function AmpIcon({ style, className = "amp-ico" }: { style: AmpStyle; className?: string }) {
  const id = `ag${useId().replace(/[^a-zA-Z0-9]/g, "")}`;
  return <svg className={className} viewBox="0 0 30 20" aria-hidden="true" dangerouslySetInnerHTML={{ __html: ampIconInner(style, id) }} />;
}
