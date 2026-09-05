# omazork

Zork I, II, and III as an [Omarchy](https://omarchy.org) shell plugin: a
phosphor console that slides down over whatever you're doing, running the
authentic Infocom story files through an embedded Z-machine. No external
dependencies — one static Go binary the plugin bootstraps itself.

Two ways to play, chosen per playthrough:

- **Classic** — the games exactly as shipped in the 1980s.
- **Casual** — a mediation layer paces play: every action takes the time it
  plausibly would — a minute or two to cross a room, longer to inflate a
  boat, hours for the rare dramatic moment — and its outcome is withheld
  until that time has passed. Short waits pass quietly ("Time passes.");
  from five minutes up, close the console, live your life, and come back to
  a "While you were away…" recap and one desktop notification. Looking,
  combat, failed turns, and deaths never wait.

Autosave every turn (a shell restart resumes invisibly), checkpoints via
in-game SAVE/RESTORE, per-playthrough stats, and lifetime achievements.

## Install

Requires a current Quickshell-based Omarchy ("quattro").

```sh
omarchy plugin add https://github.com/lucasbertoni/omazork.git --enable
```

Plugins can't register keybindings or menu entries themselves, so add each
with one line:

**Keybinding** — in `~/.config/hypr/bindings.lua`:

```lua
o.bind("SUPER + Z", "Zork console", "omarchy-shell shell toggle omazork")
```

**Menu entry** — in `~/.config/omarchy/extensions/omarchy-menu.jsonc`:

```jsonc
"zork": { "icon": "󰊠", "label": "Zork", "action": "omarchy-shell shell toggle omazork", "aliases": ["zork"] }
```

**Bar icon** — a brass lantern in the shell bar: lit while the engine runs,
dark when it isn't, with a hollow dot while a Casual wait unfolds and a filled
dot once the recap is waiting. Click toggles
the console; hover shows game and mode (never spoilers). Fresh enables place
it automatically; if omazork was already enabled before the icon existed,
re-enable it with a placement (progress is autosaved; note this rewrites the
plugin's shell.json entry, so re-apply any `"theme"` setting after):

```sh
omarchy plugin disable omazork && omarchy plugin enable omazork --after omarchy.clock
```

**Theme** — the console follows the active omarchy theme, including light
themes and live theme switches. To keep the original green-on-black phosphor
look instead, set it on the plugin's entry in `~/.config/omarchy/shell.json`:

```jsonc
"plugins": [{ "id": "omazork", "theme": "phosphor" }]
```

With the bar icon placed, the entry lives in `bar.layout` instead of
`plugins`; set it there with `omarchy bar set omazork theme '"phosphor"'`.

On first launch the overlay bootstraps the engine binary into `bin/`: it
downloads the checksum-pinned static build for your architecture
(x86_64/aarch64) from the GitHub Release, falling back to `go build` if a Go
toolchain is installed. While this repository is private, the anonymous
download 404s — you need either an authenticated `gh` CLI or a Go toolchain.

Updates arrive through `omarchy plugin update omazork`; the bootstrap notices
the new pinned checksum on the next launch and refetches.

## Development

```sh
omarchy plugin validate .   # validate the working copy (it rejects symlinked paths)
ln -s "$PWD" ~/.config/omarchy/plugins/omazork
omarchy plugin enable omazork
touch bin/DEV   # bootstrap always go-builds, never clobbers with the release binary
```

`go test ./...` covers the engine wrapper, mediation, saves, achievements, and
the NDJSON protocol (`docs/protocol.md`). Cut a release with
`scripts/release.sh vX.Y.Z` and push the tag; CI rebuilds with the same
flags, verifies the pinned checksums in `scripts/engine.version`, and attaches
the binaries to the GitHub Release.

## Licensing and trademarks

The plugin code is MIT (see [LICENSE](LICENSE)). The bundled `.z3` story
files are the authentic Infocom binaries released under the MIT License by
Microsoft in the `historicalsource` repositories — provenance, checksums, and
license text in [assets/games/LICENSE-zork.md](assets/games/LICENSE-zork.md).

**Zork and Infocom trademarks are not licensed.** The MIT release covers the
source code and story files only. This project is not affiliated with or
endorsed by Microsoft, Activision, or Infocom, and uses the game names solely
to identify the games.
