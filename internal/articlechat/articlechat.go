// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package articlechat // import "miniflux.app/v2/internal/articlechat"

import (
	"errors"
	"fmt"
	"strings"

	"miniflux.app/v2/internal/openai"
	"miniflux.app/v2/internal/reader/processor"
	"miniflux.app/v2/internal/reader/sanitizer"
	"miniflux.app/v2/internal/storage"
)

const (
	// modelName must support the Responses API web search tool together with
	// custom function tools. Promote to config if you switch models.
	modelName = "gpt-4.1"
	// searchToolType is the built-in web search tool for the Responses API.
	searchToolType = "web_search_preview"

	summaryMaxLen = 6000  // cap the feed summary injected into the system prompt
	articleMaxLen = 12000 // cap scraped full-article text returned to the model
	maxToolRounds = 6     // bound the tool loop so a confused model can't spin forever
)

// Message is a single chat turn exchanged with the browser.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Agent answers questions about a single article. It can read the feed-provided
// summary, search the web, and fetch the full article on demand.
type Agent struct {
	store   *storage.Storage
	userID  int64
	entryID int64
}

// NewAgent builds an agent bound to one user and one article.
func NewAgent(store *storage.Storage, userID, entryID int64) (*Agent, error) {
	return &Agent{store: store, userID: userID, entryID: entryID}, nil
}

func (a *Agent) tools() []map[string]any {
	return []map[string]any{
		{"type": searchToolType},
		{
			"type":        "function",
			"name":        "fetch_full_article",
			"description": "Fetch and return the complete text of the article the user is reading, scraped from its source URL. The feed only provides a short summary, so call this when the summary is not enough to answer. This can fail (paywalls, blocked requests, network errors); when it does, rely on the summary and web search instead.",
			"parameters": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
	}
}

// instructions builds the system prompt embedding the article context.
func (a *Agent) instructions() (string, error) {
	entry, err := a.store.NewEntryQueryBuilder(a.userID).
		WithEntryIDs(a.entryID).
		GetEntry()
	if err != nil {
		return "", err
	}
	if entry == nil {
		return "", errors.New("articlechat: article not found")
	}

	var b strings.Builder
	b.WriteString("You are a helpful research assistant embedded in the Miniflux feed reader. ")
	b.WriteString("The user is reading the article described below and may ask questions about it or about related topics.\n\n")
	fmt.Fprintf(&b, "Title: %s\n", entry.Title)
	fmt.Fprintf(&b, "URL: %s\n", entry.URL)
	if entry.Author != "" {
		fmt.Fprintf(&b, "Author: %s\n", entry.Author)
	}
	if entry.Feed != nil && entry.Feed.Title != "" {
		fmt.Fprintf(&b, "Source: %s\n", entry.Feed.Title)
	}
	if !entry.Date.IsZero() {
		fmt.Fprintf(&b, "Published: %s\n", entry.Date.Format("2006-01-02"))
	}

	summary := strings.TrimSpace(sanitizer.StripTags(entry.Content))
	if summary == "" {
		summary = "(the feed did not provide any summary text)"
	} else if len(summary) > summaryMaxLen {
		summary = summary[:summaryMaxLen] + "…"
	}
	fmt.Fprintf(&b, "\nArticle summary / feed content:\n%s\n\n", summary)

	b.WriteString("Guidelines:\n")
	b.WriteString("- The summary above may only be an excerpt. Call fetch_full_article when you need the complete text. Be aware the fetch may fail (paywalls, blocking, network errors); if it does, say so briefly and answer using the summary and web search.\n")
	b.WriteString("- Use web search for current events, related topics, or to verify facts, and cite sources when relevant.\n")
	b.WriteString("- Answer concisely in the user's language, using plain text suitable for a small chat window.\n")
	return b.String(), nil
}

// fetchFullArticle scrapes the article's source URL and returns its text. The
// returned string is always safe to hand back to the model, including on error.
func (a *Agent) fetchFullArticle() string {
	entry, err := a.store.NewEntryQueryBuilder(a.userID).
		WithEntryIDs(a.entryID).
		GetEntry()
	if err != nil || entry == nil {
		return "Failed to load the article from the database. Use the summary above and web search instead."
	}

	feed, err := a.store.NewFeedQueryBuilder(a.userID).
		WithFeedID(entry.FeedID).
		GetFeed()
	if err != nil || feed == nil {
		return "Failed to load the feed configuration needed to fetch the article. Use the summary above and web search instead."
	}

	user, err := a.store.UserByID(a.userID)
	if err != nil {
		return "Failed to load user settings needed to fetch the article. Use the summary above and web search instead."
	}

	if err := processor.ProcessEntryWebPage(feed, entry, user); err != nil {
		return "Could not fetch the full article (" + err.Error() + "). Use the summary above and web search instead."
	}

	text := strings.TrimSpace(sanitizer.StripTags(entry.Content))
	if text == "" {
		return "The article was fetched but no readable text could be extracted (it may be behind a paywall or rendered with JavaScript). Use the summary above and web search instead."
	}
	if len(text) > articleMaxLen {
		text = text[:articleMaxLen] + "\n…(truncated)"
	}
	return "Full article text:\n" + text
}

// Chat runs one assistant turn over the conversation history, resolving any web
// search and fetch_full_article tool calls, and returns the assistant's reply.
func (a *Agent) Chat(history []Message) (string, error) {
	instructions, err := a.instructions()
	if err != nil {
		return "", err
	}

	input := make([]any, 0, len(history)+4)
	for _, m := range history {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		input = append(input, map[string]any{"role": m.Role, "content": m.Content})
	}

	tools := a.tools()

	for range maxToolRounds {
		output, err := openai.Respond(modelName, instructions, input, tools)
		if err != nil {
			return "", err
		}

		var pendingCalls []openai.ResponseOutputItem
		var reply strings.Builder
		for _, item := range output {
			switch item.Type {
			case "function_call":
				pendingCalls = append(pendingCalls, item)
			case "message":
				reply.WriteString(item.Text())
			}
		}

		if len(pendingCalls) == 0 {
			return reply.String(), nil
		}

		for _, call := range pendingCalls {
			// Echo back the model's function_call before its output, as the
			// Responses API requires the call/output pair to be paired by call_id.
			input = append(input, map[string]any{
				"type":      "function_call",
				"call_id":   call.CallID,
				"name":      call.Name,
				"arguments": call.Arguments,
			})

			result := "Unknown tool."
			if call.Name == "fetch_full_article" {
				result = a.fetchFullArticle()
			}

			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": call.CallID,
				"output":  result,
			})
		}
	}

	return "", fmt.Errorf("articlechat: tool loop exceeded %d rounds", maxToolRounds)
}
