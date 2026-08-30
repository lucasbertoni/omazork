# Combat inventory across Zork I/II/III

**Date:** 2026-08-30
**Question:** Enumerate every combat encounter in Zork I/II/III — villains, the exact output text that marks engagement and disengagement, and the attack verbs the parser accepts — precisely enough to spec the wrapper's always-instant in-combat state detector, which sees only the player's raw input and the game's output per turn. (Issue #19, part of #17.)

**Method:** Primary sources only: the `historicalsource/zork{1,2,3}` ZIL trees, read directly. All quoted strings are verbatim from the ZIL (whitespace within strings normalized — ZIL string literals wrap across source lines but print as one flowing paragraph). Zork II and III claims were cross-checked against the compiled assembly shipped in the same repos (`zork2/2actions.zap`, `zork2/zork2str.zap`, `zork3/3actions.zap`, `zork3/zork3str.zap`): every marker string quoted below for those games appears in the compiled output. Zork I ships no `.zap` (only the Z-encoded `COMPILED/zork1.z3`), so its quotes rest on `1actions.zil`/`1dungeon.zil` alone — the usual "dev snapshot may diverge from production" caveat applies, though these particular messages match well-known transcripts. Coverage was cross-checked against the InvisiClues III manuscript bundled in the zork3 repo (`zork3/invisicluesiii.mss`), which mentions exactly the Zork III encounters listed here and no others. Note for zork3: the live source is `3actions.zil` (`zork3/zork3.zil:22-30` inserts `3DUNGEON` and `3ACTIONS`); `shadow.zil`, `actions.zil`, `dungeon.zil`, `verbs.zil`, `syntax.zil` are older duplicates with minor wording drift — everything below cites the live files.

---

## The headline: the three games have three different combat architectures

1. **Zork I is the only game with a real melee engine.** `V-ATTACK` in the shared `gverbs.zil` dispatches to `HERO-BLOW` only when `ZORK-NUMBER` is 1; for Zork II/III it compiles to `<TELL "You can't." CR>` (`zork1/gverbs.zil:176-193`). Zork I has a villain table, per-villain random message tables, a fight demon that makes villains strike back every turn, and a persistent in-combat flag (`FIGHTBIT`). This is the only true multi-turn free combat *state* in the trilogy.
2. **Zork II has no melee engine.** Every "combat" is a bespoke branch in an actor's action routine. Two encounters have real multi-turn hostile state (the dragon's anger counter, the unleashed cerberus); everything else is a one-shot response (often instant death).
3. **Zork III has exactly one fight** — the scripted hooded-figure duel in the Land of Shadow, with its own private strength/counterattack system — plus a handful of one-shot attack responses (several of them instant death or game-ruining).

A detector therefore needs: a real state machine for Zork I, two small state windows for Zork II, one state window for Zork III, and a verb-based instant rule to cover all the one-shots.

---

## Combat verbs the parser accepts (all three games)

