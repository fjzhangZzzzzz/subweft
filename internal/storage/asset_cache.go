package storage

import (
	"container/list"
	"context"
	"sync"
)

const assetCacheLimit = 16 << 20

type cachedAsset struct {
	id   string
	data []byte
}
type assetCall struct {
	done chan struct{}
	data []byte
	err  error
}
type assetCache struct {
	mu      sync.Mutex
	bytes   int
	lru     *list.List
	entries map[string]*list.Element
	pending map[string]*assetCall
}

func newAssetCache() *assetCache {
	return &assetCache{lru: list.New(), entries: map[string]*list.Element{}, pending: map[string]*assetCall{}}
}
func (c *assetCache) get(ctx context.Context, id string, load func() ([]byte, error)) ([]byte, error) {
	c.mu.Lock()
	if entry := c.entries[id]; entry != nil {
		c.lru.MoveToFront(entry)
		data := entry.Value.(cachedAsset).data
		c.mu.Unlock()
		return data, nil
	}
	if call := c.pending[id]; call != nil {
		c.mu.Unlock()
		select {
		case <-call.done:
			return call.data, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &assetCall{done: make(chan struct{})}
	c.pending[id] = call
	c.mu.Unlock()
	call.data, call.err = load()
	c.mu.Lock()
	if call.err == nil && len(call.data) <= assetCacheLimit {
		for c.bytes+len(call.data) > assetCacheLimit {
			last := c.lru.Back()
			value := last.Value.(cachedAsset)
			c.bytes -= len(value.data)
			delete(c.entries, value.id)
			c.lru.Remove(last)
		}
		c.entries[id] = c.lru.PushFront(cachedAsset{id, call.data})
		c.bytes += len(call.data)
	}
	delete(c.pending, id)
	close(call.done)
	c.mu.Unlock()
	return call.data, call.err
}
