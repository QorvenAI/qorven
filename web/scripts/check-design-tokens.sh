#!/usr/bin/env bash
# Design-token guard: fails if staged web TSX/TS files introduce arbitrary
# font sizes (text-[Npx]) or raw hex colors. Tokens belong in css/*.css.
# Allowlist: the CSS token-definition files, plus the theme-system files
# (theme-provider, appearance-settings) which define the color system by design.
set -euo pipefail
ROOT="$(git rev-parse --show-toplevel)"

# Staged web .tsx/.ts files (exclude css, which legitimately defines colors).
files=$(git diff --cached --name-only --diff-filter=ACM \
  | grep -E '^web/.*\.(tsx|ts)$' \
  | grep -vE '^web/css/|^web/lib/theme-provider\.tsx$|^web/components/settings/sections/appearance-settings\.tsx$' || true)

[ -z "$files" ] && exit 0

violations=""
for f in $files; do
  [ -f "$ROOT/$f" ] || continue
  # arbitrary px font sizes
  hits=$(grep -nE 'text-\[[0-9]+px\]' "$ROOT/$f" || true)
  # raw 6-digit hex in className/style contexts (allow in comments is hard to
  # detect cheaply; we flag all and let the author switch to a token or move to css)
  hits2=$(grep -nE '#[0-9a-fA-F]{6}\b' "$ROOT/$f" || true)
  if [ -n "$hits" ] || [ -n "$hits2" ]; then
    violations="$violations\n--- $f ---\n$hits\n$hits2"
  fi
done

if [ -n "$violations" ]; then
  echo "FAIL: design-token guard — use tokens (bg-primary, text-foreground, …) and the type scale (text-xs/2sm/sm/…), not arbitrary px or raw hex."
  echo -e "$violations"
  echo "If a raw color is genuinely needed (e.g. a brand color), define it as a --channel-*/--connector-* token in web/css/config.qorven.css."
  exit 1
fi
exit 0
