package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPISendsThePinnedModelAndReadsTheText(t *testing.T) {
	var got apiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "secret" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("headers = %v", r.Header)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("body: %v", err)
		}
		io.WriteString(w, `{"content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"{\"class\":\"movement\"}"}],"stop_reason":"end_turn"}`)
	}))
	defer server.Close()

	api := NewAPI("secret")
	api.BaseURL = server.URL
	text, err := api.Infer(context.Background(), "price this")
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if text != `{"class":"movement"}` {
		t.Fatalf("text = %q — only text blocks belong in the answer", text)
	}
	if got.Model != ModelID {
		t.Fatalf("model = %q, want the pinned %q", got.Model, ModelID)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "price this" {
		t.Fatalf("messages = %+v", got.Messages)
	}
}

func TestAPISurfacesFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"api error", http.StatusBadRequest, `{"error":{"type":"invalid_request_error","message":"bad model"}}`, "bad model"},
		{"refusal", http.StatusOK, `{"content":[{"type":"text","text":"no"}],"stop_reason":"refusal"}`, "declined"},
		{"no text", http.StatusOK, `{"content":[],"stop_reason":"max_tokens"}`, "no text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			api := NewAPI("secret")
			api.BaseURL = server.URL
			_, err := api.Infer(context.Background(), "p")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}
