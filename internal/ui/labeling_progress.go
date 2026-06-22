// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"fmt"
	"net/http"
	"time"

	"miniflux.app/v2/internal/ai"
	"miniflux.app/v2/internal/http/request"
)

// labelingProgressSSE streams AI labeling progress as Server-Sent Events.
// The client connects once; the server pushes an event only when the state changes.
func (h *handler) labelingProgressSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	userID := request.UserID(r)

	var lastTotal, lastLabeled int64
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			total, labeled := ai.GetProgress(userID)
			if total != lastTotal || labeled != lastLabeled {
				lastTotal, lastLabeled = total, labeled
				fmt.Fprintf(w, "data: {\"total\":%d,\"labeled\":%d}\n\n", total, labeled)
				flusher.Flush()
			}
		}
	}
}
