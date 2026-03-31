package main

import (
	"context"
	"io"
	"log"
	"testing"
	"time"
)

func TestRateLimiterBasic(t *testing.T) {
	rl := newRateLimiter(10)
	defer rl.stop()

	// Should have initial tokens
	if !rl.tryAcquire() {
		t.Fatal("expected initial token available")
	}

	// Wait should succeed with available tokens
	ctx := context.Background()
	if err := rl.wait(ctx); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestRateLimiterWaitCanceled(t *testing.T) {
	rl := newRateLimiter(1)
	defer rl.stop()

	// Drain all tokens
	for rl.tryAcquire() {
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rl.wait(ctx)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

func TestRateLimiterZeroQPS(t *testing.T) {
	// Should not panic, defaults to 1
	rl := newRateLimiter(0)
	defer rl.stop()
	if !rl.tryAcquire() {
		t.Fatal("expected token from qps=0 (defaults to 1)")
	}
}

func TestRateLimiterDoubleStop(t *testing.T) {
	rl := newRateLimiter(1)
	rl.stop()
	rl.stop() // should not panic
}

func TestNewDNSNodeLabel(t *testing.T) {
	tests := []struct {
		addr  string
		label string
	}{
		{"8.8.8.8:53", "8.8.8.8"},
		{"1.1.1.1:53", "1.1.1.1"},
		{"a.b", "a.b"},      // no :53 suffix
		{"x:53", "x"},       // len=4 > 3 and ends with :53 → strip
		{"abcd:53", "abcd"}, // strip :53
	}
	for _, tt := range tests {
		node := newDNSNode(tt.addr, 1)
		defer node.limiter.stop()
		if node.label != tt.label {
			t.Errorf("newDNSNode(%q).label = %q, want %q", tt.addr, node.label, tt.label)
		}
	}
}

func TestBuildWeightedIndex(t *testing.T) {
	tests := []struct {
		name    string
		servers []DNSServer
		wantLen int
	}{
		{"empty", nil, 0},
		{"single", []DNSServer{{"a", 10}}, 1},
		{"equal", []DNSServer{{"a", 10}, {"b", 10}}, 2},
		{"weighted", []DNSServer{{"a", 20}, {"b", 10}}, 3}, // 2:1 ratio
	}
	for _, tt := range tests {
		idx := buildWeightedIndex(tt.servers)
		if len(idx) != tt.wantLen {
			t.Errorf("%s: len = %d, want %d (idx=%v)", tt.name, len(idx), tt.wantLen, idx)
		}
	}
}

func TestGCD(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{12, 8, 4},
		{100, 100, 100},
		{7, 3, 1},
		{0, 5, 5},
		{-6, 4, 2},
	}
	for _, tt := range tests {
		got := gcd(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("gcd(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDNSGroupPick(t *testing.T) {
	g := &dnsGroup{
		nodes:    make([]dnsNode, 3),
		weighted: []int{0, 0, 1, 2}, // 2:1:1
	}
	// Should cycle through weighted index
	got := g.pick(0)
	if got != 0 {
		t.Errorf("pick(0) = %d, want 0", got)
	}
	got = g.pick(2)
	if got != 1 {
		t.Errorf("pick(2) = %d, want 1", got)
	}
}

func TestDNSGroupPickNoWeights(t *testing.T) {
	g := &dnsGroup{
		nodes:    make([]dnsNode, 3),
		weighted: nil,
	}
	// Falls back to counter % len(nodes)
	for i := range 3 {
		got := g.pick(uint64(i))
		if got != i {
			t.Errorf("pick(%d) = %d, want %d", i, got, i)
		}
	}
}

func TestDNSGroupAcquireAny(t *testing.T) {
	servers := []DNSServer{{"a:53", 100}, {"b:53", 100}}
	g := newDNSGroup(servers)
	defer g.stop()

	node := g.acquireAny(0)
	if node == nil {
		t.Fatal("acquireAny returned nil with available tokens")
	}
}

func TestDNSGroupAcquireAnyAllBusy(t *testing.T) {
	servers := []DNSServer{{"a:53", 1}}
	g := newDNSGroup(servers)
	defer g.stop()

	// Drain all tokens
	for g.nodes[0].limiter.tryAcquire() {
	}

	node := g.acquireAny(0)
	if node != nil {
		t.Fatal("acquireAny should return nil when all busy")
	}
}

func TestDNSGroupTotalQPS(t *testing.T) {
	servers := []DNSServer{{"a:53", 10}, {"b:53", 20}}
	g := newDNSGroup(servers)
	defer g.stop()

	got := g.totalQPS()
	if got != 30 {
		t.Errorf("totalQPS = %d, want 30", got)
	}
}

func TestNewDNSResolverPool(t *testing.T) {
	p := NewDNSResolverPool(
		[]DNSServer{{"8.8.8.8:53", 10}},
		[]DNSServer{{"223.5.5.5:53", 5}},
	)
	defer p.Close()

	if p.TotalServers() != 2 {
		t.Errorf("TotalServers = %d, want 2", p.TotalServers())
	}
	if p.TotalQPS() != 15 {
		t.Errorf("TotalQPS = %d, want 15", p.TotalQPS())
	}
}

func TestNewFlatDNSPool(t *testing.T) {
	p := NewFlatDNSPool(
		[]DNSServer{{"a:53", 10}},
		[]DNSServer{{"b:53", 20}},
	)
	defer p.Close()

	if p.TotalServers() != 2 {
		t.Errorf("TotalServers = %d, want 2", p.TotalServers())
	}
	// Flat pool: all in primary, fallback empty
	if len(p.fallback.nodes) != 0 {
		t.Errorf("fallback should be empty, got %d nodes", len(p.fallback.nodes))
	}
}

func TestLookupGroupEmptyReturnsExhausted(t *testing.T) {
	p := &DNSResolverPool{
		primary:  newDNSGroup(nil),
		fallback: newDNSGroup(nil),
	}
	defer p.Close()

	logger := log.New(io.Discard, "", 0)
	_, err := p.Lookup(context.Background(), "example.com", logger)
	if err != errDNSExhausted {
		t.Errorf("Lookup on empty pool: err = %v, want errDNSExhausted", err)
	}
}

func TestNXDOMAINCache(t *testing.T) {
	p := &DNSResolverPool{
		primary:  newDNSGroup(nil),
		fallback: newDNSGroup(nil),
	}
	defer p.Close()

	// Manually populate NXDOMAIN cache
	p.nxcache.Store("cached.example.com", time.Now().Add(time.Hour))

	logger := log.New(io.Discard, "", 0)
	_, err := p.Lookup(context.Background(), "cached.example.com", logger)
	if err != errNXDOMAIN {
		t.Errorf("cached NXDOMAIN: err = %v, want errNXDOMAIN", err)
	}
}

func TestNXDOMAINExpiredCache(t *testing.T) {
	p := &DNSResolverPool{
		primary:  newDNSGroup(nil),
		fallback: newDNSGroup(nil),
	}
	defer p.Close()

	// Expired cache entry
	p.nxcache.Store("expired.example.com", time.Now().Add(-time.Second))

	logger := log.New(io.Discard, "", 0)
	_, err := p.Lookup(context.Background(), "expired.example.com", logger)
	// Should not return errNXDOMAIN (cache expired), but errDNSExhausted (no servers)
	if err != errDNSExhausted {
		t.Errorf("expired cache: err = %v, want errDNSExhausted", err)
	}
}
