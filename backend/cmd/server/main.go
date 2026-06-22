// Command server builds the in-memory Trie + trending index from the SQLite
// store, wires up the distributed cache and batch writer, and serves the
// suggestion API plus the embedded UI. On SIGINT/SIGTERM it stops the listener
// and flushes the buffer before exiting; a hard crash loses the unflushed tail.
//
//	go run ./cmd/server -db data/typeahead.db
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"searchtypeahead/internal/api"
	"searchtypeahead/internal/batch"
	"searchtypeahead/internal/cache"
	"searchtypeahead/internal/config"
	"searchtypeahead/internal/engine"
	"searchtypeahead/internal/hashring"
	"searchtypeahead/internal/model"
	"searchtypeahead/internal/ranking"
	"searchtypeahead/internal/store"
	"searchtypeahead/internal/trending"
	"searchtypeahead/internal/trie"
)

func main() {
	cfg := config.Parse()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	total, err := st.Count()
	if err != nil {
		log.Fatalf("count: %v", err)
	}
	if total == 0 {
		log.Fatalf("store is empty — run `go run ./cmd/load -data %s -db %s` first", cfg.DataPath, cfg.DBPath)
	}

	// Trie holds the counts; trending index gets seeded from persisted recency.
	params := ranking.New(cfg.HalfLife, cfg.Alpha, cfg.Weight)
	trend := trending.New(params, cfg.TrendingCap)
	log.Printf("building trie + trending from %d queries ...", total)
	start := time.Now()
	tr := trie.New(cfg.CandidateN)
	if err := st.ForEach(func(c model.Candidate) {
		tr.Insert(c.Query, c.Count)
		if c.RecentValue > 0 {
			trend.Seed(c.Query, c.RecentValue, c.RecentTS)
		}
	}); err != nil {
		log.Fatalf("build trie: %v", err)
	}
	tr.Finalize()
	log.Printf("built in %s", time.Since(start))

	// N logical cache nodes behind a consistent-hash ring. In-process by default,
	// or one Redis server per node when CACHE_BACKEND=redis. Same ring and routing
	// either way; only the node backend changes.
	var nodeIDs []string
	nodes := map[string]cache.Node{}
	if cfg.CacheBackend == "redis" {
		for _, addr := range strings.Split(cfg.RedisAddrs, ",") {
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			id := "redis:" + addr
			nodeIDs = append(nodeIDs, id)
			nodes[id] = cache.NewRedis(id, addr)
		}
		if len(nodeIDs) == 0 {
			log.Fatal("CACHE_BACKEND=redis but REDIS_ADDRS is empty")
		}
		log.Printf("cache: redis, %d nodes × %d vnodes, ttl %s (%s)", len(nodeIDs), cfg.VnodesPerNode, cfg.CacheTTL, cfg.RedisAddrs)
	} else {
		for i := 0; i < cfg.NumCacheNodes; i++ {
			id := fmt.Sprintf("cache-%d", i)
			nodeIDs = append(nodeIDs, id)
			nodes[id] = cache.NewMemory(id, cfg.CacheMaxKeys)
		}
		log.Printf("cache: memory, %d nodes × %d vnodes, ttl %s", cfg.NumCacheNodes, cfg.VnodesPerNode, cfg.CacheTTL)
	}
	ring := hashring.New(cfg.VnodesPerNode, nodeIDs...)
	dcache := cache.NewDistributed(ring, nodes, cfg.CacheTTL)

	eng := engine.New(tr, dcache, st, trend, params, cfg.TopK, cfg.CandidateN, cfg.RankingMode)
	writer := batch.New(cfg.EventBufferCap, cfg.BatchSize, cfg.FlushInterval, eng.ApplyBatch)
	eng.SetWriter(writer)
	writer.Start()
	log.Printf("ranking: default=%s, half-life=%s, alpha=%.1f | batch: flush %d / every %s",
		cfg.RankingMode, cfg.HalfLife, cfg.Alpha, cfg.BatchSize, cfg.FlushInterval)

	srv := &http.Server{Addr: cfg.Addr, Handler: api.NewServer(eng).Routes()}

	// Stop the listener first so no new searches arrive, then flush the buffer.
	stopped := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		log.Println("shutting down ...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		close(stopped)
	}()

	log.Printf("listening on %s  (open http://localhost%s)", cfg.Addr, cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}

	<-stopped
	writer.Stop() // flushes whatever is still buffered
	log.Printf("flushed on exit: %+v", writer.Stats())
	st.Close()
}
