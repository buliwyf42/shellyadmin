---
name: doku-sync-shellyadmin
description: shellyadmin-Doku nach Änderungen nachziehen — Lint (Links, ADR-/Plan-Index, VERSION↔CHANGELOG, README-Zwilling, MCP-Toolzahl, Required Checks), CLAUDE.md-Lektionen, Roadmap. Aufrufen nach Code-/Release-/Betriebsarbeit an shellyadmin oder mit „/doku-sync-shellyadmin".
---

Ziehe die shellyadmin-Doku auf den Ist-Zustand nach. **Die Doku ist Englisch und das Repo
öffentlich** (`buliwyf42/shellyadmin`) — alles, was hier geschrieben wird, liest die Welt mit.

**Ausgabe nie abschneiden** (`| head`/`| tail` macht aus einem Teilergebnis eins, das wie das
vollständige aussieht). Volle Ausgabe lesen oder in eine Datei schreiben und `grep -c` zählen.

1. **Wer arbeitet sonst noch?** Einmal messen, bevor irgendetwas angefasst wird:

   ```bash
   git fetch -q origin && git log --oneline HEAD..origin/main
   git log --since=midnight --format='%h %ad %s' --date=format:%H:%M
   git worktree list
   ```

   `--since=midnight`, nicht `today` (git setzt dort die aktuelle Uhrzeit ein → 0 Treffer).
   Fremde Commits von heute oder fremde Worktrees mit frischem Reflog → parallele Session;
   nichts löschen/umbenennen, was ihr gehören könnte (CLAUDE.md §„`gh pr merge --delete-branch`").

2. **Lint:** `bash .claude/skills/doku-sync-shellyadmin/lint.sh` — Exit 1 bei Fund. Prüft:
   tote relative `.md`-Links · Backtick-Pfade in Living Docs, die es nicht gibt · ADR-Index ↔
   `docs/adr/*.md` inkl. Status · `docs/plans/README.md` ↔ Dateien · `VERSION` = oberster
   CHANGELOG-Release, `[Unreleased]` vorhanden, Kopf mit `— Untertitel` · `roadmap.md` „Now" /
   `plans/README.md` „Planned" nennen keine schon ausgelieferte Version · MCP-Toolzahl in
   CLAUDE.md/ARCHITECTURE.md = `AddTool`-Aufrufe im Code · `README.md` ↔ `README.de.md`
   gleiche Gliederung · jeder Required Check der Branch Protection steht in CLAUDE.md und
   `docs/DEVELOPMENT.md`.
   **Jeden Fund abarbeiten, nicht nur melden.** Ein Fehlalarm wird im Skript behoben (Ignore-Liste
   in Check 2), nicht wegdiskutiert. `WARN Branch Protection nicht lesbar` heißt: Check 9 lief
   **nicht** — ein grüner Lauf beweist dann nichts über die Checks.

3. **Was hat sich geändert?** Kurz auflisten: Code (Tools, Settings-Knöpfe, Migrationen,
   Endpunkte), Release/Deploy, gemessene Betriebsbefunde (Flotte, Scan, OTA), Entscheidungen.

4. **Lint sieht keine Wahrheit — diese Stellen von Hand gegen den Code halten**, wenn die Session
   sie berührt hat:
   - neues MCP-Tool → Liste in CLAUDE.md §MCP (13 read-only / 8 state-changing **mit Namen**),
     `docs/ARCHITECTURE.md`, `docs/adr/0011-…` (Addendum), README §MCP.
   - neuer Settings-Knopf → CLAUDE.md-Tabelle „App Settings" (Default/Bounds aus `Normalize()`).
   - neue Datei mit Schlüsselrolle → CLAUDE.md „Key Files".
   - neue versiegelte Spalte → `RotateSealedColumns` (sonst verwaist sie bei Key-Rotation).
   - neue Endpunkte → ADR-0018 / OpenAPI-Abdeckung.
   - Toolchain-Zahlen **nie als Prosa** in die Doku schreiben — `Toolchain sync` in
     `.github/workflows/test.yml` ist die Quelle (zweimal veraltet, siehe CLAUDE.md §Architecture).

5. **Betriebslektionen in `CLAUDE.md`** (Shelly-Quirks, Fleet-Messungen, Fallen) — Stil der
   bestehenden Abschnitte: Überschrift mit ISO-Datum, Messwerte mit Messdatum, `🩸` für den
   übertragbaren Teil, **was es NICHT belegt** ausdrücklich. Veraltetes **korrigieren statt
   anhängen**; widerlegte Aussagen als widerlegt markieren, nicht löschen (siehe OTA-Abschnitt).
   Flüchtige Werte (IPs, Gerätezahl, Versionen der Flotte) als **Prüfbefehl** oder mit Datum —
   die Flottengröße wandert. **Öffentliches Repo:** keine Tokens, keine Passwörter, keine
   `op://`-Pfade mit Inhalt; LAN-IPs/MACs stehen schon drin und sind ok.

6. **CHANGELOG `[Unreleased]`** laufend pflegen: Features, Fixes und **CI-Gates** kommen mit
   ihrem PR hinein (Stil: `Toolchain sync`-Eintrag, v0.6.x), nicht erst beim Release —
   nur Dependabot-Bumps sammelt der Release-Commit. Kein Lint sieht einen fehlenden Eintrag
   (2026-10-06: `Docs lint` wurde Pflicht-Check, ohne dass es im CHANGELOG stand).
   **Release-Doku**, falls released wurde (Ablauf: `docs/DEVELOPMENT.md`): `VERSION` +
   `web/package.json` + Lockfile synchron, CHANGELOG-Kopf `## [X.Y.Z] - YYYY-MM-DD — Untertitel`,
   `roadmap.md` „Recently shipped" / „Now", `plans/README.md` Planned → Shipped.
   **Merge ≠ Deploy:** läuft der Container die neue Version? (CLAUDE.md §„Deploying a release").

7. **README-Zwilling:** Änderung an `README.md` → dieselbe an `README.de.md` (und umgekehrt).
   Bewusste Asymmetrie (z. B. ein Abschnitt nur EN) wird in **beiden** Dateien begründet, sonst
   meldet Check 8 sie bei jedem Lauf.

8. **Memory** (Projekt-Memory unter `~/.claude/projects/<projekt>/memory/`):
   gleiche Regel — Veraltetes korrigieren. Was im Repo steht, gehört nicht in die Memory.

9. **Prozess-Nachschärfung:** Hat in **diesem** Lauf etwas konkret weh getan (Fehlalarm, blinder
   Check, Fund, den kein Check sah)? Dann Skill oder `lint.sh` nachschärfen. Kein Vorschlag ohne
   Beleg aus dem Lauf.

10. **Committen — über PR, nicht direkt auf `main`** (Branch Protection + Auto-Mode-Guard):

    ```bash
    git switch -c docs/<thema>
    git commit --only <dateien> -m "docs: …"
    git push -u origin HEAD && gh pr create --fill
    ```

    `--only` statt `git add -A`: committet nur die genannten Pfade, auch wenn eine Parallelsession
    etwas gestaget hat. Unerwartete Änderungen im Baum melden, nicht mitnehmen. Danach
    `bash .claude/skills/doku-sync-shellyadmin/lint.sh` erneut: **0 Funde und Exit 0** ist die Abnahme.
    Skill und `lint.sh` sind versioniert (`.claude/skills/`); der Rest von `.claude/` bleibt
    gitignored. Eine Nachschärfung aus Schritt 9 geht also über denselben PR.
