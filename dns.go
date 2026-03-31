package main

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var errNXDOMAIN = errors.New("nxdomain")
var errDNSExhausted = errors.New("dns exhausted")

// rateLimiter is a token-bucket rate limiter with graceful shutdown
type rateLimiter struct {
	tokens chan struct{}
	done   chan struct{}
}

func newRateLimiter(qps int) *rateLimiter {
	if qps <= 0 {
		qps = 1
	}
	rl := &rateLimiter{
		tokens: make(chan struct{}, qps*2),
		done:   make(chan struct{}),
	}
	for i := 0; i < qps; i++ {
		rl.tokens <- struct{}{}
	}
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(qps))
		defer ticker.Stop()
		for {
			select {
			case <-rl.done:
				return
			case <-ticker.C:
				select {
				case rl.tokens <- struct{}{}:
				default:
				}
			}
		}
	}()
	return rl
}

func (rl *rateLimiter) wait(ctx context.Context) error {
	select {
	case <-rl.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// tryAcquire attempts to get a token without blocking.
func (rl *rateLimiter) tryAcquire() bool {
	select {
	case <-rl.tokens:
		return true
	default:
		return false
	}
}

func (rl *rateLimiter) stop() {
	select {
	case <-rl.done:
	default:
		close(rl.done)
	}
}

// dnsNode is a single DNS server with pre-allocated resolver and rate limiter
type dnsNode struct {
	addr     string
	label    string
	resolver *net.Resolver
	limiter  *rateLimiter
}

func newDNSNode(addr string, qps int) dnsNode {
	label := addr
	if len(addr) > 3 && addr[len(addr)-3:] == ":53" {
		label = addr[:len(addr)-3]
	}

	dialer := &net.Dialer{Timeout: dnsTimeout}

	return dnsNode{
		addr:  addr,
		label: label,
		resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(dialCtx context.Context, network, address string) (net.Conn, error) {
				return dialer.DialContext(dialCtx, "udp4", addr)
			},
		},
		limiter: newRateLimiter(qps),
	}
}

// dnsGroup manages servers with QPS-weighted distribution.
type dnsGroup struct {
	nodes    []dnsNode
	weighted []int // expanded index: server i appears proportional to QPS
}

func newDNSGroup(servers []DNSServer) *dnsGroup {
	nodes := make([]dnsNode, len(servers))
	for i, s := range servers {
		nodes[i] = newDNSNode(s.Addr, s.QPS)
	}
	return &dnsGroup{nodes: nodes, weighted: buildWeightedIndex(servers)}
}

// probeAndFilter tests each DNS server's latency and removes slow/dead ones.
// Servers with latency > threshold are removed. Weights are adjusted by
// inverse latency so faster servers get more traffic.
// probeAndFilter tests each DNS server sequentially (to avoid burst that
// overwhelms servers), removes unresponsive ones, and records latency.
func (g *dnsGroup) probeAndFilter(domain string, threshold time.Duration) int {
	type probeResult struct {
		latency time.Duration
		ok      bool
	}

	// Probe sequentially — parallel bursts kill DNS servers at scale
	results := make([]probeResult, len(g.nodes))
	for i := range g.nodes {
		node := &g.nodes[i]

		// Single query with generous timeout (no warmup burst)
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), threshold)
		_, err := node.resolver.LookupIPAddr(ctx, domain)
		cancel()
		lat := time.Since(start)

		ok := err == nil
		if err != nil {
			if dnsErr, dnsOk := err.(*net.DNSError); dnsOk && dnsErr.IsNotFound {
				ok = true
			}
		}
		results[i] = probeResult{latency: lat, ok: ok}
	}

	// Filter: keep only responsive servers with original QPS
	var alive []dnsNode
	var aliveServers []DNSServer
	removed := 0
	for i, r := range results {
		if r.ok && r.latency < threshold {
			node := g.nodes[i]
			baseQPS := cap(node.limiter.tokens) / 2
			aliveServers = append(aliveServers, DNSServer{Addr: node.addr, QPS: baseQPS})
			alive = append(alive, node)
		} else {
			g.nodes[i].limiter.stop()
			removed++
		}
	}

	g.nodes = alive
	g.weighted = buildWeightedIndex(aliveServers)
	return removed
}

