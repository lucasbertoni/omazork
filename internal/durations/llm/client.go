package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Inferencer is one call to the model: a rendered prompt in, the model's raw
// text out. The generator only ever talks to this, so a run can be replayed
// against a recorded transcript in tests without touching the network.
type Inferencer interface {
	Infer(ctx context.Context, prompt string) (string, error)
}

// API is the live Anthropic Messages client. It is deliberately hand-rolled
// over net/http: the generator is an offline tool run a handful of times a
// year, and the repo carries no SDK dependency for it.
type API struct {
	Key     string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// NewAPI returns a client for the pinned model.
func NewAPI(key string) *API {
	return &API{
		Key:     key,
		Model:   ModelID,
		BaseURL: "https://api.anthropic.com",
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}
}

type apiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []apiMessage `json:"messages"`
}

type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Infer sends one prompt. The request carries nothing but the pinned model and
// the rendered prompt: everything that shapes an answer has to be inside the
// hash, and the hash is over the model id and the prompt (§5.3). No system
// prompt, no effort setting — an unhashed knob would change answers while
// invalidating nothing. The pinned model reasons adaptively by default and
// rejects a temperature, so determinism rests on the committed cache, not on
// sampling settings.
func (a *API) Infer(ctx context.Context, prompt string) (string, error) {
	var body apiRequest
	body.Model = a.Model
	body.MaxTokens = 2048
	body.Messages = []apiMessage{{Role: "user", Content: prompt}}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", a.Key)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var parsed apiResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", fmt.Errorf("http %d: unreadable response: %s", resp.StatusCode, clip(string(payload)))
	}
	if resp.StatusCode != http.StatusOK {
		if parsed.Error != nil {
			return "", fmt.Errorf("http %d: %s: %s", resp.StatusCode, parsed.Error.Type, parsed.Error.Message)
		}
		return "", fmt.Errorf("http %d: %s", resp.StatusCode, clip(string(payload)))
	}
	if parsed.StopReason == "refusal" {
		return "", fmt.Errorf("the model declined to answer")
	}
	var text strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", fmt.Errorf("no text in the response (stop reason %q)", parsed.StopReason)
	}
	return text.String(), nil
}

// Run infers every request and stores the answers in the cache. One row is one
// call; a bad answer gets exactly one retry with the band restated, and then
// the run hard-fails (§5.3) — the cache is written by the caller only when the
// whole pass succeeded, so a half-inferred cache never lands in the repo.
func Run(ctx context.Context, inf Inferencer, reqs []Request, cache *Cache, progress func(done, total int, key string)) error {
	for i, req := range reqs {
		row, err := inferOne(ctx, inf, req)
		if err != nil {
			return fmt.Errorf("%s: %w", req.Key, err)
		}
		cache.Put(row)
		if progress != nil {
			progress(i+1, len(reqs), req.Key)
		}
	}
	return nil
}

func inferOne(ctx context.Context, inf Inferencer, req Request) (Row, error) {
	prompt := req.Prompt
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		text, err := inf.Infer(ctx, prompt)
		if err != nil {
			// A transport or refusal failure is not something a restated band
			// fixes; fail on it rather than burn a retry.
			return Row{}, err
		}
		row, err := Decode(req, text)
		if err == nil {
			return row, nil
		}
		last = err
		prompt = req.Prompt + "\n\n" + retryNote(err)
	}
	return Row{}, fmt.Errorf("%w (after one retry)", last)
}

// retryNote is the single correction the pass allows itself: it restates what
// went wrong and the shape of a valid answer, and nothing else.
func retryNote(err error) string {
	return "Your previous answer was rejected: " + err.Error() + "\n" +
		"Answer again, staying inside the band of the class you pick.\n" + contract
}
