// Large amp illustration for the model grid (HX Edit's grid view): a front
// view in a flat, lightly shaded style, built from the same API style as the
// list thumbnail (gear-styles.json: combo / head, tolex, piping, control
// panel, cloth, speakers). Original drawings: blank name plates, no logos,
// no copied art.
import { useId } from "react";
import type { AmpStyle } from "../../editor/format";
import { KNOB_COLORS, grillePattern, shade } from "./AmpIcon";

const W = 120, H = 84;

function knobRow(n: number, x0: number, x1: number, y: number, r: number, color: string, rim: string) {
  return Array.from({ length: n }, (_, i) => {
    const x = x0 + ((x1 - x0) * i) / (n - 1);
    return `<circle cx="${x.toFixed(2)}" cy="${y}" r="${r}" fill="${color}" stroke="${rim}" stroke-width=".45"/>` +
      `<path d="M${x.toFixed(2)} ${(y - r * 0.85).toFixed(2)}V${(y - r * 0.25).toFixed(2)}" style="stroke:${rim};stroke-width:.5;fill:none"/>`;
  }).join("");
}

// speakers seen through the cloth (or an open port when there's no cloth)
function speakerSet(s: AmpStyle, grille: string, x: number, y: number, w: number, h: number) {
  const n = s.speakers ?? 0;
  if (!n) return "";
  const open = s.grilleType === "none";
  const cx = x + w / 2, cy = y + h / 2;
  const at: [number, number, number][] = n === 1 ? [[cx, cy, Math.min(w, h) * 0.4]]
    : n === 2 ? [[x + w * 0.27, cy, Math.min(w * 0.22, h * 0.42)], [x + w * 0.73, cy, Math.min(w * 0.22, h * 0.42)]]
    : [[x + w * 0.3, y + h * 0.28, h * 0.22], [x + w * 0.7, y + h * 0.28, h * 0.22], [x + w * 0.3, y + h * 0.72, h * 0.22], [x + w * 0.7, y + h * 0.72, h * 0.22]];
  return at.map(([sx, sy, r]) => open
    ? `<circle cx="${sx}" cy="${sy}" r="${r}" fill="#1f2029" stroke="${shade(grille, 0.55)}" stroke-width="1.2"/><circle cx="${sx}" cy="${sy}" r="${r * 0.92}" fill="none" style="stroke:#000;stroke-opacity:.35;stroke-width:1"/>`
    : `<circle cx="${sx}" cy="${sy}" r="${r}" fill="${shade(grille, 0.72)}" fill-opacity=".45"/><circle cx="${sx}" cy="${sy}" r="${r * 0.42}" fill="${shade(grille, 0.62)}" fill-opacity=".45"/>`).join("");
}

