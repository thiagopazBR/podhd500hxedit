import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const FIXTURES = join(here, "mock", "fixtures");
const MOCK = process.env.MOCK === "1";
const API = process.env.HD500_API ?? "http://127.0.0.1:8500";

// Mock API for layout work without a pedal: GETs are answered from
// mock/fixtures (captured with `npm run mock:capture`); every write is refused
// with 204 and logged, so nothing can reach the pedal from the UI.
function mockApi(): Plugin {
  return {
    name: "hd500-mock-api",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use("/api", (req, res) => {
        const name = new URL(req.url ?? "/", "http://mock").pathname.replace(/^\//, "");
        if (req.method !== "GET") {
          console.warn(`[mock] write ignored: ${req.method} /api/${name}`);
          res.statusCode = 204;
          res.end();
          return;
        }
        if (name === "events") { // Server-Sent Events: one State, then keep open
          res.writeHead(200, { "Content-Type": "text/event-stream", "Cache-Control": "no-cache", Connection: "keep-alive" });
          res.write(`data: ${readFileSync(join(FIXTURES, "state.json"), "utf8").trim()}\n\n`);
          return;
        }
        const file = join(FIXTURES, `${name}.json`);
        if (!existsSync(file)) {
          res.statusCode = 404;
          res.end(`no mock fixture for ${name}: run npm run mock:capture`);
          return;
        }
        res.setHeader("Content-Type", "application/json");
        res.end(readFileSync(file));
      });
    },
  };
}

// The new UI is built into the Go server's web folder and embedded in the
// binary (served at /next/). `npm run dev` serves it live instead.
export default defineConfig({
  base: "/next/",
  plugins: [react(), ...(MOCK ? [mockApi()] : [])],
  server: {
    port: 5173,
    // real editor (127.0.0.1:8500) for read-only viewing; not used in mock mode
    proxy: MOCK ? undefined : { "/api": { target: API } },
  },
  build: {
    outDir: "../cmd/hd500-edit/web/next",
    emptyOutDir: true,
  },
});
