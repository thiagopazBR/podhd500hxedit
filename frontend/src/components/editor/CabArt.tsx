// Speaker cabinet illustration (list thumbnail and HX grid view), built from
// the API's cab style (gear-styles.json "cabs"): the speaker arrangement
// sets the cabinet's proportions (a 1x8 is small, an 8x10 tall), then tolex,
// piping, cloth, the speakers behind it and a blank top plate. Original
// drawings, no logos or copied art.
import { useId } from "react";
import type { AmpStyle } from "../../editor/format";
import { grillePattern, shade } from "./AmpIcon";

const W = 120, H = 84, FLOOR = 80;

// cabinet size (viewBox units) and speaker centres (fractions of the cloth) per arrangement
const SHAPES: Record<string, { w: number; h: number; spk: [number, number][]; r: number; oval?: boolean }> = {
  "1x8": { w: 48, h: 42, spk: [[0.5, 0.5]], r: 0.36 },
  "6x9": { w: 58, h: 40, spk: [[0.5, 0.5]], r: 0.34, oval: true },
  "1x12": { w: 80, h: 58, spk: [[0.5, 0.5]], r: 0.4 },
  "2x12": { w: 106, h: 60, spk: [[0.27, 0.5], [0.73, 0.5]], r: 0.4 },
  "4x10": { w: 70, h: 70, spk: [[0.28, 0.28], [0.72, 0.28], [0.28, 0.72], [0.72, 0.72]], r: 0.21 },
  "4x12": { w: 76, h: 76, spk: [[0.28, 0.28], [0.72, 0.28], [0.28, 0.72], [0.72, 0.72]], r: 0.22 },
  "1x15": { w: 62, h: 66, spk: [[0.5, 0.5]], r: 0.4 },
  "8x10": { w: 46, h: 80, spk: [0.14, 0.38, 0.62, 0.86].flatMap((y) => [[0.28, y], [0.72, y]] as [number, number][]), r: 0.11 },
};

/** Relative luminance 0..1 of "#rrggbb". */
function lum(hex: string) {
  const n = parseInt(hex.replace("#", ""), 16);
  return Number.isNaN(n) ? 0.5 : (0.2126 * (n >> 16) + 0.7152 * ((n >> 8) & 255) + 0.0722 * (n & 255)) / 255;
}

