# Theme adoption binds to the host shell's Color singleton

The Console adopts the active omarchy theme by importing `qs.Commons` and binding to omarchy-shell's `Color` singleton — the same source the built-in shell plugins use — rather than reading `~/.local/state/omarchy/current/theme/colors.toml` ourselves and reloading via a `theme-set.d` hook. The singleton gives instant hot-reload on theme switch (the shell pushes new colors over IPC), respects the user's `shell.toml` overrides, and needs no file I/O or hook plumbing; the cost is coupling to undocumented shell internals, which we accept because the plugin already depends on the shell's manifest/overlay contract and dies with the shell anyway.

## Consequences

- All 17 palette roles are derived from only the keys the singleton exposes (`foreground`, `background`, `accent`, `muted`) via lightness-aware tint/shade/alpha math — we never read `colors.toml` directly, even for keys it has that the singleton lacks (`selection`, `dark_background`, ANSI colors). One source of truth; every role hot-reloads.
- Light themes are honored for real: derivations flip around the theme background's lightness, since the singleton exposes no light/dark mode flag.
- A shell refactor of `qs.Commons` breaks theming; the Phosphor palette remains the failure fallback.
