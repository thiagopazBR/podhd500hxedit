#!/bin/zsh
# Builds and tests the project with the portable Go toolchain kept in .tools/go
# (no Xcode Command Line Tools needed: cgo is disabled, IOKit is reached via purego).
#
#   scripts/build.sh          # tests + all tools into bin/
#   scripts/build.sh re       # also the reverse-engineering tools into re/bin/
set -e
ROOT="${0:A:h:h}"
GO="$ROOT/.tools/go/bin/go"
export CGO_ENABLED=0

cd "$ROOT"
# The new UI (frontend/, React + TypeScript + Vite) builds into
# cmd/hd500-edit/web/next, embedded in the binary and served at /next/.
NODE_BIN="$ROOT/.tools/node-v22.12.0-darwin-arm64/bin"
if [[ -d frontend/node_modules && -x "$NODE_BIN/node" ]]; then
  (cd frontend && PATH="$NODE_BIN:$PATH" npm run --silent build >/dev/null) && echo "OK: frontend → web/next"
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
