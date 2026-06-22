// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/ai"
	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/ui/view"
)

func (h *handler) showAILabelFailedPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	entries, err := h.store.NewEntryQueryBuilder(user.ID).
		WithAILabelFailed().
		WithoutContent().
		GetEntries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	v := view.New(h.tpl, r)
	v.Set("entries", entries)
	v.Set("menu", "")
	v.Set("user", user)
	navMetadata, _ := h.store.GetNavMetadata(user.ID)
	v.Set("countUnread", navMetadata.CountUnread)
	v.Set("countErrorFeeds", navMetadata.CountErrorFeeds)
	v.Set("countAILabelFailed", navMetadata.CountAILabelFailed)
	v.Set("hasSaveEntry", navMetadata.HasSaveEntry)
	response.HTML(w, r, v.Render("ai_label_failed"))
}

func (h *handler) retryAILabelFailed(w http.ResponseWriter, r *http.Request) {
	userID := request.UserID(r)

	entries, err := h.store.NewEntryQueryBuilder(userID).
		WithAILabelFailed().
		WithoutContent().
		GetEntries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	if len(entries) > 0 {
		go func() {
			ai.RegisterWork(userID, len(entries))
			for _, entry := range entries {
				cats, labelErr := ai.LabelEntry(entry)
				if labelErr != nil {
					h.store.UpdateEntryAILabels(entry.ID, []string{}, true)
				} else {
					h.store.UpdateEntryAILabels(entry.ID, cats, false)
				}
				ai.MarkLabeled(userID)
			}
		}()
	}

	http.Redirect(w, r, h.routePath("/entries/labeling-failed"), http.StatusSeeOther)
}
