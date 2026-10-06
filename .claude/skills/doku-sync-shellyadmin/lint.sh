#!/usr/bin/env bash
# Doku-Lint für shellyadmin. Meldet Drift, repariert nichts. Exit 1 bei Fund.
# ponytail: Heuristiken über grep/awk, kein Markdown-Parser — reicht für die Formate dieses Repos.
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2
fund() { echo "FUND  $*"; }
main() {

# 1. Tote relative Links in getrackten .md-Dateien
while IFS= read -r f; do
  d=$(dirname "$f")
  grep -oE '\]\([^)#][^) ]*\)' "$f" | sed -E 's/^\]\(//; s/\)$//; s/#.*//; s/:[0-9]+$//' |
  while IFS= read -r l; do
    case "$l" in http*|mailto:*|"") continue;; esac
    [ -e "$d/$l" ] || echo "FUND  toter Link: $f → $l"
  done
done < <(git ls-files '*.md')

# 2. Pfade in Backticks, die es nicht (mehr) gibt — nur Living Docs, nicht CHANGELOG/ADR/plans (Historie)
for f in CLAUDE.md README.md README.de.md CONTRIBUTING.md docs/*.md; do
  grep -oE '`(internal|cmd|web/src|docs|docker|scripts|\.github)/[^` ]+`' "$f" | tr -d '`' | sed -E 's/:[0-9-]+$//; s/\.[A-Z][A-Za-z]*$//' | sort -u |
  while IFS= read -r p; do
    case "$p" in *'<'*|*'*'*|*'{'*|*'…'*) continue;; esac
    # ponytail: bekannte Nicht-Pfade (Branch-Name, aioshelly-Datei) — Liste erweitern statt Regex verbiegen
    case "$p" in docs/session-lessons|internal/const.py) continue;; esac
    [ -e "$p" ] || echo "FUND  Pfad fehlt: $f → $p"
  done
done

# 3. ADR-Index ↔ Dateien, inkl. Status
for a in docs/adr/0*.md; do
  b=$(basename "$a"); s=$(grep -m1 -oE '^- Status: `[A-Za-z]+`' "$a" | grep -oE '`[A-Za-z]+`')
  line=$(grep -F "$b)" docs/adr/README.md) || { fund "ADR nicht im Index: $b"; continue; }
  [[ "$line" == *"$s"* ]] || fund "ADR-Status weicht ab: $b ist $s, Index sagt: ${line##*)}"
done
for b in $(grep -oE '\./0[0-9]{3}-[^)]+\.md' docs/adr/README.md); do [ -e "docs/adr/$b" ] || fund "Index zeigt ins Leere: $b"; done

# 4. plans/README ↔ Dateien
for p in docs/plans/*.md; do b=$(basename "$p"); [ "$b" = README.md ] && continue
  grep -qF "($b)" docs/plans/README.md || grep -qF "(./$b)" docs/plans/README.md || fund "Plan nicht im Index: $b"; done

# 5. VERSION ↔ CHANGELOG
v=$(tr -d '\n\r ' < VERSION)
grep -q '^## \[Unreleased\]' CHANGELOG.md || fund "CHANGELOG: ## [Unreleased] fehlt"
top=$(grep -m1 -oE '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' CHANGELOG.md | grep -oE '[0-9.]+[0-9]')
[ "$top" = "$v" ] || fund "VERSION=$v, oberster CHANGELOG-Release=$top"
grep -m1 -E '^## \[[0-9]' CHANGELOG.md | grep -vE '^## \[[0-9]+\.[0-9]+\.[0-9]+\] - [0-9]{4}-[0-9]{2}-[0-9]{2} — .+' |
  while IFS= read -r h; do echo "FUND  oberster CHANGELOG-Kopf ohne '— Untertitel' (publish-image braucht ihn): $h"; done

# 6. „Now"-Überschrift / „Planned"-Abschnitt nennt eine Version, die schon raus ist.
#    Bei roadmap nur die Überschrift: im Text darf ein ausgelieferter Release als Ereignis stehen
#    (2026-10-06: „v1.0.0 is cut" löste sonst Fehlalarm aus).
for spec in 'docs/roadmap.md:## Now:head' 'docs/plans/README.md:## Planned:body'; do
  f=${spec%%:*}; rest=${spec#*:}; sec=${rest%:*}; mode=${rest##*:}
  awk -v s="$sec" -v m="$mode" 'index($0,s)==1{if(m=="head"){print;exit} on=1;next} /^## /{on=0} on' "$f" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | sort -u |
  while IFS= read -r x; do
    [ "$(printf '%s\n%s\n' "${x#v}" "$v" | sort -V | tail -1)" = "$v" ] && echo "FUND  $f „${sec#\#\# }\" nennt $x, ausgeliefert ist v$v"
  done
done

# 7. MCP-Toolzahl: Code ↔ Living Docs
ro=$(grep -c 'AddTool' internal/mcp/tools.go); rw=$(grep -c 'AddTool' internal/mcp/tools_actions.go); t=$((ro+rw))
grep -noE '\b[0-9]+[- ]tool(s| surface)\b' CLAUDE.md docs/ARCHITECTURE.md README.md README.de.md 2>/dev/null |
  grep -vE ":${t}[- ]tool" | grep -vE '(8|13)[- ]tool' | while IFS= read -r h; do echo "FUND  MCP-Toolzahl ($t im Code): $h"; done
grep -q "$ro read-only" CLAUDE.md || fund "CLAUDE.md nennt nicht '$ro read-only' (Code: $ro read-only + $rw state-changing)"

# 8. README.md ↔ README.de.md: gleiche Gliederungstiefe
for lvl in '## ' '### '; do
  a=$(grep -c "^$lvl" README.md); b=$(grep -c "^$lvl" README.de.md)
  [ "$a" = "$b" ] || fund "README-Zwilling driftet: '${lvl% }' EN=$a DE=$b"
done

# 9. Required Checks (Branch Protection) in der Doku genannt?
if ctx=$(gh api repos/{owner}/{repo}/branches/main/protection --jq '.required_status_checks.contexts[]' 2>/dev/null); then
  while IFS= read -r c; do for f in CLAUDE.md docs/DEVELOPMENT.md; do
    grep -qF "\`$c\`" "$f" || echo "FUND  Required Check \`$c\` fehlt in $f"; done; done <<<"$ctx"
else echo "WARN  Branch Protection nicht lesbar (gh?) — Check 9 übersprungen"; fi

}
out=$(main 2>&1); printf '%s\n' "$out"
c=$(grep -c '^FUND' <<<"$out"); echo "--- $c Funde"; [ "$c" = 0 ]
