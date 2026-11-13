// Snapshots (header box + menu). The HD500 has none: the editor keeps 4 per
// preset in files (docs/API.md, "Snapshots"). Off by default; while off the box
// stays dimmed and its menu only offers to turn the feature on. All logic is in
// the API: this only reads GET /api/snapshots and posts the actions.
import { useCallback, useEffect, useState } from "react";
import { getJSON, post, type SnapshotsView } from "../api";
import { useEditor } from "../store";
import { MenuWrap, useMenu } from "./Menu";

const CAMERA = "M2.5 4.8h4.6l1.7-2.6h6.4l1.7 2.6h4.6a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1h-19a1 1 0 0 1-1-1v-10a1 1 0 0 1 1-1z";
const CameraIcon = () => <svg viewBox="0 0 24 18" aria-hidden="true"><path d={CAMERA} /><circle cx="12" cy="10.8" r="4.9" /></svg>;

export function Snapshots() {
  const { state, toast, refetchPreset } = useEditor();
  const online = !!state?.connected;
  const menu = useMenu();
  const [view, setView] = useState<SnapshotsView | null>(null);
  const [active, setActive] = useState(0); // last snapshot applied to this preset (1..4), 0 = none
  const key = online ? `${state?.setlist}:${state?.slot}:${state?.presetName}` : "";

  const load = useCallback(async () => {
    try { setView(await getJSON<SnapshotsView>("/api/snapshots")); } catch { setView(null); }
  }, []);
  useEffect(() => { setActive(0); if (key) void load(); else setView(null); }, [key, load]);

  const fail = (what: string) => (e: Error) => toast(`${what}: ${e.message}`);
  const on = !!view?.enabled;
  const mismatch = view?.mismatch ?? "";

  async function setEnabled(enabled: boolean) {
    menu.setOpen(false);
    try { await post("/api/snapshots/enabled", { enabled }); await load(); } catch (e) { fail("Snapshots")(e as Error); }
  }

  async function apply(n: number) {
    menu.setOpen(false);
    try {
      const res = await post("/api/snapshots/apply", { slot: n });
      const out = (await res.json().catch(() => ({}))) as { changes?: number; skipped?: number[] };
      setActive(n);
      if (out.skipped?.length) toast(`Snapshot ${n} applied; ${out.skipped.length} block(s) skipped because their model changed.`, true);
      void refetchPreset();
    } catch (e) { fail(`Couldn't apply snapshot ${n}`)(e as Error); }
  }

  async function save(n: number) {
    const slot = view?.slots[n - 1];
    if (slot?.saved && !window.confirm(`Replace snapshot ${n} with the current state of the preset?`)) return;
    menu.setOpen(false);
    try {
      try {
        await post("/api/snapshots/save", { slot: n });
      } catch (e) {
        // the file of this slot names another preset (renamed or moved): ask before replacing it
        if (!/would replace/.test((e as Error).message)) throw e;
        if (!window.confirm(`${(e as Error).message}\n\nReplace them with the snapshots of this preset?`)) return;
        await post("/api/snapshots/save", { slot: n, replace: true });
      }
      await load();
      toast(`Saved snapshot ${n}.`, true);
    } catch (e) { fail(`Couldn't save snapshot ${n}`)(e as Error); }
  }

  async function prepare() {
    menu.setOpen(false);
    if (!window.confirm("Set FS5–FS8 of this preset to send CC 80–83 on MIDI OUT (the signal the live player listens to)? This changes the preset's MIDI Assign on the pedal (not saved until you Save to Pedal).")) return;
    try { await post("/api/snapshots/prepare-midi"); await refetchPreset(); toast("FS5–FS8 now send CC 80–83.", true); } catch (e) { fail("Couldn't prepare the footswitches")(e as Error); }
  }

  return (
    <div className="hxh-snapwrap">
      <MenuWrap menu={menu} className="hxh-snapmw">
        <button type="button" className={`hxh-box hxh-snap${on ? "" : " off"}`} disabled={!online} aria-haspopup="menu" aria-expanded={menu.open}
          title={on ? "Snapshots" : "Snapshots (off)"} onClick={menu.toggle}>
          <span className="snap-num"><CameraIcon /><b>{active || ""}</b></span><span>{on && active ? `SNAPSHOT ${active}` : "SNAPSHOTS"}</span>
        </button>
        {menu.open && (
          <div className="menu hxh-snapmenu hxh-snapmenu-wide" role="menu">
            {!on && (
              <>
                <p className="snap-note">The POD HD500 has no snapshots. Turn this on to save up to 4 states per preset (blocks on/off and parameters) and switch between them with a Raspberry Pi live.</p>
                <button type="button" role="menuitem" onClick={() => setEnabled(true)}>Turn on snapshots</button>
              </>
            )}
            {on && (
              <>
                {mismatch && <p className="snap-note warn">{mismatch}</p>}
                {[1, 2, 3, 4].map((n) => {
                  const s = view?.slots[n - 1];
                  return (
                    <div className="snaprow" key={n}>
                      <button type="button" role="menuitem" className={`snap-apply${active === n ? " cur" : ""}`} disabled={!s?.saved || !!mismatch}
                        title={s?.saved ? `Apply snapshot ${n}` : "Empty: save the current state first"} onClick={() => apply(n)}>
                        <span className="snap-num">{n}</span><span>{s?.name || (s?.saved ? `Snapshot ${n}` : "Empty")}</span>
                      </button>
                      <button type="button" className="snap-save" title={`Save the current state to snapshot ${n}`} onClick={() => save(n)}>Save</button>
                    </div>
                  );
                })}
                <hr />
                <button type="button" role="menuitem" title="Make FS5–FS8 send the MIDI the live player listens to" onClick={prepare}>Prepare footswitches (FS5–FS8)</button>
                <button type="button" role="menuitem" onClick={() => setEnabled(false)}>Turn off snapshots</button>
              </>
            )}
          </div>
        )}
      </MenuWrap>
    </div>
  );
}
