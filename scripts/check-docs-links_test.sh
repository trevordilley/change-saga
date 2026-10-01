#!/usr/bin/env bash
# Regression: custom deck links are not filesystem paths, and an early matching
# anchor must not fail under pipefail when the producer has more headings.
set -euo pipefail
cd "$(dirname "$0")/.."
fixture_dir="$(mktemp -d "${TMPDIR:-/tmp}/saga-doc-links.XXXXXX")"
trap 'rm -rf "$fixture_dir"' EXIT
{
  printf '# Early anchor\n'
  for i in {1..40}; do printf '\n## Later heading %s\n' "$i"; done
} >"$fixture_dir/target.md"
cat >"$fixture_dir/links.md" <<'LINKS'
[Valid](target.md#early-anchor)
`[Citation](annotation:routing)` and `![Diagram](slide:request-path)`.
LINKS
./scripts/check-docs-links.sh "$fixture_dir/links.md"
printf '\n[Invalid](target.md#absent)\n' >>"$fixture_dir/links.md"
if ./scripts/check-docs-links.sh "$fixture_dir/links.md" >"$fixture_dir/output" 2>&1; then
  printf 'missing anchors must still fail\n' >&2
  exit 1
fi
grep -F 'missing anchor: target.md#absent' "$fixture_dir/output" >/dev/null
printf 'documentation link regressions passed\n'
