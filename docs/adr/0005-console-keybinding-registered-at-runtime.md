# The Console keybinding is registered at runtime by the service

The shell's plugin manifest has no field for keybindings, and Omarchy's
Hyprland config is Lua, so a plugin cannot drop a `.conf` snippet in either.
Until now the README asked the player to add `o.bind("SUPER + Z", …)` to their
own `bindings.lua`. Instead `Service.qml` now registers the binding itself:
on start it runs `scripts/keybind.sh bind`, which issues
`hyprctl eval 'o.bind(…)'` — the same helper a user would write — against the
running compositor. Runtime binds do not survive a config reload, so the
service listens for Hyprland's `configreloaded` event and re-runs the bind.
The key comes from `"keybind"` on the plugin's shell.json entry, defaulting
to `SUPER + Z`; `false` disables registration.

## Considered Options

- **Keep the manual `bindings.lua` line**: zero moving parts, but "install,
  then edit a config file" is not launch-ready, and the binding was the only
  step that could not be done with an `omarchy` command.
- **Hyprland global shortcuts (`hyprland-global-shortcuts`)**: Quickshell
  exposes them, but Hyprland still requires a `bind = …, global, …` line in
  the config, so the user edit remains.
- **`hyprctl keyword bind`**: refused by the Lua config provider ("keyword
  can't work with non-legacy parsers. Use eval.").

## Consequences

- The script is polite: it skips binding when the combination already belongs
  to something else (a user's own `bindings.lua` entry wins, and a duplicate
  would make the two toggles cancel each other out), and `unbind` only removes
  a binding carrying its own description, `Zork console`.
- The service unbinds on destruction only when the registry reports the
  plugin disabled. The shell also destroys and recreates services on rescans
  and during startup, where an unconditional unbind raced the successor's
  bind and left the key dead.
- The bind is visible in `omarchy menu keybindings --print` with its
  description, like a config-file bind.
- Dev-loop note: `omarchy-shell shell rescanPlugins` reinstates services from
  the cached component, so a change to `Service.qml` needs
  `omarchy restart shell` (what `scripts/dev.sh` already does).
