# POD HD500 Edit: frontend

The web UI of the editor: React 19 + TypeScript + Vite. It is a client of the
editor's HTTP API (`../docs/API.md`). It never talks to the pedal directly and
contains no pedal protocol.

## Who owns what

| Area | Files | Owner |
|---|---|---|
| Layout, components, CSS | `src/components/**`, `src/styles/`, `src/*.css` | UI (layout work) |
| API types and calls | `src/api.ts`, `src/store.tsx`, `src/presets.tsx` | Backend side |
| Pedal-related display rules | `src/editor/format.ts` (units, rounding, sign), `src/flow/layout.ts` (where blocks go, drop rules), `src/flow/icons.ts` (category colours) | Backend side; change only with the backend owner |
| Backend | `../cmd/`, `../pkg/`, `../docs/API.md` | Backend side |
| Classic UI | `../cmd/hd500-edit/web/` (except `next/`, which is built from here) | Not changed |

If the UI needs something the API does not provide, ask for a new endpoint.
Do not reproduce pedal logic in the UI.

## Rules

1. **Work on layout with the mock, not the pedal.** `npm run dev:mock`. It answers
   reads from fixtures and refuses every write (`204`, logged as `[mock] write
   ignored`). Mock mode is the default for UI work.
2. **Never call write endpoints by hand** (`POST` to `/api/...`), even from a test.
   Automated clicks in the real editor have already changed a user's preset
   through the "×" and model lists. Click blocks at their centre, away from the
   top edge, and do not click model items unless the test needs it.
3. **Do not change display rules without asking**: units, rounding and truncation
   (validated against the pedal's LCD), "+" sign, model and category names, and
   the text of confirmations and toasts.
4. **Art**: no Line 6 images, logos or copied artwork. Pictures are described in
   `../docs/amp-art-prompts.md`; the editor draws its own when a picture is missing.
5. Save screenshots under `../.playwright-mcp/` (git-ignored).
6. Work in a branch (`ui`), not on `dev`. The backend owner merges.

## Commands

```sh
npm install
npm run dev:mock      # layout work: http://localhost:5173/next/ with fixtures, no pedal
npm run dev           # live: needs the real editor on 127.0.0.1:8500 (reads only in practice)
npm run typecheck
npm run build         # writes ../cmd/hd500-edit/web/next, embedded in the Go binary
npm run mock:capture  # refresh fixtures from the running editor (GET only)
```

`mock/fixtures/` is git-ignored: it holds your presets. Run `npm run mock:capture`
once on your machine before `dev:mock`.

## Layout

- `src/App.tsx`: root layout (header, presets panel, signal flow, editor).
- `src/store.tsx`: live state (Server-Sent Events) and the loaded preset.
- `src/components/Header.tsx`, `Tempo.tsx`, `Tuner.tsx`: header.
- `src/components/SignalFlow.tsx`: the chain (drag, select, on/off).
- `src/components/editor/`: categories, models, parameters, grid view.
- `src/components/presets/PresetsPanel.tsx`: setlists and the 64 slots.
- `src/styles/classic.css`: the classic stylesheet, copied when the UI was split. It
  is the base of the look; move rules into components as the UI is reworked.

## Handing UI work back

Agents: see `GEMINI.md` for the short rules. Before a branch `ui` is merged,
run `../scripts/check-ui-scope.sh ui`: it fails when a changed file is outside the
UI scope (components, styles, CSS, gear pictures).
