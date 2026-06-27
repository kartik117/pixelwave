// loadtest opens N concurrent WebSocket connections against a running
// PixelWave server, confirms every one of them gets a real snapshot, then
// has one connection paint a pixel and measures how many of the others
// actually receive the broadcast and how long it took -- the two metrics
// called out in the README (500+ concurrent connections, <100ms broadcast
// latency) measured for real rather than asserted.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	host := flag.String("host", "localhost:8080", "PixelWave server host:port")
	n := flag.Int("connections", 550, "number of concurrent WebSocket connections to open")
	flag.Parse()

	var connected, gotSnapshot, gotBroadcast int64
	conns := make([]*websocket.Conn, *n)
	var wg sync.WaitGroup

	start := time.Now()
	for i := 0; i < *n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := url.URL{Scheme: "ws", Host: *host, Path: "/ws", RawQuery: "session=loadtest-" + strconv.Itoa(i)}
			conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				log.Printf("conn %d: dial failed: %v", i, err)
				return
			}
			atomic.AddInt64(&connected, 1)
			conns[i] = conn

			var snapshot map[string]any
			if err := conn.ReadJSON(&snapshot); err == nil && snapshot["type"] == "snapshot" {
				atomic.AddInt64(&gotSnapshot, 1)
			}
		}(i)
	}
	wg.Wait()
	connectTime := time.Since(start)
	fmt.Printf("connected: %d/%d, received snapshot: %d/%d, total time: %s\n", connected, *n, gotSnapshot, *n, connectTime)

	// One connection paints; every other open connection should see the
	// broadcast. user_count noise from all the connects above is drained
	// by simply reading until we see a "pixel" message or time out.
	painter := conns[0]
	if painter == nil {
		log.Fatal("painter connection (conns[0]) failed to connect, cannot measure broadcast fan-out")
	}
	t0 := time.Now()
	if err := painter.WriteJSON(map[string]any{"type": "paint", "x": 1, "y": 1, "color": "#E50000"}); err != nil {
		log.Fatalf("painter write failed: %v", err)
	}

	var latencies []time.Duration
	var mu sync.Mutex
	wg = sync.WaitGroup{}
	for i := 1; i < *n; i++ {
		conn := conns[i]
		if conn == nil {
			continue
		}
		wg.Add(1)
		go func(conn *websocket.Conn) {
			defer wg.Done()
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			for {
				var raw json.RawMessage
				if err := conn.ReadJSON(&raw); err != nil {
					return
				}
				var msg map[string]any
				if err := json.Unmarshal(raw, &msg); err != nil {
					return
				}
				if msg["type"] == "pixel" {
					atomic.AddInt64(&gotBroadcast, 1)
					mu.Lock()
					latencies = append(latencies, time.Since(t0))
					mu.Unlock()
					return
				}
			}
		}(conn)
	}
	wg.Wait()

	fmt.Printf("broadcast received by: %d/%d other connections\n", gotBroadcast, *n-1)
	if len(latencies) > 0 {
		var total time.Duration
		max := latencies[0]
		for _, l := range latencies {
			total += l
			if l > max {
				max = l
			}
		}
		fmt.Printf("broadcast latency: avg=%s max=%s\n", total/time.Duration(len(latencies)), max)
	}

	for _, c := range conns {
		if c != nil {
			_ = c.Close()
		}
	}
}
