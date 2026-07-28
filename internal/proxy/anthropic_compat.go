package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func normalizeAnthropicSystemMessages(raw []byte) ([]byte, int, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw, 0, nil
	}

	messagesRaw, ok := payload["messages"]
	if !ok {
		return raw, 0, nil
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(messagesRaw, &messages); err != nil {
		return nil, 0, fmt.Errorf("messages must be an array: %w", err)
	}

	filtered := make([]json.RawMessage, 0, len(messages))
	systemValues := make([]json.RawMessage, 0, 2)
	if existing, exists := payload["system"]; exists && !isJSONNull(existing) {
		systemValues = append(systemValues, existing)
	}

	moved := 0
	for _, messageRaw := range messages {
		var message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(messageRaw, &message); err != nil {
			return nil, 0, fmt.Errorf("invalid message: %w", err)
		}
		if strings.TrimSpace(message.Role) != "system" {
			filtered = append(filtered, messageRaw)
			continue
		}

		content := message.Content
		if len(bytes.TrimSpace(content)) == 0 || isJSONNull(content) {
			content = json.RawMessage(`""`)
		}
		systemValues = append(systemValues, content)
		moved++
	}
	if moved == 0 {
		return raw, 0, nil
	}

	filteredJSON, err := json.Marshal(filtered)
	if err != nil {
		return nil, 0, fmt.Errorf("encode messages: %w", err)
	}
	payload["messages"] = filteredJSON

	if len(systemValues) == 1 {
		payload["system"] = systemValues[0]
	} else {
		blocks, err := mergeAnthropicSystemContent(systemValues)
		if err != nil {
			return nil, 0, err
		}
		payload["system"] = blocks
	}

	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("encode request: %w", err)
	}
	return normalized, moved, nil
}

func mergeAnthropicSystemContent(values []json.RawMessage) (json.RawMessage, error) {
	blocks := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
		}

		switch trimmed[0] {
		case '"':
			var text string
			if err := json.Unmarshal(trimmed, &text); err != nil {
				return nil, fmt.Errorf("invalid system text: %w", err)
			}
			block, err := json.Marshal(map[string]any{"type": "text", "text": text})
			if err != nil {
				return nil, fmt.Errorf("encode system text: %w", err)
			}
			blocks = append(blocks, block)
		case '[':
			var valueBlocks []json.RawMessage
			if err := json.Unmarshal(trimmed, &valueBlocks); err != nil {
				return nil, fmt.Errorf("invalid system content blocks: %w", err)
			}
			blocks = append(blocks, valueBlocks...)
		case '{':
			blocks = append(blocks, append(json.RawMessage(nil), trimmed...))
		default:
			return nil, fmt.Errorf("unsupported system content")
		}
	}

	merged, err := json.Marshal(blocks)
	if err != nil {
		return nil, fmt.Errorf("encode system content blocks: %w", err)
	}
	return merged, nil
}

func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func unsupportedAnthropicMessageRoles(raw []byte) []string {
	var payload struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	for _, message := range payload.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "user" || role == "assistant" {
			continue
		}
		if role == "" {
			role = "<empty>"
		}
		seen[role] = struct{}{}
	}

	roles := make([]string, 0, len(seen))
	for role := range seen {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}
