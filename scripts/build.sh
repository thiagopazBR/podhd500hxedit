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
