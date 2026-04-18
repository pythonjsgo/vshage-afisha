package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	rdb *redis.Client
}

func NewCache(url string) (*Cache, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &Cache{rdb: redis.NewClient(opts)}, nil
}

func (c *Cache) GetList(ctx context.Context, key string) (*ListResult, bool) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil || len(b) == 0 {
		return nil, false
	}
	var out ListResult
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, false
	}
	return &out, true
}

func (c *Cache) SetList(ctx context.Context, key string, v ListResult, ttl time.Duration) {
	b, _ := json.Marshal(v)
	c.rdb.Set(ctx, key, b, ttl)
}

func (c *Cache) GetEvent(ctx context.Context, id string) (*PublicEvent, bool) {
	b, err := c.rdb.Get(ctx, "afisha:events:"+id).Bytes()
	if err != nil || len(b) == 0 {
		return nil, false
	}
	var out PublicEvent
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, false
	}
	return &out, true
}

func (c *Cache) SetEvent(ctx context.Context, ev PublicEvent, ttl time.Duration) {
	b, _ := json.Marshal(ev)
	c.rdb.Set(ctx, "afisha:events:"+ev.ID, b, ttl)
}

func (c *Cache) Invalidate(ctx context.Context, keys ...string) {
	c.rdb.Del(ctx, keys...)
}
