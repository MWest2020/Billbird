# Tasks: apply-docs-contract

- [ ] 1.1 Werk op de branch waarop je gestart bent — maak GEEN eigen
      branch aan; de habitat-harness beheert branches en pusht.
- [ ] 2.1 `docs/`-structuur aanleggen volgens het contract; bestaande docs
      migreren zoals beschreven in proposal.md (repo-specifiek); stubs
      achterlaten waar externe links kunnen bestaan.
- [ ] 2.2 Front matter op elke pagina: gemigreerd-zonder-review =
      `status: draft` + `last_reviewed` = migratiedatum.
- [ ] 2.3 `docs/index.md`: één alinea wat het project is, status, link naar
      README, links naar de aanwezige secties.
- [ ] 2.4 `.mcp.json` in de root plaatsen (template uit de seed; placeholder `TODO-change-3` laten staan).
- [ ] 3.1 Zelfcheck tegen het contract: alleen toegestane submappen dragen
      markdown, elke pagina heeft front matter, één taal (English).
- [ ] 4.1 PR openen met titel `docs: apply handbook docs contract`; body vinkt
      per contractpunt af wat is toegepast + vermeldt de punten die de
      proposal als "PR-body" markeert. STOP daarna: Mark merget.
