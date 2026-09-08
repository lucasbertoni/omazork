# omazork

## Agent skills

### Issue tracker

Issues live as GitHub issues in `lucasbertoni/omazork`, managed via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default vocabulary: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Branches

Work on `develop`. `master` is the branch the Omarchy marketplace installs and must not
contain `CLAUDE.md`, `.claude/` or `docs/agents/`. Publish with `scripts/promote.sh`,
which merges develop into master and strips those paths. Never merge master into develop.
