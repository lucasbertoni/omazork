# omazork — Context

Glossary of canonical terms. Keep implementation details out.

## Terms

- **Console**: the slide-from-top surface (an omarchy-shell QML panel/overlay) where the player reads and types. Appears over whatever is running; toggled by keybinding or menu entry. Not a terminal emulator.
- **Story file**: an authentic Infocom `.z3` binary (Zork I/II/III) from the MIT-licensed `historicalsource` repos, bundled with the plugin. The game content; never modified.
- **Z-machine**: the virtual machine that executes a story file, embedded inside the wrapper. Distinct from the games themselves.
- **Wrapper**: the self-contained Go process owning the Z-machine and all game-side logic. The Console talks only to the Wrapper.
- **Mediation layer**: the part of the Wrapper between the player and the Z-machine that shapes pacing — it may hold back an outcome before showing it. The seat of all "casual game" behavior.
- **Outcome**: the game's response to a player action. A *pending outcome* is one the mediation layer has not yet revealed.
- **Picker**: the Console's entry screen: choose Zork I/II/III or resume.
- **Score event**: one of a game's fixed, published point-awarding moments (first entry to certain rooms, taking or depositing a treasure). Triggers achievements; carries no timing.
- **Action wait**: the real-world duration a player action takes to unfold in Casual mode — inferred per action (movement per directed room edge, other actions per verb and object) rather than from score. What delays an outcome.
- **Wait tier**: the presentation register of an action wait, keyed on its resolved length. *Quiet* waits pass with minimal ceremony and never notify; *full* waits get the dramatic treatment.
- **Wait narration**: the player-facing, spoiler-free phrasing of an in-flight action wait, built from the player's own command ("climbing the tree", "heading north"). Names what the player is doing, never the outcome, the destination, or how the wait was priced. Distinct from the wait tier, which is only the register.
- **Drift edge**: a directed room-to-room passage traversed by current or vehicle rather than a walk command (the Frigid River, the balloon). Timed as movement even though the player didn't type a direction.
- **Blocked input**: the Console state while a pending outcome is unmatured, in either wait tier. Game input is unavailable — there is nothing to type into — and only Console shortcuts (menu, close) respond. Ends at maturation, when the input returns.
- **Maturation**: the moment a pending outcome's action wait elapses and it becomes revealable. Revealed on the next Console open.
- **Recap**: the "While you were away…" block shown when a matured full-tier outcome reveals: last command, the outcome, current room and score. Quiet-tier outcomes reveal plainly, without the recap frame.
- **Casual mode**: per-save mode where the mediation layer is active. **Classic mode**: per-save mode that bypasses it entirely; pure Zork.
- **Playthrough**: the durable unit of play — at most one per game. Carries its mode (Classic/Casual), its autosave, and its checkpoints; what "Resume" in the Picker resumes. Starting a new game replaces it after confirmation.
- **Autosave**: the snapshot taken after every turn; a playthrough resumes from it exactly where play left off. Never player-managed.
- **Checkpoint**: a player-created snapshot inside a playthrough, made with the in-game SAVE command; RESTORE returns to one. Distinct from the autosave.
- **Play stats**: passive counters the Wrapper keeps about a playthrough — sessions, deaths, playtime, commands entered. Displayed in the journal sidebar in both modes; reset when a new game replaces the playthrough. Never gamified.
- **Console height**: the share of the screen the Console occupies, from a fifth to the whole of it. Set by the player with Console shortcuts, in effect on every Console screen, and remembered as a preference alongside theme adoption. Defaults to roughly a third.
- **Fullscreen**: the Console at full height. A shortcut takes the Console there and, pressed again, back to the height it left; any ordinary height change while fullscreen forgets that return point. A bookmark, not a mode.
- **Theme adoption**: the Console deriving its entire palette and font from the active omarchy theme, updating live when the theme changes. The default behavior; a light theme yields a genuinely light Console.
- **Phosphor**: the committed built-in green-on-black palette. What the Console shows when the player opts out of theme adoption, and the fallback when adoption fails.
- **Installed**: present on the machine and known to the host shell, but nothing more. Installed plugins land disabled; nothing runs until enabled.
- **Enabled**: the host-shell state that allows the plugin's components to load. Toggled only by the user through the shell — never by the plugin itself.
- **On the bar**: whether the Bar icon occupies a slot in the shell bar's layout. A separate question from Enabled; placement belongs to the user, via the shell's bar commands.
- **Bar icon**: the plugin's presence in the shell bar. Shows omazork is installed and enabled, signals a waiting Recap, reflects Wrapper health, and opens the Console on click.
- **Achievement**: a named milestone unlocked once per game, ever — surviving across playthroughs, in both modes. Triggered by score events or curated non-scoring moments (a first death, finishing the game). May be *hidden*: shown as "???" until unlocked. In Casual mode an achievement on a withheld turn unlocks at the reveal, inside the recap.
