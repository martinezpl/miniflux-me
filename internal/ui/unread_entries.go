// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"
	"net/url"

	"miniflux.app/v2/internal/ai"
	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/storage"
	"miniflux.app/v2/internal/ui/view"
)

// AICategoryFilter is passed to the unread template for the filter bar.
type AICategoryFilter struct {
	Name   string
	Count  int
	Active bool
}

func (h *handler) buildUnreadQuery(user *model.User, activeCategories []string) *storage.EntryQueryBuilder {
	b := h.store.NewEntryQueryBuilder(user.ID).
		WithStatuses(model.EntryStatusUnread).
		WithSorting(user.EntryOrder, user.EntryDirection).
		WithSorting("id", user.EntryDirection).
		WithGloballyVisible().
		WithoutContent()
	if len(activeCategories) > 0 {
		b = b.WithAICategories(activeCategories...)
	}
	return b
}

func (h *handler) showUnreadPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	offset := request.QueryIntParam(r, "offset", 0)
	activeCategories := r.URL.Query()["categories"]

	entries, countUnread, err := h.buildUnreadQuery(user, activeCategories).
		WithOffset(offset).WithLimit(user.EntriesPerPage).GetEntriesWithCount()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}
	if offset >= countUnread && countUnread > 0 {
		offset = 0
		entries, countUnread, err = h.buildUnreadQuery(user, activeCategories).
			WithLimit(user.EntriesPerPage).GetEntriesWithCount()
		if err != nil {
			response.HTMLServerError(w, r, err)
			return
		}
	}

	filterParams := url.Values{}
	for _, c := range activeCategories {
		filterParams.Add("categories", c)
	}
	paginationBase := h.routePath("/unread")
	if len(filterParams) > 0 {
		paginationBase += "?" + filterParams.Encode() + "&"
	}

	categoryCounts, _ := h.store.GetAICategoryCounts(user.ID)
	activeSet := make(map[string]bool, len(activeCategories))
	for _, c := range activeCategories {
		activeSet[c] = true
	}
	catFilters := make([]AICategoryFilter, 0, len(ai.Categories()))
	for _, cat := range ai.Categories() {
		cnt := categoryCounts[cat.Name]
		if cnt == 0 && !activeSet[cat.Name] {
			continue
		}
		catFilters = append(catFilters, AICategoryFilter{Name: cat.Name, Count: cnt, Active: activeSet[cat.Name]})
	}

	v := view.New(h.tpl, r)
	v.Set("entries", entries)
	v.Set("pagination", getPagination(paginationBase, countUnread, offset, user.EntriesPerPage))
	v.Set("menu", "unread")
	v.Set("user", user)
	navMetadata, _ := h.store.GetNavMetadata(user.ID)
	v.Set("countUnread", navMetadata.CountUnread)
	v.Set("countErrorFeeds", navMetadata.CountErrorFeeds)
	v.Set("countAILabelFailed", navMetadata.CountAILabelFailed)
	v.Set("hasSaveEntry", navMetadata.HasSaveEntry)
	v.Set("aiCategoryFilters", catFilters)

	response.HTML(w, r, v.Render("unread_entries"))
}
