package lore

import "encoding/json"

type rawMessage = json.RawMessage

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

type toolCallResponse struct {
	IsError           bool            `json:"isError,omitempty"`
	Content           []toolContent   `json:"content,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (r toolCallResponse) Text() string {
	for _, item := range r.Content {
		if item.Text != "" {
			return item.Text
		}
	}
	return "tool returned error"
}

func decodeRaw(raw json.RawMessage, out any) error {
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &DecodeError{Op: "json", Err: err}
	}
	return nil
}

func decodeStructuredContent(raw json.RawMessage, out any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return &DecodeError{Op: "structuredContent", Err: errMissingStructuredContent{}}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &DecodeError{Op: "structuredContent", Err: err}
	}
	return nil
}

type errMissingStructuredContent struct{}

func (errMissingStructuredContent) Error() string { return "missing structuredContent" }
