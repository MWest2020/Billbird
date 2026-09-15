#!/usr/bin/env bash
# Install a pre-push guard in this clone: pushing to `main` checks that the docs
# moved with the code.
#
# Waarom lokaal en niet alleen in CI: in deze repo gaat lang niet alles via een
# PR. Wat rechtstreeks naar main gaat komt nooit langs de docs-gates-workflow, en
# dat is precies waar documentatie stilletjes achterloopt.
#
# Het is een guard, geen hek. `--no-verify` komt erlangs, en `DOCS_DRIFT_OK=1`
# ook — allebei zichtbaar. Wat het tegenhoudt is het echte faalgeval: per
# ongeluk code pushen en de docs vergeten.
#
#   bash scripts/install_hooks.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOOKS="$(git -C "$ROOT" rev-parse --git-path hooks)"
[ -d "$HOOKS" ] || HOOKS="$ROOT/$HOOKS"
mkdir -p "$HOOKS"

cat > "$HOOKS/pre-push" <<'HOOK'
#!/bin/sh
# Written by scripts/install_hooks.sh.
root=$(git rev-parse --show-toplevel)
while read -r _local _lsha remote _rsha; do
    case "$remote" in
        refs/heads/main)
            case "$_rsha" in
                *[!0]*) van="$_rsha" ;;
                *)      van=$(git merge-base "$_lsha" origin/main 2>/dev/null) ;;
            esac
            [ -n "$van" ] || continue
            if ! git diff --name-only "$van" "$_lsha" \
                    | (cd "$root" && python3 scripts/docs_drift.py) >&2; then
                echo "pre-push: GEWEIGERD — zie hierboven." >&2
                exit 1
            fi
            ;;
    esac
done
exit 0
HOOK
chmod +x "$HOOKS/pre-push"
echo "pre-push-guard geïnstalleerd in $HOOKS"
