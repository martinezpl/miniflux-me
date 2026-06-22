// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/ai"
	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
)

func (h *handler) relabelEntry(w http.ResponseWriter, r *http.Request) {
	userID := request.UserID(r)
	entryID := request.RouteInt64Param(r, "entryID")

	entries, err := h.store.NewEntryQueryBuilder(userID).
		WithEntryIDs(entryID).
		WithoutContent().
		GetEntries()
	if err != nil || len(entries) == 0 {
		response.HTMLServerError(w, r, err)
		return
	}

	entry := entries[0]
	ai.RegisterWork(userID, 1)
	go func() {
		cats, labelErr := ai.LabelEntry(entry)
		if labelErr != nil {
			h.store.UpdateEntryAILabels(entry.ID, []string{}, true)
		} else {
			h.store.UpdateEntryAILabels(entry.ID, cats, false)
		}
		ai.MarkLabeled(userID)
	}()

	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = h.routePath("/unread")
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}
