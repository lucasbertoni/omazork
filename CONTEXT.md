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
- **Score event**: one of a game's fixed, published point-awarding moments (first entry to certain rooms, taking or depositing a treasure). The only trigger for delaying an outcome.
- **Maturation**: the moment a pending outcome's wait elapses and it becomes revealable. Revealed on the next Console open.
- **Recap**: the "While you were away…" block shown when a matured outcome reveals: last command, the outcome, current room and score.
- **Casual mode**: per-save mode where the mediation layer is active. **Classic mode**: per-save mode that bypasses it entirely; pure Zork.
- **Playthrough**: the durable unit of play — at most one per game. Carries its mode (Classic/Casual), its autosave, and its checkpoints; what "Resume" in the Picker resumes. Starting a new game replaces it after confirmation.
- **Autosave**: the snapshot taken after every turn; a playthrough resumes from it exactly where play left off. Never player-managed.
- **Checkpoint**: a player-created snapshot inside a playthrough, made with the in-game SAVE command; RESTORE returns to one. Distinct from the autosave.
- **Play stats**: passive counters the Wrapper keeps about a playthrough — sessions, deaths, playtime, commands entered. Displayed in the journal sidebar in both modes; reset when a new game replaces the playthrough. Never gamified.
- **Achievement**: a named milestone unlocked once per game, ever — surviving across playthroughs, in both modes. Triggered by score events or curated non-scoring moments (a first death, finishing the game). May be *hidden*: shown as "???" until unlocked. In Casual mode an achievement on a withheld turn unlocks at the reveal, inside the recap.
