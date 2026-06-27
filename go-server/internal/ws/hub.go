// Package ws implements the real-time fan-out: one goroutine per
// connection reads paint events from that client, and a single hub
// goroutine subscribed to Redis pub/sub rebroadcasts every paint (from any
// client, on any process) to every connected client's own send channel.
//
// That split is what makes broadcasting safe to scale past one go-server
// process: each process only holds the WebSocket connections made directly
// to it, but every process publishes to and subscribes from the same Redis
// channel, so a paint that arrives on process A still reaches a client
// connected to process B.
package ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
)

const eventsChannel = "pixelwave:events"

// Client wraps one WebSocket connection. send is buffered so a slow client
// can't block the hub's broadcast loop -- a full buffer means dropping that
// one client's message rather than stalling everyone else's.
type Client struct {
	ID   string
	send chan []byte
}

func newClient(id string) *Client {
	return &Client{ID: id, send: make(chan []byte, 32)}
}

type Hub struct {
	rdb *redis.Client

	mu      sync.RWMutex
	clients map[*Client]bool
}

func NewHub(rdb *redis.Client) *Hub {
	return &Hub{rdb: rdb, clients: make(map[*Client]bool)}
}

// Register adds a client and returns the new total connection count.
func (h *Hub) Register(c *Client) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = true
	return len(h.clients)
}

// Unregister removes a client and returns the new total connection count.
func (h *Hub) Unregister(c *Client) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	close(c.send)
	return len(h.clients)
}

func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Publish marshals msg and publishes it on the shared Redis channel. It
// does not write directly to any client -- every process's Run loop
// (including this one's) picks it up via the subscription and broadcasts
// it locally, so publishing and local delivery always go through the same
// path, with no special case for "messages this process generated."
func (h *Hub) Publish(ctx context.Context, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return h.rdb.Publish(ctx, eventsChannel, data).Err()
}

// BroadcastLocal sends raw bytes to every client connected to this process.
func (h *Hub) BroadcastLocal(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			log.Printf("dropping broadcast to client %s: send buffer full", c.ID)
		}
	}
}

// Run subscribes to the shared Redis channel and rebroadcasts every message
// to this process's locally-connected clients. Meant to run for the
// lifetime of the server in its own goroutine.
func (h *Hub) Run(ctx context.Context) {
	pubsub := h.rdb.Subscribe(ctx, eventsChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			h.BroadcastLocal([]byte(msg.Payload))
		}
	}
}
