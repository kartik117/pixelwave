package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/kartik117/pixelwave/go-server/internal/canvas"
	"github.com/kartik117/pixelwave/go-server/internal/history"
	"github.com/kartik117/pixelwave/go-server/internal/palette"
	"github.com/kartik117/pixelwave/go-server/internal/ratelimit"
)

// EnableCompression matters here specifically because of the initial
// snapshot: a 500x500 grid of quoted hex strings ("#FFFFFF", repeated)
// serializes to roughly 2.5MB of JSON, which exceeds the default max
// message size of several real WebSocket client libraries (not browsers --
// this was found by a Python `websockets` client closing the connection
// with code 1009 "message too big" against its own 1MB default). Repeated
// hex strings compress extremely well with permessage-deflate -- a mostly
// blank canvas drops to a few KB on the wire -- and negotiation is
// transparent to any client (browsers included) that advertises support
// for it, with no change to the JSON message shape itself.
var upgrader = websocket.Upgrader{
	CheckOrigin:       func(r *http.Request) bool { return true },
	EnableCompression: true,
}

// incomingPaint is the client -> server message shape: {"type":"paint","x":100,"y":200,"color":"#FF0000"}
type incomingPaint struct {
	Type  string `json:"type"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Color string `json:"color"`
}

// outgoingPixel is the server -> all-clients broadcast shape:
// {"type":"pixel","x":100,"y":200,"color":"#FF0000","user":"anon-xyz"}
type outgoingPixel struct {
	Type  string `json:"type"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Color string `json:"color"`
	User  string `json:"user"`
}

// snapshotMsg.Pixels is 500 row-strings, each 500 hex-nibble characters
// (one per pixel's 4-bit palette index, 0-f) -- not 500 arrays of 500
// quoted 7-character "#RRGGBB" strings. The straightforward nested-array-
// of-hex-colors encoding serializes to ~2.5MB, which exceeded the default
// max message size of every WebSocket client library tested against this
// server (not browsers specifically -- found with a Python client) even
// with permessage-deflate negotiated, since max_size in most client
// implementations caps the decompressed logical size, not the wire size.
// This encoding is ~250KB before any compression. The frontend maps each
// nibble back to a real color via the same 16-entry palette it already
// needs for the color picker.
type snapshotMsg struct {
	Type   string   `json:"type"`
	Pixels []string `json:"pixels"`
}

const hexDigits = "0123456789abcdef"

func encodeRowNibbles(row []uint8) string {
	chars := make([]byte, len(row))
	for i, idx := range row {
		chars[i] = hexDigits[idx&0x0F]
	}
	return string(chars)
}

type userCountMsg struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type errorMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type Server struct {
	Hub     *Hub
	Canvas  *canvas.Canvas
	History *history.Store
	Limiter *ratelimit.Limiter
}

// Handle upgrades the request to a WebSocket and serves one connection.
//
// Per the spec, "each connection gets its own goroutine" -- concretely
// that's the goroutine net/http already runs this handler in (which becomes
// this connection's read loop, blocked in conn.ReadJSON until the client
// disconnects), plus one explicit writePump goroutine below. gorilla/
// websocket requires that at most one goroutine ever calls a connection's
// write methods at a time; routing every outgoing message through the
// client's buffered `send` channel into a single dedicated writer goroutine
// is what guarantees that, rather than every caller (the read loop on a
// paint echo, the hub on a broadcast) writing directly and racing.
func (s *Server) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		sessionID = "anon-" + uuid.NewString()[:8]
	}

	client := newClient(sessionID)
	go s.writePump(conn, client)

	// Send the snapshot *before* registering with the hub. Registering
	// first opened a real race under concurrent connections: Snapshot()
	// is a Redis round trip, and while it's in flight, an already-
	// registered client can receive a broadcast (another user's paint, or
	// even another connection's own user_count bump) that gets queued
	// ahead of its own snapshot -- found by load-testing 550 concurrent
	// connections, where only 60 of them saw "snapshot" as their first
	// message instead of all 550. Queuing the snapshot through a client
	// the hub doesn't know about yet, then registering only once that's
	// done, makes "snapshot first" structurally guaranteed rather than
	// merely likely.
	if err := s.sendSnapshot(r.Context(), client); err != nil {
		log.Printf("failed to send snapshot to %s: %v", sessionID, err)
	}
	count := s.Hub.Register(client)
	s.broadcastUserCount(r.Context(), count)

	s.readPump(r.Context(), conn, client, sessionID)

	newCount := s.Hub.Unregister(client)
	_ = conn.Close()
	s.broadcastUserCount(r.Context(), newCount)
}

