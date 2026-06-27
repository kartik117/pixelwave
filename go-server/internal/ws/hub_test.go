package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kartik117/pixelwave/go-server/internal/testutil"
)

func TestRegisterUnregisterTrackConnectionCount(t *testing.T) {
	h := NewHub(testutil.NewTestRedis(t))

	c1 := newClient("c1")
	c2 := newClient("c2")

	if got := h.Register(c1); got != 1 {
		t.Errorf("Register(c1) returned %d, want 1", got)
	}
	if got := h.Register(c2); got != 2 {
		t.Errorf("Register(c2) returned %d, want 2", got)
	}
	if got := h.ConnectionCount(); got != 2 {
		t.Errorf("ConnectionCount = %d, want 2", got)
	}

	if got := h.Unregister(c1); got != 1 {
		t.Errorf("Unregister(c1) returned %d, want 1", got)
	}
	if got := h.ConnectionCount(); got != 1 {
		t.Errorf("ConnectionCount after unregister = %d, want 1", got)
	}
}

func TestPublishFansOutToEveryLocalClientViaRedis(t *testing.T) {
	rdb := testutil.NewTestRedis(t)
	h := NewHub(rdb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Run(ctx)
	// Run() subscribes asynchronously -- give it a moment to actually
	// register the subscription with Redis before publishing, otherwise
	// the publish can race ahead of the subscribe and the message is lost
	// (pub/sub has no backlog for subscribers that join late).
	time.Sleep(100 * time.Millisecond)

	c1 := newClient("c1")
	c2 := newClient("c2")
	h.Register(c1)
	h.Register(c2)

	type pixelMsg struct {
		Type string `json:"type"`
		X    int    `json:"x"`
	}
	if err := h.Publish(ctx, pixelMsg{Type: "pixel", X: 42}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	for _, c := range []*Client{c1, c2} {
		select {
		case data := <-c.send:
			var got pixelMsg
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal broadcast: %v", err)
			}
			if got.Type != "pixel" || got.X != 42 {
				t.Errorf("client %s got %+v, want {pixel 42}", c.ID, got)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("client %s never received the broadcast", c.ID)
		}
	}
}

func TestUnregisterClosesSendChannel(t *testing.T) {
	h := NewHub(testutil.NewTestRedis(t))
	c := newClient("c1")
	h.Register(c)
	h.Unregister(c)

	_, ok := <-c.send
	if ok {
		t.Error("send channel should be closed after Unregister")
	}
}

func TestBroadcastLocalDoesNotBlockWhenAClientBufferIsFull(t *testing.T) {
	h := NewHub(testutil.NewTestRedis(t))
	c := newClient("slow-client")
	h.Register(c)

	// send has a buffer of 32 -- fill it, then broadcast one more. This
	// must return immediately (the select/default in BroadcastLocal) and
	// not deadlock the test waiting for the never-draining channel.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 40; i++ {
			h.BroadcastLocal([]byte("msg"))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("BroadcastLocal blocked on a full client buffer instead of dropping the message")
	}
}
