// The large picture of an amp or cab in the grid view: an image from
// public/gear/<art>.png when there is one (listed in public/gear/index.json),
// otherwise our own drawing (AmpArt / CabArt).
import { useEffect, useState } from "react";
import type { AmpStyle } from "../../editor/format";
import { AmpArt } from "./AmpArt";
import { CabArt } from "./CabArt";

const BASE = `${import.meta.env.BASE_URL}gear/`;
let available: Promise<Set<string>> | null = null;
const loadAvailable = () =>
  (available ??= fetch(`${BASE}index.json`).then((r) => (r.ok ? r.json() : [])).then((l: string[]) => new Set(l)).catch(() => new Set<string>()));

export function GearPicture({ style, kind }: { style: AmpStyle; kind: "amp" | "cab" }) {
  const [has, setHas] = useState<Set<string> | null>(null);
  const [broken, setBroken] = useState(false);
  useEffect(() => { void loadAvailable().then(setHas); }, []);
  if (style.art && has?.has(style.art) && !broken) {
    return <img className="gear-pic" src={`${BASE}${style.art}.png`} alt="" draggable={false} onError={() => setBroken(true)} />;
  }
  return kind === "cab" ? <CabArt style={style} /> : <AmpArt style={style} />;
}
