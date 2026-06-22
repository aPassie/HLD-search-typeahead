// Package config holds runtime settings. Defaults come from the environment;
// flags override them.
package config

import (
	"flag"
	"os"
	"strconv"
	"time"
)

// Config is the full runtime configuration.
type Config struct {
	Addr       string
	DBPath     string
	DataPath   string
	TopK       int
	CandidateN int

	NumCacheNodes int
	VnodesPerNode int
	CacheTTL      time.Duration
	CacheMaxKeys  int
	CacheBackend  string // "memory" (in-process) or "redis"
	RedisAddrs    string // comma-separated, used when CacheBackend == "redis"

	BatchSize      int
	FlushInterval  time.Duration
	EventBufferCap int

	HalfLife    time.Duration
	Alpha       float64
	Weight      float64
	RankingMode string
	TrendingCap int
}

// Parse builds the config from env defaults, then applies flag overrides.
func Parse() Config {
	c := Config{
		Addr:           env("ADDR", ":8080"),
		DBPath:         env("DB_PATH", "data/typeahead.db"),
		DataPath:       env("DATA_PATH", "data/queries.csv"),
		TopK:           envInt("TOP_K", 10),
		CandidateN:     envInt("CANDIDATE_N", 50),
		NumCacheNodes:  envInt("NUM_CACHE_NODES", 3),
		VnodesPerNode:  envInt("VNODES_PER_NODE", 150),
		CacheTTL:       envDur("CACHE_TTL", 30*time.Second),
		CacheMaxKeys:   envInt("CACHE_MAX_KEYS", 10000),
		CacheBackend:   env("CACHE_BACKEND", "memory"),
		RedisAddrs:     env("REDIS_ADDRS", "localhost:6379,localhost:6380,localhost:6381"),
		BatchSize:      envInt("BATCH_SIZE", 500),
		FlushInterval:  envDur("FLUSH_INTERVAL", 5*time.Second),
		EventBufferCap: envInt("EVENT_BUFFER_CAP", 100000),
		HalfLife:       envDur("HALF_LIFE", 30*time.Minute),
		Alpha:          envFloat("ALPHA", 2.0),
		Weight:         envFloat("WEIGHT", 1.0),
		RankingMode:    env("RANKING_MODE", "recency"),
		TrendingCap:    envInt("TRENDING_CAP", 2000),
	}
	flag.StringVar(&c.Addr, "addr", c.Addr, "HTTP listen address")
	flag.StringVar(&c.DBPath, "db", c.DBPath, "SQLite database path")
	flag.StringVar(&c.DataPath, "data", c.DataPath, "CSV dataset path (query,count)")
	flag.IntVar(&c.TopK, "topk", c.TopK, "max suggestions returned")
	flag.IntVar(&c.CandidateN, "candidates", c.CandidateN, "per-node precomputed candidates")
	flag.IntVar(&c.NumCacheNodes, "cache-nodes", c.NumCacheNodes, "number of logical cache nodes")
	flag.IntVar(&c.VnodesPerNode, "vnodes", c.VnodesPerNode, "virtual nodes per cache node")
	flag.DurationVar(&c.CacheTTL, "cache-ttl", c.CacheTTL, "cache entry TTL")
	flag.IntVar(&c.CacheMaxKeys, "cache-max", c.CacheMaxKeys, "max entries per cache node")
	flag.StringVar(&c.CacheBackend, "cache-backend", c.CacheBackend, "cache backend: memory|redis")
	flag.StringVar(&c.RedisAddrs, "redis-addrs", c.RedisAddrs, "comma-separated Redis addresses (redis mode)")
	flag.IntVar(&c.BatchSize, "batch-size", c.BatchSize, "flush after this many buffered searches")
	flag.DurationVar(&c.FlushInterval, "flush-interval", c.FlushInterval, "flush at least this often")
	flag.IntVar(&c.EventBufferCap, "buffer-cap", c.EventBufferCap, "search event buffer capacity")
	flag.DurationVar(&c.HalfLife, "half-life", c.HalfLife, "recency decay half-life")
	flag.Float64Var(&c.Alpha, "alpha", c.Alpha, "recency weight in the combined score")
	flag.Float64Var(&c.Weight, "weight", c.Weight, "recency added per search")
	flag.StringVar(&c.RankingMode, "mode", c.RankingMode, "default ranking mode: count|recency")
	flag.IntVar(&c.TrendingCap, "trending-cap", c.TrendingCap, "max queries tracked for trending")
	flag.Parse()
	return c
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envDur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
