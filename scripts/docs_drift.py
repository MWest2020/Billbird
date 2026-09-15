#!/usr/bin/env python3
# SPDX-License-Identifier: EUPL-1.2
"""Raakt deze push de code zonder dat er docs meebewegen?

De gezaghebbende checker staat in de hub (MWest2020/handbook) en draait op elke
PR. Dit is de lokale spiegel voor de push die géén PR is — in deze repo gaat
lang niet alles via een PR, en wat rechtstreeks naar main gaat komt nooit langs
die workflow.

Ja, dit bestand staat ook in ratatoskr. Dat is bewust en het heeft een grens:
wat hier gekopieerd is, is de *plumbing* (een prefix-match van veertien regels),
niet de *afspraak*. De afspraak is `code_paths`, en die staat per repo op precies
één plek — in de workflow hieronder, waar ook de CI hem uit leest. Zou de
matchlogica ooit iets gaan betekenen (uitzonderingen, per-map-regels), dan hoort
hij naar de hub en niet naar een derde kopie.

Om te voorkomen dat dit een tweede afspraak wordt in plaats van een tweede
handhaving van dezelfde afspraak, leest dit script `code_paths` uit
`.github/workflows/docs-gates.yml`. Staat daar een pad bij, dan geldt het hier
meteen ook. Wie de lijst op één plek aanpast, past hem overal aan.

    git diff --name-only <van> <naar> | python3 scripts/docs_drift.py

Exit 1 bij drift. `DOCS_DRIFT_OK=1` in de omgeving slaat hem over — hetzelfde
ontsnappingsluik als het label `docs-drift-ok` op een PR, en net zo zichtbaar.
"""
import fnmatch
import os
import re
import sys

HIER = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HIER)
WORKFLOW = os.path.join(ROOT, ".github", "workflows", "docs-gates.yml")
DOCS = ("docs/", "README.md")


def code_paths(pad=WORKFLOW):
    """De code-paden uit de workflow. Geen yaml-afhankelijkheid voor één regel,
    maar wel een die hard faalt als de vorm verandert — stil terugvallen op een
    lege lijst zou de gate onzichtbaar uitzetten."""
    with open(pad) as f:
        m = re.search(r'^\s*code_paths:\s*"([^"]*)"', f.read(), re.M)
    if not m:
        sys.exit(f"docs-drift: geen code_paths in {pad} — gate kan niets afdwingen")
    return [p.strip() for p in m.group(1).replace("\n", ",").split(",") if p.strip()]


def raakt(pad: str, patronen: list[str]) -> bool:
    for pat in patronen:
        if pat.endswith("/") and (pad == pat[:-1] or pad.startswith(pat)):
            return True
        if fnmatch.fnmatch(pad, pat) or fnmatch.fnmatch(pad, pat.rstrip("/") + "/*"):
            return True
    return False


def main() -> int:
    paden = [r.strip() for r in sys.stdin if r.strip()]
    if not paden:
        return 0
    patronen = code_paths()
    code = [p for p in paden if raakt(p, patronen)]
    docs = [p for p in paden if p.startswith(DOCS[0]) or p == DOCS[1]]
    if not code or docs:
        return 0
    if os.environ.get("DOCS_DRIFT_OK"):
        print("docs-drift: code zonder docs, maar DOCS_DRIFT_OK staat aan:")
        for p in code[:8]:
            print(f"  {p}")
        return 0
    print("docs-drift: code gewijzigd zonder dat er iets onder docs/ meebeweegt:")
    for p in code[:12]:
        print(f"  {p}")
    print("\nWie code wijzigt, werkt de documentatie in dezelfde push bij.")
    print("Kan het echt niet: DOCS_DRIFT_OK=1 git push — en zeg erbij waarom.")
    return 1


if __name__ == "__main__":
    sys.exit(main())
