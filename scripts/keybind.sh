#!/usr/bin/env bash
# Register (or drop) the Console keybinding in the running Hyprland.
#
#   keybind.sh bind   "SUPER + Z"
#   keybind.sh unbind "SUPER + Z"
#
# Omarchy configures Hyprland in Lua, so `hyprctl keyword bind` is refused;
# `hyprctl eval` runs the same o.bind()/hl.unbind() helpers a user would
# write in bindings.lua. The bind is runtime-only — a config reload wipes
# it — so the service re-runs `bind` on every configreloaded event.
#
# Idempotent and polite: the key is left alone when it is already bound to
# something else (the user's own bindings.lua wins), and `unbind` only
# removes a binding this script made, recognised by its description.
set -euo pipefail

DESCRIPTION="Zork console"
COMMAND="omarchy-shell shell toggle omazork"

action="${1:-}"
keys="${2:-}"

fail() { echo "omazork keybind: $*" >&2; exit 1; }

[[ $action == bind || $action == unbind ]] || fail "usage: keybind.sh bind|unbind \"MODS + KEY\""
[[ -n $keys ]] || exit 0
[[ $keys =~ ^[A-Za-z0-9_+[:space:]]+$ ]] || fail "refusing keybinding with unexpected characters: $keys"
command -v hyprctl >/dev/null 2>&1 || fail "hyprctl not found"

# "SUPER + SHIFT + Z" → modmask 65, key Z — the shape `hyprctl binds -j` reports.
modmask=0
key=""
IFS='+' read -ra parts <<<"$keys"
for part in "${parts[@]}"; do
  part=$(tr -d '[:space:]' <<<"$part")
  [[ -n $part ]] || continue
  case "${part^^}" in
    SHIFT) modmask=$((modmask | 1)) ;;
    CAPS | CAPSLOCK) modmask=$((modmask | 2)) ;;
    CTRL | CONTROL) modmask=$((modmask | 4)) ;;
    ALT | MOD1) modmask=$((modmask | 8)) ;;
    MOD2) modmask=$((modmask | 16)) ;;
    MOD3) modmask=$((modmask | 32)) ;;
    SUPER | WIN | LOGO | MOD4) modmask=$((modmask | 64)) ;;
    MOD5) modmask=$((modmask | 128)) ;;
    *) [[ -z $key ]] || fail "more than one key in: $keys"; key="$part" ;;
  esac
done
[[ -n $key ]] || fail "no key in: $keys"

# Description of whatever holds the combo now; "" when it is free.
holder=$(hyprctl binds -j | jq -r --argjson mods "$modmask" --arg key "$key" '
  map(select(.modmask == $mods and (.key | ascii_downcase) == ($key | ascii_downcase) and .submap == ""))
  | if length == 0 then "" else (.[0].description // "(no description)") end')

lua_str() { printf '"%s"' "$(sed 's/["\\]/\\&/g' <<<"$1")"; }

case "$action" in
  bind)
    if [[ $holder == "$DESCRIPTION" ]]; then
      exit 0 # already ours
    elif [[ -n $holder ]]; then
      echo "omazork keybind: $keys is already bound to '$holder'; not overriding" >&2
      exit 0
    fi
    hyprctl eval "o.bind($(lua_str "$keys"), $(lua_str "$DESCRIPTION"), $(lua_str "$COMMAND"))" >/dev/null
    ;;
  unbind)
    [[ $holder == "$DESCRIPTION" ]] || exit 0 # not ours to remove
    hyprctl eval "hl.unbind($(lua_str "$keys"))" >/dev/null
    ;;
esac
