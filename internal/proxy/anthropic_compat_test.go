package proxy

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestNormalizeAnthropicSystemMessages(t *testing.T) {
	t.Parallel()

	t.Run("leaves standard request byte for byte", func(t *testing.T) {
		t.Parallel()
		raw := []byte(`{"model":"anthropic/claude-sonnet-5","messages":[{"role":"user","content":"hi"}]}`)
		got, moved, err := normalizeAnthropicSystemMessages(raw)
		if err != nil {
			t.Fatal(err)
		}
		if moved != 0 || string(got) != string(raw) {
			t.Fatalf("moved=%d body=%s", moved, got)
		}
	})

	t.Run("moves a single system message", func(t *testing.T) {
		t.Parallel()
		raw := []byte(`{
			"model":"anthropic/claude-sonnet-5",
			"messages":[
				{"role":"system","content":"be precise"},
				{"role":"user","content":"hi"}
			]
		}`)
		got, moved, err := normalizeAnthropicSystemMessages(raw)
		if err != nil {
			t.Fatal(err)
		}
		if moved != 1 {
			t.Fatalf("moved = %d", moved)
		}

		var payload struct {
			System   string `json:"system"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(got, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.System != "be precise" {
			t.Fatalf("system = %q", payload.System)
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
			t.Fatalf("messages = %#v", payload.Messages)
		}
	})

	t.Run("merges existing and inline system content", func(t *testing.T) {
		t.Parallel()
		raw := []byte(`{
			"system":"existing",
			"messages":[
				{"role":"system","content":[{"type":"text","text":"inline","cache_control":{"type":"ephemeral"}}]},
				{"role":"assistant","content":"ready"}
			]
		}`)
		got, moved, err := normalizeAnthropicSystemMessages(raw)
		if err != nil {
			t.Fatal(err)
		}
		if moved != 1 {
			t.Fatalf("moved = %d", moved)
		}

		var payload struct {
			System []struct {
				Type         string         `json:"type"`
				Text         string         `json:"text"`
				CacheControl map[string]any `json:"cache_control"`
			} `json:"system"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(got, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.System) != 2 ||
			payload.System[0].Text != "existing" ||
			payload.System[1].Text != "inline" ||
			payload.System[1].CacheControl["type"] != "ephemeral" {
			t.Fatalf("system = %#v", payload.System)
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Role != "assistant" {
			t.Fatalf("messages = %#v", payload.Messages)
		}
	})
}

func TestUnsupportedAnthropicMessageRoles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "standard roles",
			body: `{"messages":[{"role":"user"},{"role":"assistant"}]}`,
		},
		{
			name: "deduplicates and sorts unsupported roles",
			body: `{"messages":[{"role":"tool"},{"role":" developer "},{"role":"tool"}]}`,
			want: []string{"developer", "tool"},
		},
		{
			name: "reports empty role",
			body: `{"messages":[{"role":""}]}`,
			want: []string{"<empty>"},
		},
		{
			name: "invalid JSON is ignored",
			body: `{`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := unsupportedAnthropicMessageRoles([]byte(tt.body)); !slices.Equal(got, tt.want) {
				t.Fatalf("roles = %q, want %q", got, tt.want)
			}
		})
	}
}
