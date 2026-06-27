package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"

	"github.com/kartik117/pixelwave/go-server/internal/api"
	"github.com/kartik117/pixelwave/go-server/internal/canvas"
	"github.com/kartik117/pixelwave/go-server/internal/config"
	"github.com/kartik117/pixelwave/go-server/internal/history"
	"github.com/kartik117/pixelwave/go-server/internal/ratelimit"
	"github.com/kartik117/pixelwave/go-server/internal/ws"
)

func withCORS(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		next.ServeHTTP(w, r)
	})
}

func main() {
	cfg := config.Load()
	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect to redis at %s: %v", cfg.RedisAddr, err)
	}

	db, err := sql.Open("pgx", cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("failed to open postgres connection: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("failed to ping postgres: %v", err)
	}

	hist := history.New(db)
	if err := hist.Migrate(ctx); err != nil {
		log.Fatalf("migrate failed: %v", err)
	}

	c := canvas.New(rdb, cfg.CanvasWidth, cfg.CanvasHeight)
	exists, err := c.Exists(ctx)
	if err != nil {
		log.Fatalf("failed to check canvas existence: %v", err)
	}
	if !exists {
		// Redis has no canvas state -- either this is the very first boot,
		// or Redis was restarted/wiped without AOF persistence. Either way,
		// Postgres's durable event log is the source of truth to rebuild
		// from, which is the whole reason RecordPaint logs every pixel
		// instead of only ever updating Redis.
		writes, err := hist.LatestPixels(ctx)
		if err != nil {
			log.Fatalf("failed to load history for canvas restore: %v", err)
		}
		if len(writes) > 0 {
			log.Printf("canvas missing from redis -- restoring %d pixels from postgres history", len(writes))
			if err := c.RestoreFromHistory(ctx, writes); err != nil {
				log.Fatalf("failed to restore canvas from history: %v", err)
			}
		}
	}

	limiter := ratelimit.New(rdb, time.Duration(cfg.RateLimitWindow)*time.Second)
	hub := ws.NewHub(rdb)
	go hub.Run(ctx)

	wsServer := &ws.Server{Hub: hub, Canvas: c, History: hist, Limiter: limiter}
	router := api.NewRouter(wsServer, c, hist)

	log.Printf("PixelWave server listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, withCORS(cfg.CORSOrigin, router)); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
