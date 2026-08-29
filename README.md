# omazork

Zork I, II, and III as an [Omarchy](https://omarchy.org) shell plugin: a
phosphor console that slides down over whatever you're doing, running the
authentic Infocom story files through an embedded Z-machine. No external
dependencies — one static Go binary the plugin bootstraps itself.

Two ways to play, chosen per playthrough:

- **Classic** — the games exactly as shipped in the 1980s.
- **Casual** — a mediation layer paces the big moments: when your score
  changes, the outcome is withheld and matures over minutes to hours. Close
  the console, live your life, and come back to a "While you were away…"
  recap. Combat and deaths never wait.

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

**Theme** — the console follows the active omarchy theme, including light
themes and live theme switches. To keep the original green-on-black phosphor
look instead, set it on the plugin's entry in `~/.config/omarchy/shell.json`:

```jsonc
"plugins": [{ "id": "omazork", "theme": "phosphor" }]
```

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
