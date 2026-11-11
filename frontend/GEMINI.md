# Instructions for the layout agent

Read `README.md` in this folder first. It is the full contract; this file is the
short version for working on the UI.

## Your job

Layout, components and CSS of the POD HD500 Edit web UI (React + TypeScript +
Vite). The backend (Go) and the pedal logic belong to someone else.

You may change:

- `src/components/**`
- `src/styles/**`, `src/*.css`
- `public/gear/*.png`, `public/gear/index.json`

Do not change, even if you think it is wrong. Ask instead:

- `src/api.ts`, `src/store.tsx`, `src/presets.tsx`: the data layer.
- `src/editor/format.ts`: units, rounding and truncation. They were checked
  digit by digit against the pedal's LCD.
- `src/flow/layout.ts`: where blocks go and the drop rules.
- `src/flow/icons.ts`: category colours.
- `src/assign/`: bypass and controller assignment rules and calls. The tab
  only renders what these functions return.
- Anything in `../cmd/`, `../pkg/`, `../docs/`, or the classic UI in
  `../cmd/hd500-edit/web/`.
- Model names, the text of `confirm()` and toast messages.

## How to work

- Develop with `npm run dev:mock`, at `http://localhost:5173/next/`. It serves
  fixture data and refuses every write. Never use `npm run dev` for layout work.
- Never send `POST`, `PUT` or `DELETE` to `/api/...`, not even from a test script.
  Clicking the real editor has already changed a user's preset.
- Click blocks at their centre, away from the top edge. Do not click items in the
  model lists or move sliders unless the task needs it.
- Use the branch `ui`. Do not commit to `dev`.
- Keep the look close to the Helix Native reference: colours, sizes and spacing
  in the screenshots the user gives you.
- No Line 6 images, logos or copied artwork. Amp pictures follow
  `../docs/amp-art-prompts.md`.
- Do not add dependencies without asking.

## Before handing back

```sh
npm run typecheck
npm run build
../scripts/check-ui-scope.sh ui
```

`check-ui-scope.sh` must print `OK`. If it lists files outside the scope, move
those changes to a separate commit and say so in your report.

Report what you changed, what you could not check (for example, a part only the
real pedal shows), and any API you would need.

## Reference screens

Two screens show the target look. Use them for proportions, colours and spacing
only, never to copy artwork, icons or logos.

**Which source to use:**

1. **Live, when REAPER is open:** `http://127.0.0.1:8500/helix.html` shows the
   Helix Native window as it is now. Use it only if the editor runs in debug
   mode and the REAPER bridge is active. If the page says the window is not
   open or the bridge is not running, skip to step 2.
2. **Screenshots otherwise:** the files in `frontend/reference/` (git-ignored).
   They work with no REAPER and no pedal.
3. **Classic UI:** `http://127.0.0.1:8500/`, for the classic look. It needs the
   real editor, wired to the pedal, so it is look-only.

Rules for all of them:

1. **Look, never click.** The Helix page has sliders that change the plugin, and
   the classic UI is wired to the real pedal. A click there has already changed a
   user's preset.
2. **Measure, don't guess.** For a size or colour, sample the image pixel by pixel
   and state the value you found.
