package main

import (
	"strings"
	"sync"
	"testing"
)

// G-04 修复回归:newID 必须并发唯一(原 UnixMilli 毫秒碰撞)。并发 1000 次无重复。
func TestNewIDConcurrentUnique(t *testing.T) {
	const total = 1000
	const goroutines = 8
	perG := total / goroutines

	ids := make(chan string, total)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				ids <- newID()
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[string]struct{}, total)
	count := 0
	for id := range ids {
		count++
		if !strings.HasPrefix(id, "sess_") {
			t.Fatalf("id %q missing sess_ prefix", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id generated: %s", id)
		}
		seen[id] = struct{}{}
	}
	if count != total {
		t.Fatalf("expected %d ids, got %d", total, count)
	}
}

// newID 格式回归:sess_<unixms>_<8hex>。
func TestNewIDFormat(t *testing.T) {
	id := newID()
	if !strings.HasPrefix(id, "sess_") {
		t.Fatalf("id %q missing sess_ prefix", id)
	}
	parts := strings.Split(id, "_")
	if len(parts) != 3 {
		t.Fatalf("id %q should have 3 underscore parts (sess_<ms>_<hex>), got %d parts: %v", id, len(parts), parts)
	}
	hex := parts[2]
	if len(hex) != 8 {
		t.Fatalf("random hex part should be 8 chars, got %q (%d)", hex, len(hex))
	}
}
