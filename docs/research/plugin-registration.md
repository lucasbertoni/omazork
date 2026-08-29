# Research: Plugin keybinding and menu registration

Resolves [#3](https://github.com/lucasbertoni/omazork/issues/3) (part of map #1).

Investigated 2026-08-29 against primary sources on a live Omarchy quattro install
(`/usr/share/omarchy`, version `4.0.0.alpha`):

- `shell/README.md` — manifest schema, IPC contract ([quattro branch](https://github.com/basecamp/omarchy/blob/quattro/shell/README.md))
- `shell/plugins/README.md` — first-party plugin catalogue, omarchy-menu internals ([quattro branch](https://github.com/basecamp/omarchy/blob/quattro/shell/plugins/README.md))
- `shell/services/PluginRegistry.qml`, `shell/plugins/menu/Menu.qml`, `shell/plugins/menu/MenuModel.js`
- `bin/omarchy-menu`, `bin/omarchy-shell`, `bin/omarchy-plugin-add`
- `default/hypr/` (helpers.lua, omarchy.lua, bindings/*), `~/.config/hypr/bindings.lua`, `~/.config/omarchy/extensions/omarchy-menu.jsonc`
- https://omarchy.org/manual/shell-plugins/ (defers to `shell/README.md`: "the source is the documentation")
- https://omarchyplugins.com (community marketplace; currently 0 plugins, no extra conventions)

## TL;DR

| Question | Answer |
|---|---|
| (a) Can a plugin register a global keybinding? | **No.** Keybindings live only in Hyprland Lua config. The user adds one line to `~/.config/hypr/bindings.lua`; the plugin README instructs it. The installer cannot automate it (it never runs plugin code or hooks). |
| (b) How does a plugin surface an omarchy-menu entry? | **Only via the user's `~/.config/omarchy/extensions/omarchy-menu.jsonc`.** There is no manifest field, no plugin-owned jsonc, and custom `provider:`s are not extensible by third parties. |
| (c) What IPC toggles a panel/overlay? | `omarchy-shell shell toggle <plugin-id> ['<payloadJson>']` — a thin wrapper over `qs ipc -p $OMARCHY_PATH/shell call shell toggle …`. Not hyprctl. |

## (a) Global keybindings

The manifest schema has **no keybinding field**. `PluginRegistry.qml` validates only
`schemaVersion`, `id`, `name`, `version`, `kinds`, `entryPoints` (plus per-kind blocks like
`barWidget`), and nothing in `shell/` uses Quickshell's `GlobalShortcut` (zero grep hits).
The Hyprland config loads bindings exclusively from `default/hypr/bindings/*` and the user's
`~/.config/hypr/bindings.lua` — there is no hook that pulls Lua from
`~/.config/omarchy/plugins/<id>/`.

Every first-party summonable plugin is bound this way in `default/hypr/bindings/*.lua`, e.g.:

```lua
o.bind("SUPER + CTRL + E", "Emojis", "omarchy-shell shell toggle omarchy.emojis")
o.bind("SUPER + CTRL + V", "Clipboard manager", "omarchy-shell shell toggle omarchy.clipboard")
```

So for omazork the user adds to `~/.config/hypr/bindings.lua`:

```lua
o.bind("SUPER + Z", "Zork console", "omarchy-shell shell toggle <our-plugin-id>")
```

The `o.bind(keys, description, dispatcher, options)` helper (`default/hypr/helpers.lua:81`)
wraps `exec` and records the description, which then shows up in the keybindings viewer
(`omarchy menu keybindings --print`) — so a good description doubles as discoverability.

**Can install automate it?** No, by design: `shell/README.md` — "The installer never runs
plugin code, install hooks, or sudo — it only clones files, validates the manifest, and
toggles enabled state over shell IPC." `bin/omarchy-plugin-add` confirms: no binding/menu
edits. The plugin's README must instruct the one-liner (optionally shipping a snippet or a
script the user runs themselves). The commented examples in the stock
`~/.config/hypr/bindings.lua` use exactly this `omarchy-shell shell toggle <id>` pattern,
so it is the blessed convention.

Related nicety: `omarchy plugin clone` routes "existing shortcuts and shell IPC calls made
to the built-in id … to the enabled clone", i.e. bindings address plugin **ids**, and the
shell handles indirection — one more reason the binding line is just an IPC call by id.

## (b) omarchy-menu entry

The menu definition lives outside plugins entirely (`shell/plugins/README.md`, confirmed in
`menu/Menu.qml:50-51`):

- defaults: `$OMARCHY_PATH/default/omarchy/omarchy-menu.jsonc`
- user extensions: `~/.config/omarchy/extensions/omarchy-menu.jsonc` (watched; edits apply
  without restart, user entries win over defaults)

There is **no manifest field** for menu entries and no per-plugin jsonc discovery — a plugin
cannot install a menu entry; the user (or the README's copy-paste snippet) adds one to the
extensions file. Entry shape (from the shipped extensions file header): keys are dotted ids
(`"personal.notes"` nests under `"personal"`), fields are `icon`, `label`, `action` (shell
command; omit for submenu), `target`, `provider`, `aliases`, `description`, `when`,
`checked`. Example for omazork:

```jsonc
"zork": {"icon": "󰅻", "label": "Zork", "action": "omarchy-shell shell toggle <our-plugin-id>"}
```

`aliases` additionally makes it summonable by route: `omarchy menu summon zork`.

**`provider:` is not a third-party extension point.** Providers resolve against a hardcoded
map in `menu/Menu.qml` (`root.providers`: `fonts`, `power-profiles`, plus QML-native `apps`);
an unknown provider name silently does nothing (`var spec = root.providers[entry.provider];
if (!spec) return`). So a static `action:` entry is the only route for us.

A plugin *can* register its own extra **IPC target** (e.g. image-picker's `image-selector`)
via a QML `IpcHandler`, and a menu `action:` can call it — but the entry itself still comes
from the user's extensions jsonc.

## (c) IPC for toggling the panel/overlay

The shell exposes one `shell` IPC target inside the single long-running Quickshell instance
(`shell/README.md`, "IPC contract"):

| Method | Effect |
|---|---|
| `summon <id> <payloadJson>` | load + open a panel/overlay plugin |
| `hide <id>` | close it |
| `toggle <id> <payloadJson>` | summon if closed, hide if open |
| `call <id> <method> <arg>` | call a method on a loaded plugin |

Canonical caller is the **`omarchy-shell` wrapper** (`bin/omarchy-shell`), which is
`qs ipc -n -p "$OMARCHY_PATH/shell" call -- "$@"` with timeout/error handling and
`WAYLAND_DISPLAY` recovery. So:

```bash
omarchy-shell shell toggle omazork.console            # payload optional
omarchy-shell shell toggle omazork.console '{"game":"zork1"}'
omarchy-shell -q ...                                  # best-effort, for bindings
```

`hyprctl` is not involved; Hyprland's only role is running the `exec` from the binding.
`omarchy-menu` itself is the same pattern — a thin wrapper doing
`omarchy-shell shell toggle omarchy.menu '{"menu":"root"}'` (`bin/omarchy-menu`).
Panels/overlays/menus are lazy-loaded on first summon; set `keepLoaded: true` in the
manifest (as `omarchy.emojis` and the image picker do) to keep the window mounted between
summons — right for a game console that must retain state while hidden. The keybind → IPC →
visible path is ~30 ms cold per `shell/plugins/README.md`.

## Consequences for omazork

1. Manifest: `kinds: ["panel"]` or `["overlay"]`, `keepLoaded: true` so the Z-machine
   session survives hide/summon.
2. README ships two copy-paste snippets: one `o.bind(...)` line for
   `~/.config/hypr/bindings.lua`, one jsonc entry for
   `~/.config/omarchy/extensions/omarchy-menu.jsonc`. No way to automate either at install.
3. Everything (keybinding, menu action, scripting) funnels through
   `omarchy-shell shell toggle <id> [payload]`; the payload JSON can carry a picker route
   (e.g. which game) if we want deep links later.
