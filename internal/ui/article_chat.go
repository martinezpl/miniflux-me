// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"encoding/json"
	"net/http"

	"miniflux.app/v2/internal/articlechat"
	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
)

func (h *handler) articleChat(w http.ResponseWriter, r *http.Request) {
	entryID := request.RouteInt64Param(r, "entryID")

	var req struct {
		Messages []articlechat.Message `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	agent, err := articlechat.NewAgent(h.store, request.UserID(r), entryID)
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	reply, err := agent.Chat(req.Messages)
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSON(w, r, map[string]string{"reply": reply})
}
