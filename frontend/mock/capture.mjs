// Saves the editor's current read-only data as mock fixtures. GET requests
// only: it never writes to the pedal. Needs the real editor running.
// Fixtures contain your presets, so they stay out of git (see .gitignore).
import { mkdirSync, writeFileSync } from "node:fs";

const API = process.env.HD500_API ?? "http://127.0.0.1:8500";
const out = new URL("./fixtures/", import.meta.url);
mkdirSync(out, { recursive: true });

const paths = {
  state: "/api/state",
  meta: "/api/meta",
  models: "/api/models",
  preset: "/api/preset",
  setlists: "/api/setlists",
  setlist: "/api/setlist?n=0", // mock serves this for any setlist
};
for (const [name, path] of Object.entries(paths)) {
  const res = await fetch(API + path);
  if (!res.ok) throw new Error(`${path} answered ${res.status}`);
  writeFileSync(new URL(`${name}.json`, out), await res.text());
  console.log("saved", name);
}
