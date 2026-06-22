// SPDX-License-Identifier: Apache-2.0
// Copyright 2025 Miniflux Authors. All rights reserved.
// Use of this source code is governed by the Apache 2.0
// license that can be found in the LICENSE file.

package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const (
	apiURL       = "https://api.openai.com/v1/chat/completions"
	responsesURL = "https://api.openai.com/v1/responses"
)

// Call sends prompt to the OpenAI API and returns the parsed JSON response.
// schema is a map of expected keys to a description of acceptable values,
// e.g. {"sentiment": "positive|negative|neutral", "score": "integer 0-100"}.
func Call(prompt, model string, schema map[string]string) (map[string]any, error) {
	schemaBytes, _ := json.Marshal(schema)
	fullPrompt := prompt + "\n\nRespond in JSON format matching this schema: " + string(schemaBytes)

	body, _ := json.Marshal(map[string]any{
		"model":           model,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "user", "content": fullPrompt},
		},
	})

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Error   *struct{ Message string } `json:"error"`
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("openai: %s", envelope.Error.Message)
	}
	if len(envelope.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(envelope.Choices[0].Message.Content), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Message is a chat message exchanged with the OpenAI API.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is a function call requested by the model.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Chat runs a multi-turn conversation (optionally with tools) and returns the
// assistant's reply, which may contain tool calls instead of text content.
func Chat(model string, messages []Message, tools []map[string]any) (*Message, error) {
	payload := map[string]any{"model": model, "messages": messages}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Error   *struct{ Message string } `json:"error"`
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("openai: %s", envelope.Error.Message)
	}
	if len(envelope.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}
	return &envelope.Choices[0].Message, nil
}

// ResponseContentPart is one piece of a Responses API message output.
type ResponseContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ResponseOutputItem is a single item in the Responses API output array. It can
// be an assistant message, a function call requested by the model, or a record
// of a built-in tool invocation (e.g. web search).
type ResponseOutputItem struct {
	Type      string                `json:"type"`
	ID        string                `json:"id"`
	CallID    string                `json:"call_id"`
	Name      string                `json:"name"`
	Arguments string                `json:"arguments"`
	Role      string                `json:"role"`
	Content   []ResponseContentPart `json:"content"`
}

// Text concatenates the textual parts of a message output item.
func (i ResponseOutputItem) Text() string {
	var b bytes.Buffer
	for _, part := range i.Content {
		if part.Type == "output_text" {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

// Respond performs a single round-trip to the OpenAI Responses API. The input
// slice holds conversation items in the Responses API format (role/content
// messages, function_call and function_call_output items). Tools may mix
// built-in tools (such as web search) with custom function tools. The parsed
// output items are returned so callers can resolve any function calls and loop.
func Respond(model, instructions string, input []any, tools []map[string]any) ([]ResponseOutputItem, error) {
	payload := map[string]any{
		"model": model,
		"input": input,
	}
	if instructions != "" {
		payload["instructions"] = instructions
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, responsesURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Error  *struct{ Message string } `json:"error"`
		Output []ResponseOutputItem      `json:"output"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("openai: %s", envelope.Error.Message)
	}
	return envelope.Output, nil
}
