package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/kartik117/pixelwave/go-server/internal/canvas"
	"github.com/kartik117/pixelwave/go-server/internal/history"
	"github.com/kartik117/pixelwave/go-server/internal/ratelimit"
	"github.com/kartik117/pixelwave/go-server/internal/testutil"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	rdb := testutil.NewTestRedis(t)
	pg := testutil.NewTestPostgres(t)

	hist := history.New(pg)
	if err := hist.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	c := canvas.New(rdb, 500, 500)
	hub := NewHub(rdb)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)
	time.Sleep(100 * time.Millisecond) // let the subscription register, same as hub_test.go

	s := &Server{
		Hub:     hub,
		Canvas:  c,
		History: hist,
		Limiter: ratelimit.New(rdb, time.Second),
	}

	srv := httptest.NewServer(http.HandlerFunc(s.Handle))
	t.Cleanup(srv.Close)
	return s, srv
}

func dialWS(t *testing.T, srv *httptest.Server, session string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?session=" + session
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var msg map[string]any
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read message: %v", err)
	}
	return msg
}

// readMessageOfType skips past any user_count broadcasts (sent whenever any
// client connects/disconnects, including this test's own connection setup)
// to find the message type a test actually cares about, rather than
// assuming a fixed position in the stream.
func readMessageOfType(t *testing.T, conn *websocket.Conn, wantType string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg := readMessage(t, conn, timeout)
		if msg["type"] == wantType {
			return msg
		}
	}
	t.Fatalf("never received a %q message within %s", wantType, timeout)
	return nil
}

func TestNewConnectionReceivesASnapshotFirst(t *testing.T) {
	_, srv := newTestServer(t)
	conn := dialWS(t, srv, "session-1")

	msg := readMessageOfType(t, conn, "snapshot", 2*time.Second)
	pixels, ok := msg["pixels"].([]any)
	if !ok || len(pixels) != 500 {
		t.Fatalf("snapshot pixels has %d rows, want 500", len(pixels))
	}
	// Each row is a 500-character string of hex nibbles, not an array of
	// 500 quoted "#RRGGBB" strings -- see the comment on snapshotMsg for
	// why (the naive encoding hit real WebSocket client message-size limits).
	row0, ok := pixels[0].(string)
	if !ok || len(row0) != 500 {
		t.Fatalf("snapshot row 0 = %T with length %d, want a 500-character string", pixels[0], len(row0))
	}
}

func TestPaintBroadcastsToAllConnectedClients(t *testing.T) {
	_, srv := newTestServer(t)
	conn1 := dialWS(t, srv, "session-1")
	readMessageOfType(t, conn1, "snapshot", 2*time.Second)
	conn2 := dialWS(t, srv, "session-2")
	readMessageOfType(t, conn2, "snapshot", 2*time.Second)

	if err := conn1.WriteJSON(map[string]any{"type": "paint", "x": 10, "y": 20, "color": "#E50000"}); err != nil {
		t.Fatalf("write paint: %v", err)
	}

	for _, conn := range []*websocket.Conn{conn1, conn2} {
		msg := readMessageOfType(t, conn, "pixel", 2*time.Second)
		if msg["x"] != float64(10) || msg["y"] != float64(20) || msg["color"] != "#E50000" {
			t.Errorf("broadcast = %+v, want x=10 y=20 color=#E50000", msg)
		}
		if msg["user"] != "session-1" {
			t.Errorf("broadcast user = %v, want session-1", msg["user"])
		}
	}
}

func TestPaintIsPersistedToCanvasAndHistory(t *testing.T) {
	s, srv := newTestServer(t)
	conn := dialWS(t, srv, "session-1")
	readMessageOfType(t, conn, "snapshot", 2*time.Second)

	if err := conn.WriteJSON(map[string]any{"type": "paint", "x": 5, "y": 5, "color": "#0000EA"}); err != nil {
		t.Fatalf("write paint: %v", err)
	}
	readMessageOfType(t, conn, "pixel", 2*time.Second) // the broadcast back to ourselves

	idx, err := s.Canvas.GetPixel(context.Background(), 5, 5)
	if err != nil {
		t.Fatalf("GetPixel: %v", err)
	}
	if idx != 13 { // #0000EA is index 13 in the palette
		t.Errorf("canvas pixel(5,5) = %d, want 13", idx)
	}

	count, err := s.History.EventCount(context.Background())
	if err != nil {
		t.Fatalf("EventCount: %v", err)
	}
	if count != 1 {
		t.Errorf("history EventCount = %d, want 1", count)
	}
}

func TestSecondPaintWithinOneSecondIsRateLimited(t *testing.T) {
	_, srv := newTestServer(t)
	conn := dialWS(t, srv, "session-1")
	readMessageOfType(t, conn, "snapshot", 2*time.Second)

	_ = conn.WriteJSON(map[string]any{"type": "paint", "x": 1, "y": 1, "color": "#E50000"})
	readMessageOfType(t, conn, "pixel", 2*time.Second) // the first paint's own broadcast

	_ = conn.WriteJSON(map[string]any{"type": "paint", "x": 2, "y": 2, "color": "#E50000"})
	readMessageOfType(t, conn, "error", 2*time.Second)
}

func TestPaintWithAColorOutsideThePaletteIsRejected(t *testing.T) {
	_, srv := newTestServer(t)
	conn := dialWS(t, srv, "session-1")
	readMessageOfType(t, conn, "snapshot", 2*time.Second)

	_ = conn.WriteJSON(map[string]any{"type": "paint", "x": 1, "y": 1, "color": "#123456"})
	readMessageOfType(t, conn, "error", 2*time.Second)
}

func TestPaintOutOfBoundsIsRejected(t *testing.T) {
	_, srv := newTestServer(t)
	conn := dialWS(t, srv, "session-1")
	readMessageOfType(t, conn, "snapshot", 2*time.Second)

	_ = conn.WriteJSON(map[string]any{"type": "paint", "x": 999, "y": 1, "color": "#E50000"})
	readMessageOfType(t, conn, "error", 2*time.Second)
}

