#!/bin/zsh
# Builds and tests the project (no Xcode Command Line Tools needed: cgo is
# disabled, IOKit is reached via purego).
#
#   scripts/build.sh          # tests + all tools into bin/
#   scripts/build.sh re       # also the reverse-engineering tools into re/bin/
set -e
ROOT="${0:A:h:h}"
export CGO_ENABLED=0

# Toolchains: the portable ones in .tools/ if present (the author's setup),
# otherwise whatever is on PATH. Needs Go (version in go.mod) and, for the new
# UI, Node 20+ with npm.
GO="$ROOT/.tools/go/bin/go"
[[ -x "$GO" ]] || GO="$(command -v go || true)"
if [[ -z "$GO" ]]; then
  echo "Go not found: install it from https://go.dev/dl/ (version in go.mod) or put it in .tools/go" >&2
  exit 1
fi
NODE_BIN="$ROOT/.tools/node-v22.12.0-darwin-arm64/bin"
[[ -x "$NODE_BIN/node" ]] && export PATH="$NODE_BIN:$PATH"

cd "$ROOT"
# The new UI (frontend/, React + TypeScript + Vite) builds into
# cmd/hd500-edit/web/next, embedded in the binary and served at /next/. It is not
# in git: it is built here (dependencies are installed on the first run).
if command -v npm >/dev/null 2>&1; then
  if [[ ! -d frontend/node_modules ]]; then
    (cd frontend && npm ci --silent) || { echo "npm ci failed" >&2; exit 1; }
  fi
  (cd frontend && npm run --silent build >/dev/null) && echo "OK: frontend -> cmd/hd500-edit/web/next"
else
  echo "WARNING: Node/npm not found: the new UI (/next/) is NOT built; only the classic UI at / will work" >&2
fi
"$GO" vet ./pkg/... ./cmd/... 2>&1 | grep -v "possible misuse of unsafe.Pointer" | grep -v "^#" || true
"$GO" test ./pkg/...
mkdir -p bin
"$GO" build -o bin/ ./cmd/...
echo "OK: $(ls bin | tr '\n' ' ')"

if [[ "$1" == "re" ]]; then
  cd "$ROOT/re"
  mkdir -p bin
  "$GO" build -o bin/ ./cmd/...
  echo "OK re/bin: $(ls bin | tr '\n' ' ')"
fi