func buildWeightedIndex(servers []DNSServer) []int {
	if len(servers) == 0 {
		return nil
	}
	g := servers[0].QPS
	for _, s := range servers[1:] {
		g = gcd(g, s.QPS)
	}
	if g <= 0 {
		g = 1
	}
	var idx []int
	for i, s := range servers {
		count := s.QPS / g
		if count < 1 {
			count = 1
		}
		for j := 0; j < count; j++ {
			idx = append(idx, i)
		}
	}
	return idx
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func (g *dnsGroup) stop() {
	for i := range g.nodes {
		g.nodes[i].limiter.stop()
	}
}

func (g *dnsGroup) totalQPS() int {
	total := 0
	for i := range g.nodes {
		total += cap(g.nodes[i].limiter.tokens) / 2
	}
	return total
}

// pick returns the node index using weighted distribution
func (g *dnsGroup) pick(counter uint64) int {
	if len(g.weighted) == 0 {
		return int(counter) % len(g.nodes)
	}
	return g.weighted[int(counter)%len(g.weighted)]
}

// acquireAny tries to get a token from any server without blocking.
// Starts from the weighted pick position, scans all servers.
// Returns the node, or nil if all servers are busy.
func (g *dnsGroup) acquireAny(counter uint64) *dnsNode {
	if len(g.nodes) == 0 {
		return nil
	}
	start := g.pick(counter)
	for i := 0; i < len(g.nodes); i++ {
		idx := (start + i) % len(g.nodes)
		if g.nodes[idx].limiter.tryAcquire() {
			return &g.nodes[idx]
		}
	}
	return nil
}

// DNSResolverPool manages two tiers: overseas (primary) and domestic (fallback)
type DNSResolverPool struct {
	primary  *dnsGroup
	fallback *dnsGroup
	nxcache  sync.Map // domain -> time.Time (expiry)
	counter  atomic.Uint64
}

func NewDNSResolverPool(overseas, domestic []DNSServer) *DNSResolverPool {
	return &DNSResolverPool{
		primary:  newDNSGroup(overseas),
		fallback: newDNSGroup(domestic),
	}
}

// NewFlatDNSPool merges all servers into a single group (no primary/fallback
// cascade). Eliminates timeout overhead from the two-tier lookup.
func NewFlatDNSPool(servers ...[]DNSServer) *DNSResolverPool {
	var all []DNSServer
	for _, group := range servers {
		all = append(all, group...)
	}
	return &DNSResolverPool{
		primary:  newDNSGroup(all),
		fallback: newDNSGroup(nil),
	}
}

func (p *DNSResolverPool) Close() {
	p.primary.stop()
	p.fallback.stop()
}

// Lookup resolves domain to IPv4. Tries overseas first, falls back to domestic.
func (p *DNSResolverPool) Lookup(ctx context.Context, domain string, logger *log.Logger) (string, error) {
	if exp, ok := p.nxcache.Load(domain); ok {
		if time.Now().Before(exp.(time.Time)) {
			return "", errNXDOMAIN
		}
		// Expired — don't delete here (TOCTOU race with concurrent Store).
		// Let the next Store overwrite it naturally.
	}

	ip, err := p.lookupGroup(ctx, p.primary, domain, logger)
	if err == nil {
		return ip, nil
	}
	if errors.Is(err, errNXDOMAIN) {
		return "", err
	}

	return p.lookupGroup(ctx, p.fallback, domain, logger)
}

// lookupGroup tries up to maxDNSRetries different servers per domain.
// Uses non-blocking token acquisition: grabs whichever server has capacity first,
// only blocking as a last resort. This prevents idle workers when some servers
// have available tokens.
func (p *DNSResolverPool) lookupGroup(ctx context.Context, group *dnsGroup, domain string, logger *log.Logger) (string, error) {
	n := len(group.nodes)
	if n == 0 {
		return "", errDNSExhausted
	}

	for attempt := 0; attempt < maxDNSRetries; attempt++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		counter := p.counter.Add(1)

		// Fast path: grab any server with available tokens (non-blocking)
		node := group.acquireAny(counter)
		if node == nil {
			// Slow path: all servers busy, block on weighted pick
			idx := group.pick(counter)
			node = &group.nodes[idx]
			if err := node.limiter.wait(ctx); err != nil {
				return "", err
			}
		}

		dnsCtx, cancel := context.WithTimeout(ctx, dnsTimeout)
		ips, err := node.resolver.LookupIPAddr(dnsCtx, domain)
		cancel()

		if err != nil {
			dnsErr, isDNSErr := err.(*net.DNSError)

			// NXDOMAIN: domain does not exist — cache and skip, no retry
			if isDNSErr && dnsErr.IsNotFound {
				p.nxcache.Store(domain, time.Now().Add(nxdomainCacheTTL))
				return "", errNXDOMAIN
			}

			// Timeout / refused / network error: retry on a different server immediately
			if isDNSErr && dnsErr.IsTimeout {
				logger.Printf("FAIL dns(timeout) %s (@%s)", domain, node.label)
			} else {
				logger.Printf("FAIL dns(error)   %s (@%s)  %v", domain, node.label, err)
			}
			continue
		}

		for _, ip := range ips {
			if ip.IP.To4() != nil {
				return ip.IP.String(), nil
			}
		}
	}

	return "", errDNSExhausted
}

func (p *DNSResolverPool) TotalServers() int {
	return len(p.primary.nodes) + len(p.fallback.nodes)
}

func (p *DNSResolverPool) TotalQPS() int {
	return p.primary.totalQPS() + p.fallback.totalQPS()
}
