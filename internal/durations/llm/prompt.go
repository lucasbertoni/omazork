package llm

import (
	"strconv"
	"strings"
	"text/template"

	"github.com/lucasbertoni/omazork/internal/durations"
)

// The three prompt templates (§5.3). They are Go string constants on purpose:
// the rendered prompt is hashed into every row's inputHash, so editing one of
// these invalidates exactly the rows of that kind and nothing else. Keep edits
// deliberate — they cost a re-inference.

const contract = `You are pricing for a slow-burn adaptation of Zork, and your answer is data,
not conversation. Answer with one JSON object and nothing else:
{"class": "<class>", "minutes": <integer>, "rationale": "<one sentence>"}
No prose, no code fence, no extra keys.`

const edgeTemplate = `You are pricing how long one passage between two rooms takes in a
slow-burn, real-time version of Zork. The player types a direction and the outcome is
withheld for the duration you set.

Game: {{.Game}}
Passage: {{.From}} ({{.FromName}}) -> {{.To}} ({{.ToName}}){{if .Dir}}, direction {{.Dir}}{{end}}, exit kind {{.Kind}}
Origin flags: {{join .FromFlags}}
Destination flags: {{join .ToFlags}}
Origin description: {{desc .FromDesc}}
Destination description: {{desc .ToDesc}}

Mechanical rules already priced this passage at {{.Baseline}} minutes ({{.BaselineNote}}).
Your job is only to nudge that baseline for how the passage reads: a short step through a
lit room is quicker than a crawl through a flooded, unlit crevice.

This is a movement row, so class is always "movement" and minutes must be an integer from
{{.BandLow}} to {{.BandHigh}}. Stay near the baseline unless the descriptions justify moving.

` + contract

const routineTemplate = `You are pricing one passage in a slow-burn, real-time version of Zork.
This exit runs a game routine rather than a fixed rule, so classify it yourself.

Game: {{.Game}}
Passage: {{.From}} ({{.FromName}}) -> {{.To}} ({{.ToName}}){{if .Dir}}, direction {{.Dir}}{{end}}
Origin flags: {{join .FromFlags}}
Destination flags: {{join .ToFlags}}
Origin description: {{desc .FromDesc}}
Destination description: {{desc .ToDesc}}
Exit routine {{.Routine}}:
{{source .Source}}

Mechanical rules would price this at {{.Baseline}} minutes, for reference only.

{{classes}}

` + contract

const handlerTemplate = `You are pricing one player action in a slow-burn, real-time version of
Zork. The player types the command and the outcome is withheld for the duration you set, so
price the felt effort of doing it once — not how long the game takes to print a line.

Game: {{.Game}}
Command: {{.Verb}} {{.ObjectName}}
Object: {{.Object}}{{if .ObjectName}} ("{{.ObjectName}}"){{end}}
Object description: {{desc .ObjectDesc}}{{if .Syntax}}
Grammar lines: {{join .Syntax}}{{end}}
Handler {{.Handler}}:
{{source .HandlerSource}}

{{classes}}

Most handler actions are manipulation or mechanism. Nominate "dramatic" only for a set-piece
the game itself treats as a ceremony; a nomination is reviewed by a human and is written to
the table capped at 30 minutes until then.

` + contract

// EdgePayload is the deterministic §5.3 payload for a static or routine edge.
type EdgePayload struct {
	Game         string
	From, To     string
	FromName     string
	ToName       string
	FromDesc     string
	ToDesc       string
	FromFlags    []string
	ToFlags      []string
	Dir          string
	Kind         string
	Baseline     int
	BaselineNote string
	BandLow      int
	BandHigh     int
	Routine      string
	Source       string
}

// HandlerPayload is the payload for one (verb, object) handler pair.
type HandlerPayload struct {
	Game          string
	Verb          string
	Object        string
	ObjectName    string
	ObjectDesc    string
	Syntax        []string
	Handler       string
	HandlerSource string
}

var funcs = template.FuncMap{
	"join": func(items []string) string {
		if len(items) == 0 {
			return "(none)"
		}
		return strings.Join(items, ", ")
	},
	"desc": func(s string) string {
		s = strings.Join(strings.Fields(s), " ")
		if s == "" {
			return "(none recorded)"
		}
		return s
	},
	"source": func(s string) string {
		s = strings.TrimRight(s, "\n")
		if strings.TrimSpace(s) == "" {
			return "(routine text unavailable)"
		}
		return s
	},
	"classes": classTable,
}

var (
	edgeTmpl    = template.Must(template.New("edge").Funcs(funcs).Parse(edgeTemplate))
	routineTmpl = template.Must(template.New("routine").Funcs(funcs).Parse(routineTemplate))
	handlerTmpl = template.Must(template.New("handler").Funcs(funcs).Parse(handlerTemplate))
)

// classTable renders the §2 duration model into the prompt, so the bands the
// validator enforces are the bands the model was shown.
func classTable() string {
	var b strings.Builder
	b.WriteString("Pick exactly one class and an integer number of minutes inside its band:\n")
	for _, c := range []struct {
		class    string
		examples string
	}{
		{"instant", "look, inventory, a failed or no-op turn"},
		{"movement", "one directed room edge"},
		{"manipulation", "take, drop, open, close, put, read"},
		{"mechanism", "inflate the boat, tie the rope, turn the bolt, ring the bell"},
		{"dramatic", "the exorcism, a prayer, a treasure-vault moment"},
	} {
		band, _ := MinuteBand(durations.Class(c.class))
		b.WriteString("  " + c.class + ": " + strconv.Itoa(band.Low) + "-" + strconv.Itoa(band.High) + " min — " + c.examples + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Render builds an edge's prompt from the template its kind is priced by.
func (p EdgePayload) Render(kind Kind) string {
	if kind == KindRoutineEdge {
		return render(routineTmpl, p)
	}
	return render(edgeTmpl, p)
}

// Render builds the prompt for a handler pair.
func (p HandlerPayload) Render() string { return render(handlerTmpl, p) }

// render executes one of the three constant templates over its payload.
func render(t *template.Template, data any) string {
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		// The templates are constants and the payloads are plain structs, so
		// an execution error is a programming mistake, not a data problem.
		panic("llm: render: " + err.Error())
	}
	return b.String()
}