func (s *Server) sendSnapshot(ctx context.Context, client *Client) error {
	grid, err := s.Canvas.Snapshot(ctx)
	if err != nil {
		return err
	}
	pixels := make([]string, len(grid))
	for y, row := range grid {
		pixels[y] = encodeRowNibbles(row)
	}
	data, err := json.Marshal(snapshotMsg{Type: "snapshot", Pixels: pixels})
	if err != nil {
		return err
	}
	client.send <- data
	return nil
}

func (s *Server) broadcastUserCount(ctx context.Context, count int) {
	_ = s.Hub.Publish(ctx, userCountMsg{Type: "user_count", Count: count})
}

// readPump is this connection's read loop -- the "one goroutine per
// connection" the net/http server already gave us. Blocks on ReadJSON
// until the client disconnects or sends something unreadable.
func (s *Server) readPump(ctx context.Context, conn *websocket.Conn, client *Client, sessionID string) {
	for {
		var msg incomingPaint
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		if msg.Type != "paint" {
			continue
		}
		s.handlePaint(ctx, client, sessionID, msg)
	}
}

func (s *Server) handlePaint(ctx context.Context, client *Client, sessionID string, msg incomingPaint) {
	if !s.Canvas.InBounds(msg.X, msg.Y) {
		s.sendError(client, "coordinates out of bounds")
		return
	}
	colorIndex, err := palette.IndexForHex(msg.Color)
	if err != nil {
		s.sendError(client, err.Error())
		return
	}

	allowed, err := s.Limiter.Allow(ctx, sessionID)
	if err != nil {
		log.Printf("rate limiter error for %s: %v", sessionID, err)
		return
	}
	if !allowed {
		s.sendError(client, "rate limited: 1 pixel per second per user")
		return
	}

	if err := s.Canvas.SetPixel(ctx, msg.X, msg.Y, colorIndex); err != nil {
		log.Printf("SetPixel failed: %v", err)
		return
	}
	if err := s.History.RecordPaint(ctx, sessionID, msg.X, msg.Y, colorIndex); err != nil {
		// Canvas state is already updated and broadcast -- losing the
		// durable history log entry for one pixel is a real gap (it'll be
		// missing from replay and from a future RestoreFromHistory if
		// Redis is ever wiped), but it shouldn't roll back the paint or
		// drop the broadcast over a logging failure.
		log.Printf("RecordPaint failed (canvas was still updated): %v", err)
	}

	out := outgoingPixel{Type: "pixel", X: msg.X, Y: msg.Y, Color: palette.HexForIndex(colorIndex), User: sessionID}
	if err := s.Hub.Publish(ctx, out); err != nil {
		log.Printf("publish failed: %v", err)
	}
}

func (s *Server) sendError(client *Client, message string) {
	data, err := json.Marshal(errorMsg{Type: "error", Message: message})
	if err != nil {
		return
	}
	select {
	case client.send <- data:
	default:
	}
}

// writePump is the single goroutine allowed to write to this connection --
// see the comment on Handle for why that matters with gorilla/websocket.
func (s *Server) writePump(conn *websocket.Conn, client *Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case data, ok := <-client.send:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