function art(s: AmpStyle, id: string) {
  const tolex = s.tolex || "#2a2a2a", panel = s.panel || "#202020", grille = s.grille || "#3a3a3a";
  const trim = s.trim || shade(tolex, 0.6), knob = KNOB_COLORS[s.knob ?? ""] || KNOB_COLORS.chrome;
  const rim = s.knob === "black" ? "rgba(255,255,255,.45)" : "rgba(0,0,0,.55)";
  const strap = shade(tolex, 0.42);
  // the cloth pattern of the thumbnail, scaled up
  const defs = `<defs>
    ${grillePattern(`${id}c`, s.grilleType, grille).replace("<pattern ", '<pattern patternTransform="scale(1.6)" ')}
    <linearGradient id="${id}g" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".14"/><stop offset=".45" stop-color="#fff" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".22"/></linearGradient>
  </defs>`;
  const shadow = `<ellipse cx="${W / 2}" cy="${H - 3}" rx="50" ry="2.6" fill="#000" fill-opacity=".45"/>`;
  const jewel = (x: number, y: number) => `<circle cx="${x}" cy="${y}" r="1.7" fill="#d9302c" stroke="${shade(panel, 0.5)}" stroke-width=".5"/><circle cx="${x - 0.5}" cy="${y - 0.5}" r=".55" fill="#fff" fill-opacity=".6"/>`;
  const jacks = (x: number, y: number) => `<circle cx="${x}" cy="${y}" r="1.4" fill="#111" stroke="${shade(panel, 0.6)}" stroke-width=".5"/><circle cx="${x + 4.2}" cy="${y}" r="1.4" fill="#111" stroke="${shade(panel, 0.6)}" stroke-width=".5"/>`;
  const plate = (x: number, y: number, w: number) => `<rect x="${x - w / 2}" y="${y}" width="${w}" height="4.6" rx="1.2" fill="${shade(panel, 0.55)}" stroke="${shade(trim, 1.2)}" stroke-width=".5"/>`;

  if (s.layout === "head") {
    const x = 6, y = 24, w = 108, h = 46;
    return defs + shadow +
      // handle, feet / corners
      `<path d="M50 ${y + 0.5}V${y - 4.2}Q50 ${y - 6.4} 52.4 ${y - 6.4}H67.6Q70 ${y - 6.4} 70 ${y - 4.2}V${y + 0.5}" fill="none" style="stroke:${strap};stroke-width:2.6;stroke-linecap:round"/>` +
      `<rect x="${x + 4}" y="${y + h - 1}" width="8" height="3.6" rx="1" fill="#111"/><rect x="${x + w - 12}" y="${y + h - 1}" width="8" height="3.6" rx="1" fill="#111"/>` +
      `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="3.6" fill="${tolex}" stroke="${trim}" stroke-width="1"/>` +
      `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="3.6" fill="url(#${id}g)"/>` +
      // upper front: cloth / plate band with a blank name plate
      `<rect x="${x + 5}" y="${y + 5}" width="${w - 10}" height="17" rx="1.2" fill="url(#${id}c)" stroke="${shade(trim, 0.8)}" stroke-width=".6"/>` +
      plate(W / 2, y + 11.2, 26) +
      // control panel: inputs, knobs, switches, jewel light
      `<rect x="${x + 5}" y="${y + 25}" width="${w - 10}" height="15" rx="1" fill="${panel}" stroke="${shade(panel, 0.6)}" stroke-width=".5"/>` +
      `<rect x="${x + 5}" y="${y + 25}" width="${w - 10}" height="15" rx="1" fill="url(#${id}g)"/>` +
      jacks(x + 11, y + 32.5) + knobRow(8, x + 26, x + 84, y + 32.5, 2.3, knob, rim) +
      `<rect x="${x + 90}" y="${y + 29.5}" width="2.4" height="6" rx=".6" fill="#222"/><rect x="${x + 95}" y="${y + 29.5}" width="2.4" height="6" rx=".6" fill="#222"/>` + jewel(x + 101.5, y + 32.5);
  }
  // combo
  const x = 10, y = 7, w = 100, h = 72;
  const top = 15; // control panel height
  return defs + shadow +
    `<path d="M48 ${y + 0.5}V${y - 3.4}Q48 ${y - 5.4} 50.4 ${y - 5.4}H69.6Q72 ${y - 5.4} 72 ${y - 3.4}V${y + 0.5}" fill="none" style="stroke:${strap};stroke-width:2.6;stroke-linecap:round"/>` +
    `<rect x="${x + 6}" y="${y + h - 1}" width="9" height="3.4" rx="1" fill="#111"/><rect x="${x + w - 15}" y="${y + h - 1}" width="9" height="3.4" rx="1" fill="#111"/>` +
    `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="4.2" fill="${tolex}" stroke="${trim}" stroke-width="1"/>` +
    `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="4.2" fill="url(#${id}g)"/>` +
    // control panel along the top
    `<rect x="${x + 5}" y="${y + 4}" width="${w - 10}" height="${top}" rx="1" fill="${panel}" stroke="${shade(panel, 0.6)}" stroke-width=".5"/>` +
    `<rect x="${x + 5}" y="${y + 4}" width="${w - 10}" height="${top}" rx="1" fill="url(#${id}g)"/>` +
    // controls on the front panel, or none seen when they sit on top of the cabinet (a blank name plate there)
    (s.controls === "top" ? plate(W / 2, y + 4 + top / 2 - 2.3, 30)
      : jacks(x + 11, y + 4 + top / 2) + knobRow(7, x + 26, x + 82, y + 4 + top / 2, 2.3, knob, rim) +
        `<rect x="${x + 88}" y="${y + 4 + top / 2 - 3}" width="2.4" height="6" rx=".6" fill="#222"/>` + jewel(x + 96, y + 4 + top / 2)) +
    // cloth with the speakers behind it, piping around it, a blank name plate
    `<rect x="${x + 5}" y="${y + top + 7}" width="${w - 10}" height="${h - top - 12}" rx="1.6" fill="url(#${id}c)" stroke="${shade(trim, 1.15)}" stroke-width=".9"/>` +
    speakerSet(s, grille, x + 5, y + top + 7, w - 10, h - top - 12) +
    (s.controls === "top" ? "" : plate(x + 22, y + top + 10, 18));
}

export function AmpArt({ style, className = "amp-art" }: { style: AmpStyle; className?: string }) {
  const id = `aa${useId().replace(/[^a-zA-Z0-9]/g, "")}`;
  return <svg className={className} viewBox={`0 0 ${W} ${H}`} aria-hidden="true" dangerouslySetInnerHTML={{ __html: art(style, id) }} />;
}
