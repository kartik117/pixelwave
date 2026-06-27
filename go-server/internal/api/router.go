package api

import (
	"encoding/json"
	"net/http"

	"github.com/kartik117/pixelwave/go-server/internal/canvas"
	"github.com/kartik117/pixelwave/go-server/internal/history"
	"github.com/kartik117/pixelwave/go-server/internal/palette"
	"github.com/kartik117/pixelwave/go-server/internal/ws"
)

func NewRouter(wsServer *ws.Server, c *canvas.Canvas, h *history.Store) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// HTTP fallback for clients that just want the current state without
	// opening a WebSocket -- same pixel grid shape as the WS snapshot message.
	mux.HandleFunc("/canvas", func(w http.ResponseWriter, r *http.Request) {
		grid, err := c.Snapshot(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		pixels := make([][]string, len(grid))
		for y, row := range grid {
			hexRow := make([]string, len(row))
			for x, idx := range row {
				hexRow[x] = palette.HexForIndex(idx)
			}
			pixels[y] = hexRow
		}

		eventCount, err := h.EventCount(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pixels":      pixels,
			"event_count": eventCount,
		})
	})

	mux.HandleFunc("/ws", wsServer.Handle)

	return mux
}