function art(s: AmpStyle, id: string) {
  const shape = SHAPES[s.layout ?? ""] || SHAPES["4x12"];
  const tolex = s.tolex || "#1d1e21", grille = s.grille || "#2a2b2e", trim = s.trim || shade(tolex, 1.6);
  const { w, h } = shape;
  const x = (W - w) / 2, y = FLOOR - h;
  const m = Math.max(3, Math.min(w, h) * 0.07); // tolex border around the cloth
  const plateH = s.panel ? Math.max(4, h * 0.09) : 0; // top strip (logo area, blank)
  const cx = x + m, cy = y + m + plateH, cw = w - 2 * m, ch = h - 2 * m - plateH;
  const open = s.grilleType === "none" && !!s.speakers; // an open port, no cloth (Field Coil)
  const rr = Math.min(cw, ch) * shape.r;
  const speakers = (s.speakers || open) ? shape.spk.map(([fx, fy]) => {
    const sx = cx + cw * fx, sy = cy + ch * fy;
    if (open) return `<circle cx="${sx}" cy="${sy}" r="${rr}" fill="#1f2029" stroke="${shade(grille, 0.55)}" stroke-width="1.2"/>`;
    const r = shape.oval ? undefined : rr;
    return shape.oval
      ? `<ellipse cx="${sx}" cy="${sy}" rx="${cw * 0.3}" ry="${ch * 0.3}" fill="${shade(grille, 0.72)}" fill-opacity=".45"/>`
      : `<circle cx="${sx}" cy="${sy}" r="${r}" fill="${shade(grille, 0.7)}" fill-opacity=".5"/><circle cx="${sx}" cy="${sy}" r="${r! * 0.95}" fill="none" style="stroke:${lum(grille) < 0.25 ? "#9a9da3" : shade(grille, 1.5)};stroke-opacity:${lum(grille) < 0.25 ? ".42" : ".18"};stroke-width:.7"/><circle cx="${sx}" cy="${sy}" r="${r! * 0.36}" fill="${shade(grille, 0.6)}" fill-opacity=".5"/>`;
  }).join("") : "";
  const corners = h >= 66 && s.grilleType !== "tweed" ? [[x, y], [x + w - 6, y], [x, y + h - 6], [x + w - 6, y + h - 6]].map(([px, py]) =>
    `<rect x="${px}" y="${py}" width="6" height="6" rx="1.4" fill="#2b2c30" stroke="#55575d" stroke-width=".5"/>`).join("") : "";
  return `<defs>
      ${grillePattern(`${id}c`, s.grilleType === "none" ? "silver" : s.grilleType, grille).replace("<pattern ", '<pattern patternTransform="scale(1.6)" ')}
      <linearGradient id="${id}g" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".13"/><stop offset=".45" stop-color="#fff" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".22"/></linearGradient>
    </defs>
    <ellipse cx="${W / 2}" cy="${FLOOR + 1.5}" rx="${w * 0.48}" ry="2.2" fill="#000" fill-opacity=".45"/>
    <rect x="${x}" y="${y}" width="${w}" height="${h}" rx="3" fill="${tolex}" stroke="${trim}" stroke-width="1"/>
    <rect x="${x}" y="${y}" width="${w}" height="${h}" rx="3" fill="url(#${id}g)"/>
    ${s.panel ? `<rect x="${cx}" y="${y + m * 0.6}" width="${cw}" height="${plateH}" rx="1" fill="${s.panel}" fill-opacity=".9"/><rect x="${W / 2 - Math.min(14, cw * 0.18)}" y="${y + m * 0.6 + plateH * 0.25}" width="${Math.min(28, cw * 0.36)}" height="${plateH * 0.5}" rx="1" fill="${shade(s.panel, 0.6)}"/>` : ""}
    ${open ? `<rect x="${cx}" y="${cy}" width="${cw}" height="${ch}" rx="1.4" fill="${shade(tolex, 0.92)}" stroke="${shade(trim, 1.1)}" stroke-width=".6"/>`
      : `<rect x="${cx}" y="${cy}" width="${cw}" height="${ch}" rx="1.4" fill="url(#${id}c)" stroke="${shade(trim, 1.15)}" stroke-width=".8"/>`}
    ${speakers}${corners}`;
}

export function CabArt({ style, className = "amp-art" }: { style: AmpStyle; className?: string }) {
  const id = `ca${useId().replace(/[^a-zA-Z0-9]/g, "")}`;
  return <svg className={className} viewBox={`0 0 ${W} ${H}`} aria-hidden="true" dangerouslySetInnerHTML={{ __html: art(style, id) }} />;
}

/** List thumbnail: the same drawing, cropped to the cabinet's own box. */
export function CabIcon({ style }: { style: AmpStyle }) {
  const id = `ci${useId().replace(/[^a-zA-Z0-9]/g, "")}`;
  const sh = SHAPES[style.layout ?? ""] || SHAPES["4x12"];
  // the cabinet's own box (a little margin), at least as wide as the 3:2 thumbnail frame needs
  const bw = Math.max(sh.w + 4, (sh.h + 4) * 1.5), bh = bw / 1.5;
  const vb = `${(W - bw) / 2} ${FLOOR + 2 - bh} ${bw} ${bh}`;
  return <svg className="amp-ico cab-ico" viewBox={vb} preserveAspectRatio="xMidYMax meet" aria-hidden="true" dangerouslySetInnerHTML={{ __html: art(style, id) }} />;
}
