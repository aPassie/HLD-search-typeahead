package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"searchtypeahead/internal/model"
)

// Redis is a cache node backed by a Redis server, the optional physical
// distribution mode (CACHE_BACKEND=redis). The ring routes to these the same as
// to in-process nodes; only the storage backend differs, so the routing and
// rebalance logic stay the same. Redis errors degrade to a cache miss, which is
// safe because the request then falls through to the Trie.
type Redis struct {
	id  string
	rdb *redis.Client
}

// NewRedis connects a node to the Redis server at addr (e.g. "localhost:6379").
func NewRedis(id, addr string) *Redis {
	return &Redis{
		id: id,
		rdb: redis.NewClient(&redis.Options{
			Addr:         addr,
			ReadTimeout:  500 * time.Millisecond,
			WriteTimeout: 500 * time.Millisecond,
		}),
	}
}

func (r *Redis) ID() string { return r.id }

// Get fetches and decodes the candidate set; any error (including redis.Nil) is a miss.
func (r *Redis) Get(key string) ([]model.Candidate, bool) {
	b, err := r.rdb.Get(context.Background(), key).Bytes()
	if err != nil {
		return nil, false
	}
	var v []model.Candidate
	if json.Unmarshal(b, &v) != nil {
		return nil, false
	}
	return v, true
}

// Set stores the candidate set with the TTL. Redis handles expiry, and eviction
// via maxmemory-policy allkeys-lru.
func (r *Redis) Set(key string, val []model.Candidate, ttl time.Duration) {
	if b, err := json.Marshal(val); err == nil {
		r.rdb.Set(context.Background(), key, b, ttl)
	}
}

// Peek reports presence without fetching the value (for /cache/debug). Redis
// removes expired keys itself, so an absent key is a miss.
func (r *Redis) Peek(key string) State {
	n, err := r.rdb.Exists(context.Background(), key).Result()
	if err != nil || n == 0 {
		return StateMiss
	}
	return StateHit
}

func (r *Redis) Delete(key string) {
	r.rdb.Del(context.Background(), key)
}

// Len returns the key count for this node's Redis; each node is its own DB.
func (r *Redis) Len() int {
	n, err := r.rdb.DBSize(context.Background()).Result()
	if err != nil {
		return 0
	}
	return int(n)
}
