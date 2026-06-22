// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ai // import "miniflux.app/v2/internal/ai"

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/openai"
)

const modelName = "gpt-4.1-nano"

// Category is a single AI label definition.
type Category struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var (
	categories    []Category
	validNames    map[string]bool
	schemaValue   map[string]string
	promptSection string
)

func init() {
	data, err := os.ReadFile("categories.json")
	if err != nil {
		panic("ai: categories.json not found in working directory: " + err.Error())
	}
	if err := json.Unmarshal(data, &categories); err != nil {
		panic("ai: failed to parse categories.json: " + err.Error())
	}

	validNames = make(map[string]bool, len(categories))
	names := make([]string, 0, len(categories))
	lines := make([]string, 0, len(categories))
	for _, c := range categories {
		validNames[c.Name] = true
		names = append(names, `"`+c.Name+`"`)
		lines = append(lines, "- "+c.Name+": "+c.Description)
	}
	promptSection = strings.Join(lines, "\n")
	schemaValue = map[string]string{
		"categories": "array — pick all that apply: " + strings.Join(names, ","),
	}
}

// Categories returns the loaded category definitions (for UI use).
func Categories() []Category {
	return categories
}

// LabelEntry returns AI categories for an entry based on its title.
func LabelEntry(entry *model.Entry) ([]string, error) {
	if entry.Title == "" {
		return []string{"inne"}, nil
	}

	prompt := fmt.Sprintf(
		"Categorize this news article title: \"%s\"\n\nCategory definitions:\n%s",
		entry.Title, promptSection,
	)

	result, err := openai.Call(prompt, modelName, schemaValue)
	if err != nil {
		return nil, err
	}

	list, _ := result["categories"].([]any)
	var cats []string
	for _, v := range list {
		if s, ok := v.(string); ok && validNames[s] {
			cats = append(cats, s)
		}
	}
	if len(cats) == 0 {
		return []string{"inne"}, nil
	}
	// inne must be sole label — drop it when other labels are present
	if len(cats) > 1 {
		filtered := cats[:0]
		for _, c := range cats {
			if c != "inne" {
				filtered = append(filtered, c)
			}
		}
		cats = filtered
	}
	return cats, nil
}

// -- Progress tracking --

type userProgress struct {
	total   int64
	labeled int64
}

var mu sync.Mutex
var byUser = map[int64]*userProgress{}

func RegisterWork(userID int64, count int) {
	if count == 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	p, ok := byUser[userID]
	if !ok {
		p = &userProgress{}
		byUser[userID] = p
	}
	p.total += int64(count)
}

func MarkLabeled(userID int64) {
	mu.Lock()
	defer mu.Unlock()
	p, ok := byUser[userID]
	if !ok {
		return
	}
	p.labeled++
	if p.labeled >= p.total {
		delete(byUser, userID)
	}
}

func GetProgress(userID int64) (total, labeled int64) {
	mu.Lock()
	defer mu.Unlock()
	p, ok := byUser[userID]
	if !ok {
		return 0, 0
	}
	return p.total, p.labeled
}