Syntax is shared (`gsyntax.zil`, byte-identical across the three trees apart from comments; a `ZORK-NUMBER 2` conditional adds Zork II's bare forms). Line refs are to `zork1/gsyntax.zil`.

| Input pattern | Verb routine | Notes |
|---|---|---|
| `ATTACK actor WITH weapon` | V-ATTACK | synonyms of ATTACK: **FIGHT, HURT, INJURE, HIT** (:96-98) |
| `KILL actor WITH weapon` | V-ATTACK | synonyms of KILL: **MURDER, SLAY, DISPATCH** (:264-266) |
| `ATTACK actor` / `KILL actor` (no WITH) | V-ATTACK | **Zork II only** as literal syntax (:93-94, :261-262). In Zork I/III the bare form still parses because the WITH slot is `(FIND WEAPONBIT)` — the parser auto-supplies a single held weapon, echoing `(with the sword)`. |
| `STAB actor` | V-STAB | picks a held weapon itself, then performs ATTACK; with no weapon: "No doubt you propose to stab the [actor] with your pinky?" (`gverbs.zil:1297`) |
| `STAB actor WITH weapon` | V-ATTACK | (:268-270) |
| `STRIKE actor WITH weapon` | V-ATTACK | (:455-456); bare `STRIKE actor`: "Since you aren't versed in hand-to-hand combat, you'd better attack the [actor] with a weapon." |
| `SWING weapon` / `SWING weapon AT actor` | V-SWING | synonym **THRUST**; with a target it performs ATTACK, alone prints "Whoosh!" (:464-467, `gverbs.zil:1347`) |
| `KNOCK DOWN actor` | V-ATTACK | (:276) |
| `THROW obj AT actor` | V-THROW | synonyms **HURL, CHUCK, TOSS** (:486-495). Generic result is *not* combat: "The [actor] ducks as the [obj] flies by and crashes to the ground." (`gverbs.zil:1445`) — but villain action routines intercept it (thief, troll, dragon, guardians…). |
| `DESTROY/DAMAGE/BREAK/SMASH/BLOCK obj [WITH obj]`, `POKE actor WITH obj` | V-MUNG | (:158-163, :355-358). MUNG is grouped with ATTACK in most Zork II/III villain handlers. |

Universal refusals (same in all three, `gverbs.zil:176-189`): non-actor target → "I've known strange people, but fighting a [obj]?"; no weapon / bare hands → "Trying to attack a [actor] with your bare hands is suicidal."; non-weapon → "Trying to attack the [actor] with a [obj] is suicidal."

**Detector note (input side):** the full trigger vocabulary is ATTACK FIGHT HURT INJURE HIT KILL MURDER SLAY DISPATCH STAB STRIKE SWING THRUST KNOCK POKE MUNG DESTROY DAMAGE BREAK SMASH THROW HURL CHUCK TOSS. Only the first eleven-ish reliably mean melee; THROW/MUNG/DESTROY mostly hit non-combat code paths.

---

## Zork I — the melee engine

### Mechanics (what a state machine must emulate)

- Villain roster: `<GLOBAL VILLAINS <LTABLE <TABLE TROLL SWORD 1 0 TROLL-MELEE> <TABLE THIEF KNIFE 1 0 THIEF-MELEE> <TABLE CYCLOPS <> 0 0 CYCLOPS-MELEE>>>` (`zork1/1actions.zil:3801-3804`). Nothing else in the game melees.
- **Engagement** = the villain's `FIGHTBIT` becoming set, which happens two ways:
  - the player attacks it — `HERO-BLOW` sets `FIGHTBIT` unconditionally (`1actions.zil:3484`);
  - the villain initiates via its action routine's `F-FIRST?` branch, rolled once per turn while it is visible in the room: troll 33 % (`1actions.zil:702-706`), thief 20 % (`1actions.zil:2066-2071`); entering the Treasure Room always sets the thief fighting (`TREASURE-ROOM-FCN`, `1actions.zil:2140-2150`). **There is no dedicated engagement banner for villain initiative** — the first evidence in the output is simply a melee-table attack line (below).
- While engaged, the fight demon `I-FIGHT` (`1actions.zil:3813`) runs **every turn** and prints one `VILLAIN-BLOW` line per fighting villain — this is the per-turn "you are in combat" signal.
- **Disengagement** happens when: the villain dies; the villain is knocked unconscious; the thief flees; or **the player leaves the room** — `I-FIGHT` clears `FIGHTBIT`/`STAGGERED` whenever the villain is no longer in `HERE` (`1actions.zil:3832-3839`). Room change is a hard combat-ender (but the troll blocks exits, below).
- Player weapons the engine recognizes: sword, knife, axe, stiletto, rusty knife (`FIND-WEAPON`, `1actions.zil:3400-3408`).
- Wounds persist and heal on a timer (`I-CURE` every 30 turns); death goes through `JIGS-UP` with the banner `    ****  You have died  ****` (`1actions.zil:4046-4062`).

### Message shape

Every combat line is assembled by `REMARK` from a per-villain table, one line per turn per fighting villain (`1actions.zil:3361-3369`). Player-attack results come from `HERO-MELEE` (`:3611-3650`), villain attacks from `TROLL-MELEE` (`:3689`), `THIEF-MELEE` (`:3735`), `CYCLOPS-MELEE` (`:3654`). Each table has 9 rows: miss, unconscious, killed, light wound, serious wound, stagger, disarm, hesitate, finish-off-helpless.

`HERO-MELEE` lines (result of the player's attack) embed the villain name (`F-DEF`) and weapon (`F-WEP`), e.g. "Clang! Crash! The [villain] parries.", "It's curtains for the [villain] as your [weapon] removes his head.", "The [villain] is staggered, and drops to his knees." **Three lines contain neither name:** "Your stroke lands, but it was only the flat of the blade.", "Slash! Your blow lands! That one hit an artery, it could be serious!", "Slash! Your stroke connects! This could be serious!"

Universal disengagement line — **villain death** (any villain, `VILLAIN-RESULT`, `1actions.zil:3570-3578`):

> "Almost as soon as the [villain] breathes his last breath, a cloud of sinister black fog envelops him, and when the fog lifts, the carcass has disappeared."

Universal player-death-in-combat lines (`WINNER-RESULT`, `:3556-3566`): "It appears that that last blow was too much for you. I'm afraid you are dead." Also each villain table's "killed" row (below) followed by the death banner.

Proximity tell (not combat, but a useful precursor while carrying the sword — `I-SWORD`, `1actions.zil:3851-3878`): "Your sword has begun to glow very brightly." (villain in this room) / "Your sword is glowing with a faint blue glow." (villain one room away) / "Your sword is no longer glowing."

### Troll — The Troll Room (fixed)

- **Presence/engagement:** room description object line "A nasty-looking troll, brandishing a bloody axe, blocks all passages out of the room." (`1dungeon.zil:1036-1046`). Initiates combat 33 %/turn; otherwise first melee line. Exit blocking while alive/conscious: "The troll fends you off with a menacing gesture." (`1dungeon.zil:1488-1490`).
- **Villain-attack lines** (all contain "troll" or "axe"; `TROLL-MELEE`): misses e.g. "The troll swings his axe, but it misses." / "The axe crashes against the rock, throwing sparks!"; knockout "The flat of the troll's axe hits you delicately on the head, knocking you out."; kills "The troll neatly removes your head." / "The troll's axe stroke cleaves you from the nave to the chops."; wounds "The axe gets you right in the side. Ouch!" / "An axe stroke makes a deep wound in your leg."; disarm "The axe knocks your [weapon] out of your hand. It falls to the floor."; hesitation "The troll hesitates, fingering his axe."; finisher "Conquering his fears, the troll puts you to death."
- **Disengagement:**
  - dies → black-fog line (drops the axe first, silently);
  - knocked unconscious → "The [troll] is battered into unconsciousness."-class line, then LDESC becomes "An unconscious troll is sprawled on the floor. All passages out of the room are open." (`TROLL-FCN F-UNCONSCIOUS`, `1actions.zil:671-680`); passages unblock;
  - re-engagement on waking: "The troll stirs, quickly resuming a fighting stance." (`:682-685`);
  - disarmed (troll loses axe) → next turn "The troll, disarmed, cowers in terror, pleading for his life in the guttural tongue of the trolls." — combat effectively suspends; if he recovers it: "The troll, angered and humiliated, recovers his weapon. He appears to have an axe to grind with you." (`F-BUSY?`, `:644-663`);
  - fed a weapon (20 %) → " and eats it hungrily. Poor troll, he dies from an internal hemorrhage and his carcass disappears in a sinister black fog." (`:735-741`); giving him the axe back: "The troll scratches his head in confusion, then takes the axe." (re-arms and re-engages).

### Thief — wanders anywhere; lair is the Treasure Room

The thief visits **every below-ground room** (`RLANDBIT` and not `SACREDBIT`, `I-THIEF`, `1actions.zil:3918-3927`); he won't surface in lit rooms or in the troll's room (`:3956-3958`). Engagement can begin in any dungeon room.

- **Appearance (pre-engagement):** "Someone carrying a large bag is casually leaning against one of the walls here. He does not speak, but it is clear from his aspect that the bag will be taken only over his dead body." (`THIEF-VS-ADVENTURER`, `1actions.zil:1768-1776`). Wander-through lines ("A seedy-looking individual with a large bag just wandered through the room…", "A 'lean and hungry' gentleman just wandered through…") are robbery, not combat.
- **Engagement:** player attack, 20 %/turn initiative while visible, or Treasure Room entry: "You hear a scream of anguish as you violate the robber's hideaway. Using passages unknown to you, he rushes to its defense." then (if treasures present) "The thief gestures mysteriously, and the treasures in the room suddenly vanish." (`1actions.zil:2140-2156`). Missing him with a thrown knife also engages: "You missed. The thief makes no attempt to take the knife… He does seem angered by your attempt." (`ROBBER-FUNCTION`, `:1982-1985`).
- **Villain-attack lines** (all contain "thief" or "stiletto"; `THIEF-MELEE`): misses e.g. "The thief stabs nonchalantly with his stiletto and misses."; knockouts "The thief knocks you out."; kills "Finishing you off, the thief inserts his blade into your heart."; wounds "The thief draws blood, raking his stiletto across your arm."; disarms "The thief neatly flips your [weapon] out of your hands, and it drops to the floor."; hesitation "The thief amuses himself by searching your pockets."; finishers "The thief, forgetting his essentially genteel upbringing, cuts your throat."
- **Disengagement:**
  - **flees when losing** (checked each turn he's fighting): "Your opponent, determining discretion to be the better part of valor, decides to terminate this little contretemps. With a rueful nod of his head, he steps backward into the gloom and disappears." (`1actions.zil:1785-1792`);
  - leaves while not fighting: "The holder of the large bag just left, looking disgusted. Fortunately, he took nothing." / "The thief just left, still carrying his large bag. You may not have noticed that he robbed you blind first." / "The thief, finding nothing of value, left disgusted.";
  - **pacified by a treasure gift**: "The thief is taken aback by your unexpected generosity, but accepts the [treasure] and stops to admire its beauty." (`:2004-2009`; sets `THIEF-ENGROSSED` — he skips his next blow and his strength is capped for it);
  - knocked unconscious → LDESC "There is a suspicious-looking individual lying unconscious on the ground." (`ROBBER-U-DESC`, `:2090-2092`); revives with "The robber revives, briefly feigning continued unconsciousness, and, when he sees his moment, scrambles away from you." (re-engages, `:2078-2083`); "Your proposed victim suddenly recovers consciousness." if you interact first;
  - frightened by a thrown knife (~10 %): "You evidently frightened the robber, though you didn't hit him. He flees, but the contents of his bag fall on the floor." (`:1960-1977`);
  - **dies** → black-fog line, then in the Treasure Room "As the thief dies, the power of his magic decreases, and his treasures reappear:" (itemized list) and "The chalice is now safe to take." (`F-DEAD`, `:2040-2063`).

### Cyclops — Cyclops Room

Two distinct hostile modes, only one of which is engine melee:

- **Wrath countdown (the normal encounter).** Room entry with the cyclops awake starts `I-CYCLOPS`; attacking/throwing feeds it: "The cyclops shrugs but otherwise ignores your pitiful attempt." (MUNG: "\"Do you think I'm as stupid as my father was?\", he says, dodging.") (`CYCLOPS-FCN`, `1actions.zil:1577-1587`). Each turn prints the escalating `CYCLOMAD` series (`:1648-1655`): "The cyclops seems somewhat agitated." → "The cyclops appears to be getting more agitated." → "The cyclops is moving about the room, looking for something." → "The cyclops was looking for salt and pepper…" → "The cyclops is moving toward you in an unfriendly manner." → "You have two choices: 1. Leave  2. Become dinner." — then death: "The cyclops, tired of all of your games and trickery, grabs you firmly. As he licks his chops…" (`I-CYCLOPS`, `:1596-1608`). Leaving the room stops the demon.
- **Engine melee** only if the player wakes the *sleeping* cyclops by attacking: "The cyclops yawns and stares at the thing that woke him up." sets `FIGHTBIT` (`:1531-1539`) and `CYCLOPS-MELEE` fires (he is unarmed and never disarms the player… he *grabs* instead): misses "The Cyclops misses, but the backwash almost knocks you over."; knockout "The Cyclops sends you crashing to the floor, unconscious."; kill "The Cyclops breaks your neck with a massive smash."; wounds "The monster smashes his huge fist into your chest, breaking several ribs."; weapon-grab "The Cyclops grabs your [weapon], tastes it, and throws it to the ground in disgust."; hesitation "The Cyclops seems unable to decide whether to broil or stew his dinner."; finisher "The Cyclops, no sportsman, dispatches his unconscious victim." **One line names no one:** "A quick punch, but it was only a glancing blow."
- **Disengagement:** `ODYSSEUS`/`ULYSSES` — "The cyclops, hearing the name of his father's deadly nemesis, flees the room by knocking down the wall on the east of the room." (`gverbs.zil:945-957`); hot-pepper lunch then water — "The cyclops takes the bottle… and then falls fast asleep (what did you put in that drink, anyway?)." (`1actions.zil:1558-1566`, clears `FIGHTBIT`); or player death. The cyclops cannot be killed by the engine (his strength is 10000; "attacking the cyclops is pointless"-class outcomes).

---

## Zork II — no engine, two hostile states, many one-shots

`V-ATTACK` with a weapon prints "You can't." unless the target's action routine intercepts (`gverbs.zil:190-193` under `ZORK-NUMBER 2`). Bare `ATTACK actor` / `KILL actor` are legal syntax here.

### Dragon — Dragon Room; pursuit through adjacent rooms (multi-turn state)

Everything lives in `DRAGON-FCN`/`I-DRAGON` (`zork2/2actions.zil:2389-2557`) driven by a hidden `DRAGON-ANGER` counter (attack +4, blocked exit +3, talk +2, examine +1; −2/turn decay).

- **Presence:** "A huge red dragon is lying here, blocking the entrance to a tunnel leading north. Smoke curls from his nostrils and out between his teeth." (`2dungeon.zil:484-496`). Blocked exit: "The dragon puts out a claw, grins (all of his sword-sharp teeth glinting in the light), and blocks your way."
- **Engagement (attack with a weapon)** — one of five `DRAGON-ATTACKS` lines (`:2437-2445`): "Dragon hide is tough as steel, but you have succeeded in annoying him a bit. He looks at you as if deciding whether or not to eat you." / "That captured his interest. He stares at you balefully." / "The dragon is surprised and interested (for the moment)." / "You've made him rather angry. You had better be very careful now." / "That did no damage, but he turns his smoky yellow eyes in your direction and sighs." Bare hands: "With your bare hands? I doubt the dragon even noticed."
- **While angry** (per turn): "The dragon follows you, out of mingled curiosity and anger." / "The dragon continues to watch you carefully." Over-provoking (anger > 6) is death: "The dragon tires of this game. With an almost bored yawn, he opens his mouth and incinerates you in a blast of white-hot dragon fire." Trying to slip past: "The dragon doubles back and charges into the room, maddened by your attempt to sneak past him…" (death).
- **Disengagement:** anger decays to 0 → "The dragon seems to have lost interest in you. He wanders off." / "The dragon is no longer around. He must have become bored with you." / idle "The dragon looks bored."; refusal to pursue: "The dragon will follow no further."; **resolution** — leading him to the Glacier Room triggers the long scripted death scene beginning "As the dragon enters, he sees his reflection on the icy surface of the glacier…" and ending "…the melting of the ice has revealed a passage leading west." (`:2525-2545`, +5 score).

### Cerberus-style three-headed dog — Cerberus Room (state = whole room while unleashed)

`CERBERUS-FCN` (`2actions.zil:2298-2355`). While unleashed, **any** un-special interaction prints "The three-headed dog snaps at you viciously!" — the room itself is effectively a combat state.

- **Attack (unleashed):** 50 % instant death "The dog-thing snaps at you viciously, and succeeds. Your head, it seems, is only a small mouthful for the poor animal…", else "The maddened dog-thing snaps viciously at you."
- **Disengagement — pacify:** `PUT COLLAR ON DOG` → "The creature whines happily, then the center head licks your face… Its huge tail wags enthusiastically…"; LDESC becomes "An insipidly grinning three-headed dog is wagging its tail here. It is wearing a huge dog collar."
- Post-pacification: attacking kills it ("With a quiet bark of disappointment, the creature expires… collapses into a small pile of dust"); taking the collar back is death ("…its three fang-crammed mouths rend you into little doggy biscuits.", `COLLAR-FCN`).

### One-shot attack responses (single turn, no state)

| Target (room) | ATTACK/KILL result | Source |
|---|---|---|
| Princess (Dragon's Lair onward) | Wizard appears and kills you: "…\"Fry!\" he intones, and a massive bolt of lightning reduces you to a pile of smoking ashes." | `2actions.zil:2711-2725` |
| Unicorn (garden) | leaves for good: "The unicorn… melts into the hedges and is gone." (loses the key = unwinnable) | `:2653-2660` |
| Gnome of Zurich (Bank vault) | "The gnome says \"Well, I never...\" and disappears with a snap of his fingers, leaving you alone." | `:1498-1503` |
| Doorkeeper lizard (Wizard's door) | "The guardian seems impervious to your attack. In fact, your blows don't even seem to be landing." | `:2959-2962` |
| Baby sea serpent (Aquarium) | "He swims towards you… contents himself with splashing you with water." (GIVE/TAKE is death: "He takes you instead. *Uurrp!*") | `:3156-3170` |
| Wizard of Frobozz | he flees: "The Wizard retreats, waving his wand and chanting. He says \"Fear!\"…" | `:3431-3445` |
| Demon ("genie", Pentagram) | "The demon laughs uproariously."; attack **self**: "\"Foolish mortal, if you insist...\" The demon crushes you with one blow of his enormous hand."; ordering the demon to kill the Wizard is the intended resolution: "…Nothing remains of the Wizard but his wand." | `:3245-3300` |
| Flathead heads (Tomb) | instant death: "Although the Flatheads are dead, they foresaw that some cretin might tamper with their remains…" | `:602-607` |

### False-positive hazard: the Wizard's harassment demon

`I-WIZARD` (`2actions.zil:3455-3620`) randomly produces violent-sounding, non-combat text throughout the game: "A strange little man in a long cloak appears suddenly in the room…", "The Wizard draws forth his wand and waves it in your direction. It begins to glow with a faint blue glow.", "The Wizard, in a deep and resonant voice, speaks the word \"[Feeble|Fumble|Fear|Filch|Freeze|Fall|Ferment|Fierce|Float|Fireproof|Fence|Fantasize]!\" He then vanishes, cackling gleefully." The **Fear** spell then force-moves the player for several turns (random walk). These are hostile-magic events, not combat; a combat detector must not latch on them.

---

## Zork III — one duel, several one-shots

### The hooded figure — Land of Shadow (rooms `SHADOW-1`…`SHADOW-8`, all displayed as "Land of Shadow")

Fully scripted private system in `3actions.zil:3400-3650` (`SHADOW-F`, `SHADOW-ATTACK`, `I-SHADOW-REPLY`, `SHADOW-ROOMS`, `SHADOW-ARRIVAL`): both sides start at strength 5; hits subtract 1 (2 for a "serious" 15 %/10 % roll); both regenerate +1 per 10 turns (`I-CURE`); the player is never killed outright while defenseless — see below — and the figure must be reduced to strength 1, then relieved of its hood.

- **Precursor:** "You can hear quiet footsteps nearby." (30 %/turn while in the area).
- **Engagement (figure appears, 30 %/turn in a lit shadow room):** "Through the shadows, a cloaked and hooded figure appears before you, blocking the [direction]ern exit from the room and carrying a brightly glowing sword." followed, if needed, by "Your sword, glowing wildly, appears in your hand!" or "From nowhere, the sword from the junction appears in your hand, wildly glowing!" (`SHADOW-ARRIVAL`, `:3620-3640`). If the player strikes first instead, the first exchange lines mark engagement. Attacking without the sword: "The hooded figure ignores your feeble attack." (still arms the counterattack demon). During the fight, movement in the blocked direction: "Your way is blocked by the hooded figure."
- **Player-attack lines** (`P-HITS`/`P-MISSES`): hits "A good parry! Your sword wounds the hooded figure!" / "A quick stroke catches the hooded figure off guard! Blood trickles down the figure's arm!" / "The hooded figure is hit with a quick slash!" / serious "A sharp thrust and the hooded figure is badly wounded!"; misses "Your move was not quick enough and misses the mark." / "A quick stroke, but the hooded figure is on guard." / "A good stroke, but it's too slow." / "A good slash, but it misses by a mile." / "You charge, but the hooded figure jumps nimbly aside." / (weak figure) "Your opponent blocks your attack with its sword." Every hit is followed by a diagnosis line "The figure [has a great deal of strength, perhaps matching your own. | has a light wound which hasn't affected its seemingly great strength. | has some wounds and is probably not capable of hindering your movement. | is hurt, and its strength appears to be fading. | appears to be badly hurt and defenseless.]" (`SHADOW-DIAG`).
- **Figure-attack lines** (every turn while engaged, `I-SHADOW-REPLY`/`S-HITS`/`S-MISSES`): hits "The hooded figure catches you off guard and wounds you!" / "You are wounded by a lightning thrust!" / "Your quick reflexes cannot stop the hooded figure's stroke! You are hit!" / serious "A brilliant feint puts you off guard, and the hooded figure slips its sword between your ribs. You are hurt very badly."; at player strength 1 the figure shows mercy: "The hooded figure swings its sword and sends yours flying to the ground. Although you are defenseless, the figure reaches for your sword and hands it back to you, nodding grimly."; misses "The hooded figure stabs nonchalantly with its sword and misses." / "You dodge as the hooded figure comes in low." / "The hooded figure tries to sneak past your guard, but you twist away." / "The hooded figure thrusts, but you fight back and send it flying to the ground!" / (weakened) "The hooded figure attempts a thrust, but its weakened state prevents hitting you." If the sword left the player's hands: "Your sword, glowing wildly, leaps into your hand!"
- **Player death** (only via a serious hit at low strength): "In your wounded state, you cannot defend yourself against your still-quick opponent. Slowly and carefully, the figure starts to remove its hood as you fall to the ground, dead."
- **Disengagement:**
  - **intended resolution** — `TAKE HOOD` at figure strength 1: "You slowly remove the hood from your badly wounded opponent and recoil in horror at the sight of your own face, weary and wounded… The image fades and with it the body of your hooded opponent. The cloak remains on the ground." (`HOOD-F`, `:3560-3580`); too strong: "The hooded figure, though recovering from wounds, is strong enough to force you back." / "You cannot get close enough to the hooded figure to remove the hood.";
  - **killing it** (wrong path — forfeits the cloak/hood): "The hooded figure, fatally wounded, slumps to the ground. It gazes up at you once, and you catch a brief glimpse of deep and sorrowful eyes. Before you can react, the figure vanishes in a cloud of fetid vapor." (`SHADOW-DIES`);
  - **walking out of the shadow rooms** — the counterattack demon self-disables when the figure isn't present; combat resumes only on a fresh appearance.

### Everything else in Zork III is a one-shot

| Target (room) | Result of ATTACK | Source (`3actions.zil`) |
|---|---|---|
| Guardians of Zork (Hall of the Guardians area) | "You aren't close enough, and even if you were, the fight would be a bit one-sided."; THROW anything: "…in front of the Guardians, who destroy it in perfect unison."; **entering their field of view is instant death** (not combat): "The Guardians awake, and in perfect unison, pulverize you with their bludgeons. Satisfied, they resume their posts." — the mirror-box variants say "utterly destroy you with their stone bludgeons" or "two immense stone bludgeons come through the top of the structure, crushing you." | `:794-832`, `:1186-1250` |
| Dungeon Master (Behind Door onward) | instant death: "The dungeon master is taken by surprise. He dodges your blow, and with a disappointed expression on his face, traces a complicated pattern in the air with his staff. You crumble into dust." | `:1373-1377` |
| Old man (Engravings Room) | he leaves forever (unwinnable): "The attack seems to have left the old man unharmed! You watch in awe as he rises to his feet and seems to tower above you. He peers down menacingly, then sadly and wearily. \"Not yet,\" he mourns, and vanishes in a puff of smoke." | `:1848-1857` |
| Man at the cliff (Cliff / Cliff Ledge) | with the sword, a scripted kill: "The man is taken by surprise and is hit with the sword. He grabs you and throws you to the ground[, breaking the staff in the process], but you finish him off with a quick thrust to the chest. He dies, and disappears without ceremony in the usual style of the Great Underground Empire. His assorted valuables remain behind." — note it **breaks the staff** if present (unwinnable). Other weapons: "You wouldn't hurt him with that!"; from below: "It's unlikely you'll succeed at this distance." | `:4379-4397`, `:4452-4460` |
| "Zork IV" sacrificial altar (Scenic Vista teleport) | scripted entry death (robed figure with glowing dagger "…slices you neatly across your abdomen.") — no combat, no input involved | `:4915-4931` |

Zork III also has the Zork-I-style sword glow (`I-SWORD`, `3actions.zil:9-40`: "Your sword is glowing with a faint blue glow." / "no longer glowing") — in practice it signals hooded-figure proximity.

---

## What this means for the detector (spec input)

1. **Zork I needs a real state machine.** Enter in-combat on: (a) input turn whose verb is a melee verb and whose object resolves to troll/thief/cyclops, or (b) any output line matching a villain melee table (villain-initiated combat has **no** banner — the first melee line *is* the engagement marker). Exit on: black-fog death line, unconsciousness lines, thief-flee/left lines, troll-cowers line, pacification lines (treasure gift / cyclops sleep / Odysseus), player death banner, or **room change** (matches engine behavior exactly). While in-combat, every turn's output contains at least one melee line, so a per-turn "still fighting" check is also viable.
2. **Marker-string matching is tractable:** all TROLL-MELEE lines contain "troll" or "axe"; all THIEF-MELEE lines contain "thief" or "stiletto"; CYCLOPS-MELEE lines contain "Cyclops"/"monster" except "A quick punch, but it was only a glancing blow."; HERO-MELEE lines contain the villain's name except the three noted above. Melee lines are interleaved with normal turn output (they print after the player's command result), so match per-line, not per-screen.
3. **Zork II needs two state windows**, not an engine: dragon (from any DRAGON-ATTACKS/follow/block line until wanders-off/bored/glacier-scene/death) and the unleashed dog's room (arguably the whole room visit until the collar text). Everything else is one-shot.
4. **Zork III needs one state window**: hooded-figure fight, from appearance/first-exchange until slumps/hood-removal/player leaves the Land of Shadow. The eight shadow rooms share the display name "Land of Shadow" (already a known constraint for edge keys).
5. **A verb-based instant rule covers all one-shots cheaply**: any input turn using a melee verb (ATTACK/KILL/FIGHT/HIT/HURT/INJURE/MURDER/SLAY/DISPATCH/STAB/STRIKE/SWING/THRUST/KNOCK DOWN, and arguably THROW…AT/MUNG at an actor) should be instant regardless of state. That single rule makes every encounter above instant on the player-action side even where no state is tracked; state windows are then only needed to make the *surrounding* turns (dodging, fleeing, taking the hood, putting on the collar) instant too.
6. **Known false positives to exclude:** Zork II Wizard apparitions and F-spells (including forced Fear-movement), Zork I CYCLOMAD escalation if cyclops-wrath is not treated as combat (recommendation: treat it as combat — it is a timed lethal encounter), vampire-bat abduction (Zork I) and grue deaths (violent text, no combat), Zork III guardians/altar scripted deaths (single-turn, need no state).
7. **Player flight always works as disengagement** except: Zork I troll blocks exits while armed and conscious ("The troll fends you off with a menacing gesture."), the Zork II dragon *follows* (state must survive room changes until a disengage line), and the Zork III figure blocks one exit direction only.
