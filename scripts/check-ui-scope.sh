#!/bin/zsh
# Checks that a UI branch only changes UI files (who owns what: frontend/README.md).
#
#   scripts/check-ui-scope.sh [branch=ui] [base=dev]
#
# Exit 0: every changed file is UI (components, styles, CSS, gear pictures).
# Exit 1: lists the files outside that scope. Exit 2: branch missing.
set -e
ROOT="${0:A:h:h}"
cd "$ROOT"

BRANCH="${1:-ui}"
BASE="${2:-dev}"

if ! git rev-parse --verify -q "$BRANCH" >/dev/null; then
  echo "no branch '$BRANCH' (create it first: git branch $BRANCH dev)"
  exit 2
fi

# Paths the UI may change. Everything else belongs to the backend side.
allowed() {
  case "$1" in
    frontend/src/components/*) return 0 ;;
    frontend/src/styles/*) return 0 ;;
    frontend/src/*.css) return 0 ;;
    frontend/public/gear/*.png) return 0 ;;
    frontend/public/gear/index.json) return 0 ;;
  esac
  return 1
}

changed=("${(@f)$(git diff --name-only --no-renames "$BASE...$BRANCH")}")
if [[ -z "${changed[1]}" ]]; then
  echo "$BRANCH has no changes against $BASE"
  exit 0
fi

outside=()
for f in "${changed[@]}"; do
  allowed "$f" || outside+=("$f")
done

if (( ${#outside[@]} )); then
  echo "OUT OF SCOPE in $BRANCH (changed against $BASE):"
  printf '  %s\n' "${outside[@]}"
  echo "Ask the backend side to take these changes, or move them to a separate branch."
  exit 1
fi

echo "OK: ${#changed[@]} file(s) in $BRANCH, all inside the UI scope"
