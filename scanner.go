package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Shared resolver and dialer for systemDNS/fallback workers
var (
	sharedSystemResolver *net.Resolver
	sharedSystemDialer   = &net.Dialer{Timeout: dnsTimeout}

	newSignalContextFunc = newSignalContext
	setupLoggerFunc      = setupLogger
)

// Pre-computed strings for domain generation (avoids fmt.Sprintf in hot loops)
var twoDigit [maxTwoDigit + 1]string
var intStr [gotchaNumberEnd + 1]string

func init() {
	sharedSystemResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(dialCtx context.Context, network, address string) (net.Conn, error) {
			return sharedSystemDialer.DialContext(dialCtx, "udp4", address)
		},
	}
	for i := range twoDigit {
		twoDigit[i] = fmt.Sprintf("%02d", i)
	}
	for i := range intStr {
		intStr[i] = fmt.Sprintf("%d", i)
	}
}

func newSignalContext() (context.Context, context.CancelCauseFunc, func()) {
	ctx, cancel := context.WithCancelCause(context.Background())
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})

	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigCh:
			fmt.Fprintf(os.Stderr, "\n\n⏹ Received %s, shutting down...\n", sig)
			cancel(context.Canceled)
		case <-done:
		}
	}()

	return ctx, cancel, func() {
		signal.Stop(sigCh)
		close(done)
		cancel(nil)
	}
}

type outputFile struct {
	finalPath string
	tempPath  string
	file      *os.File
	committed bool
}

func newOutputFile(finalPath string) (*outputFile, error) {
	dir := filepath.Dir(finalPath)
	file, err := os.CreateTemp(dir, "."+filepath.Base(finalPath)+".*.tmp")
	if err != nil {
		return nil, err
	}
	return &outputFile{
		finalPath: finalPath,
		tempPath:  file.Name(),
		file:      file,
	}, nil
}

func (o *outputFile) Cleanup() {
	if o.file != nil {
		o.file.Close()
		o.file = nil
	}
	if o.committed || o.tempPath == "" {
		return
	}
	// If temp file has content (interrupted scan), preserve it as the output
	if info, err := os.Stat(o.tempPath); err == nil && info.Size() > 0 {
		os.Rename(o.tempPath, o.finalPath)
		return
	}
	os.Remove(o.tempPath)
}

func (o *outputFile) Commit() error {
	if o.file != nil {
		return errors.New("output file is still open")
	}
	if err := sortFileAtomic(o.tempPath); err != nil {
		return err
	}
	if err := replaceFile(o.tempPath, o.finalPath); err != nil {
		return err
	}
	o.committed = true
	return nil
}

// Run executes the pipeline: generate → verify → output
func Run() error {
	outDir := filepath.Dir(flagOutput)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", outDir, err)
	}

	// Lock output file to prevent concurrent instances from colliding
	lockPath := flagOutput + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%s is locked by another instance (remove %s if stale)", flagOutput, lockPath)
		}
		return fmt.Errorf("create lock: %w", err)
	}
	defer func() {
		lockFile.Close()
		os.Remove(lockPath)
	}()

	logger, closeLog, err := setupLoggerFunc()
	if err != nil {
		return err
	}
	defer closeLog()

	// Checkpoint for resume support
	ckptPath := flagOutput + ".ckpt"
	var skipCount int
	if flagResume {
		skipCount = loadCheckpoint(ckptPath)
		if skipCount > 0 {
			fmt.Fprintf(os.Stderr, "\n[Resume] Skipping %d already-scanned domains\n", skipCount)
		}
	}

	output, err := newOutputFile(flagOutput)
	if err != nil {
		return fmt.Errorf("create temp output for %s: %w", flagOutput, err)
	}
	defer output.Cleanup()

	if flagResume && skipCount > 0 {
		if err := copyExistingResults(flagOutput, output.file); err != nil {
			return fmt.Errorf("copy existing results from %s: %w", flagOutput, err)
		}
	}

	ctx, cancelRun, cleanup := newSignalContextFunc()
	defer cleanup()

	// Setup
	locations := allLocations()
	totalEstimate := estimateTotalDomains(locations)

	probeDomain := flagDomain
	probeThreshold := 500 * time.Millisecond

	var dnsPool *DNSResolverPool

	if flagDNSStrategy == 0 {
		// Auto mode: probe, benchmark, and pick the best strategy
		var err error
		dnsPool, err = autoTuneDNS(ctx, probeDomain, probeThreshold)
		if err != nil {
			return err
		}
	} else {
		sep := strings.Repeat("─", 50)
		fmt.Fprintf(os.Stderr, "\n[DNS Probe]\n%s\n", sep)

		switch flagDNSStrategy {
		case 1: // Global: overseas primary + domestic fallback
			dnsPool = NewDNSResolverPool(dnsGlobal, dnsCN)
			removedP := dnsPool.primary.probeAndFilter(probeDomain, probeThreshold)
			removedF := dnsPool.fallback.probeAndFilter(probeDomain, probeThreshold)
			fmt.Fprintf(os.Stderr, "  Global:  %d alive, %d removed\n", len(dnsPool.primary.nodes), removedP)
			fmt.Fprintf(os.Stderr, "  CN:  %d alive, %d removed\n", len(dnsPool.fallback.nodes), removedF)
		case 2: // CN: domestic only, flat group
			dnsPool = NewFlatDNSPool(dnsCN)
			removed := dnsPool.primary.probeAndFilter(probeDomain, probeThreshold)
			fmt.Fprintf(os.Stderr, "  CN:  %d alive, %d removed\n", len(dnsPool.primary.nodes), removed)
		case 3: // System: still create pool for probe display, but workers use system resolver
			dnsPool = NewDNSResolverPool(dnsGlobal, dnsCN)
			fmt.Fprint(os.Stderr, "  Using system resolver\n")
		}

		fmt.Fprintf(os.Stderr, "  Total QPS: ~%d\n", dnsPool.TotalQPS())

		if flagDNSStrategy != 3 && dnsPool.TotalServers() == 0 {
			return fmt.Errorf("all DNS servers failed probe (check network)")
		}
	}
	defer dnsPool.Close()

	if flagConcurrency <= 0 {
		flagConcurrency = autoConcurrency(flagDNSStrategy, dnsPool)
	}

	client := newHTTPClient()
	printConfig(dnsPool, locations, totalEstimate)

	// Diff mode: recheck old domains before full scan
	var recheckAlive map[string]bool
	if flagDiff != "" {
		var err error
		recheckAlive, err = recheckDomains(ctx, client, flagDiff)
		if err != nil {
			return fmt.Errorf("recheck: %w", err)
		}
		// Write alive domains to output file immediately
		for domain := range recheckAlive {
			if _, err := output.file.WriteString(domain + "\n"); err != nil {
				return fmt.Errorf("write recheck results: %w", err)
			}
		}
	}

	// Pipeline: jobs → [DNS workers] → resolved → [HTTP workers] → results → writer
	fmt.Fprintf(os.Stderr, "\n[Scan]\n%s\n", strings.Repeat("─", 50))
	jobs := make(chan string, jobBufferSize)
	resolvedCh := make(chan resolved, httpWorkerCount*4)
	results := make(chan string, httpWorkerCount*2)
	bar := NewProgressBar(totalEstimate, flagQuiet)
	defer bar.Finish()

	// Writer
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	resultsFile := flagOutput
	var (
		resultCount int
		writerErr   error
	)
	go func() {
		resultCount, writerErr = writeResults(ctx, output.file, results, cancelRun)
		output.file = nil
		writerWg.Done()
	}()

	// HTTP workers (stage 2)
	var httpWg sync.WaitGroup
	for range httpWorkerCount {
		httpWg.Add(1)
		go httpWorker(ctx, &httpWg, resolvedCh, results, client, logger, bar)
	}

	// DNS workers (stage 1)
	var dnsWg sync.WaitGroup
	switch flagDNSStrategy {
	case 0, 1, 2: // Auto, Global, CN
		for range flagConcurrency {
			dnsWg.Add(1)
			go customDNSOnlyWorker(ctx, &dnsWg, jobs, resolvedCh, logger, bar, dnsPool)
		}
	case 3: // System
		for range flagConcurrency {
			dnsWg.Add(1)
			go systemDNSOnlyWorker(ctx, &dnsWg, jobs, resolvedCh, logger, bar)
		}
	default:
		return fmt.Errorf("unsupported DNS strategy %d", flagDNSStrategy)
	}

	// Generator
	go func() {
		count := generateAllJobs(ctx, jobs, locations, skipCount)
		bar.SetTotal(count)
		close(jobs)
	}()

	// Periodic checkpoint saver (every 10s)
	ckptDone := make(chan struct{})
	go func() {
		defer close(ckptDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				saveCheckpoint(ckptPath, int(bar.tested.Load())+skipCount)
			case <-ctx.Done():
				saveCheckpoint(ckptPath, int(bar.tested.Load())+skipCount)
				return
			}
		}
	}()

	// Wait: DNS done → close resolved → HTTP done → close results → writer done
	dnsWg.Wait()
	close(resolvedCh)
	httpWg.Wait()
	close(results)
	bar.Finish()
	writerWg.Wait()

	// Capture scan error before stopping checkpoint (cancelRun taints ctx)
	scanErr := context.Cause(ctx)

	// Stop checkpoint saver and wait for it to finish
	cancelRun(nil)
	<-ckptDone

	if writerErr != nil {
		return fmt.Errorf("write %s: %w", resultsFile, writerErr)
	}

	if scanErr != nil {
		if errors.Is(scanErr, context.Canceled) {
			return context.Canceled
		}
		return scanErr
	}
	if err := output.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", resultsFile, err)
	}

	// Completed successfully — remove checkpoint
	os.Remove(ckptPath)

	// Summary: count lines in final output for accurate total
	finalCount := resultCount
	if data, err := os.ReadFile(flagOutput); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			finalCount = len(lines)
		}
	}
	elapsed := time.Since(bar.startTime)
	tested := bar.tested.Load()
	fmt.Fprint(os.Stderr, "\n")
	if flagDiff != "" && finalCount != resultCount {
		fmt.Fprintf(os.Stderr, "  Scanned: %d new + recheck merged\n", resultCount)
	}
	fmt.Fprintf(os.Stderr, "  Total:   %d valid domains\n", finalCount)
	fmt.Fprintf(os.Stderr, "  Saved to %s\n", resultsFile)
	printStats(elapsed, tested)
	return nil
}

func printStats(elapsed time.Duration, tested uint64) {
	sep := strings.Repeat("─", 50)
	fmt.Fprintf(os.Stderr, "\n[Stats]\n%s\n", sep)

	// Time
	rate := float64(tested) / elapsed.Seconds()
	fmt.Fprintf(os.Stderr, "  Duration:    %s\n", formatDuration(elapsed))
	fmt.Fprintf(os.Stderr, "  Avg rate:    %.0f domains/s\n", rate)

	// Memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(os.Stderr, "  Memory:      %d MB alloc, %d MB sys\n", m.Alloc/1024/1024, m.Sys/1024/1024)
	fmt.Fprintf(os.Stderr, "  GC cycles:   %d\n", m.NumGC)

	// Goroutines
	fmt.Fprintf(os.Stderr, "  Goroutines:  %d (peak)\n", runtime.NumGoroutine())

	// CPU
	fmt.Fprintf(os.Stderr, "  CPU cores:   %d\n", runtime.NumCPU())

	// Network estimate (DNS: ~60 bytes/query, HTTP HEAD: ~200 bytes/request)
	dnsBytes := tested * 60 * 2     // query + response
	httpBytes := tested * 200 / 100 // only ~1% get HTTP checked (rough estimate)
	totalBytes := dnsBytes + httpBytes
	fmt.Fprintf(os.Stderr, "  Network:     ~%d MB (estimated)\n", totalBytes/1024/1024)
	mbps := float64(totalBytes) * 8 / elapsed.Seconds() / 1024 / 1024
	fmt.Fprintf(os.Stderr, "  Bandwidth:   ~%.1f Mbps (estimated)\n", mbps)
}

func loadCheckpoint(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func saveCheckpoint(path string, count int) {
	os.WriteFile(path, []byte(strconv.Itoa(count)+"\n"), 0o644)
}

// recheckDomains reads a previous domains file and quickly HTTP-checks each one.
// Returns a set of still-alive domains.
func recheckDomains(ctx context.Context, client *http.Client, path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var domains []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			domains = append(domains, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(domains) == 0 {
		return nil, nil
	}

	sep := strings.Repeat("─", 50)
	fmt.Fprintf(os.Stderr, "\n[Recheck] %d domains from %s\n%s\n", len(domains), path, sep)

	alive := make(map[string]bool)
	var mu sync.Mutex
	var dead int

	// Concurrent HTTP recheck
	sem := make(chan struct{}, httpWorkerCount)
	var wg sync.WaitGroup
	for _, domain := range domains {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d string) {
			defer wg.Done()
			defer func() { <-sem }()

			// DNS resolve first
			dnsCtx, cancel := context.WithTimeout(ctx, dnsTimeout*2)
			ips, err := net.DefaultResolver.LookupIPAddr(dnsCtx, d)
			cancel()

			var ip string
			if err == nil {
				for _, addr := range ips {
					if addr.IP.To4() != nil {
						ip = addr.IP.String()
						break
					}
				}
			}
			if ip == "" {
				mu.Lock()
				dead++
				mu.Unlock()
				return
			}

			// HTTP check
			status, err := httpCheck(ctx, client, ip, d)
			if err != nil || !isHTTPAlive(status) {
				mu.Lock()
				dead++
				mu.Unlock()
				return
			}

			mu.Lock()
			alive[d] = true
			mu.Unlock()
		}(domain)
	}
	wg.Wait()

	fmt.Fprintf(os.Stderr, "  ✓ %d alive, ✗ %d dead\n", len(alive), dead)
	return alive, nil
}

func copyExistingResults(path string, dst *os.File) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	writer := bufio.NewWriterSize(dst, 64*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if _, err := writer.WriteString(line); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return writer.Flush()
}

// detectLocalDNS discovers system DNS servers via multiple methods.
// Returns only servers not already in dnsGlobal/dnsCN.
// Works on Linux, macOS, and Windows.
func detectLocalDNS() []DNSServer {
	known := make(map[string]bool)
	for _, s := range dnsGlobal {
		known[s.Addr] = true
	}
	for _, s := range dnsCN {
		known[s.Addr] = true
	}

	seen := make(map[string]bool)
	var rawIPs []string

	collect := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip != "" && !seen[ip] {
			seen[ip] = true
			rawIPs = append(rawIPs, ip)
		}
	}

	// Method 1: /etc/resolv.conf (Linux, macOS)
	if data, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "nameserver") {
				continue
			}
			if fields := strings.Fields(line); len(fields) >= 2 {
				collect(fields[1])
			}
		}
	}

	// Method 2: scutil --dns (macOS — gets actual DNS even when /etc/resolv.conf is stubbed)
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("scutil", "--dns").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "nameserver[") {
					// Format: "nameserver[0] : 192.168.1.1"
					if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
						collect(strings.TrimSpace(parts[1]))
					}
				}
			}
		}
	}

	// Method 3: PowerShell (Windows)
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("powershell", "-Command",
			"Get-DnsClientServerAddress -AddressFamily IPv4 | Select-Object -ExpandProperty ServerAddresses",
		).Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				collect(line)
			}
		}
	}

	// Filter: keep only valid, non-loopback, non-known IPv4 addresses
	var servers []DNSServer
	for _, ip := range rawIPs {
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.To4() == nil {
			continue
		}
		if parsed.IsLoopback() || parsed.IsLinkLocalUnicast() || parsed.IsUnspecified() {
			continue
		}
		addr := ip + ":53"
		if known[addr] {
			continue
		}
		servers = append(servers, DNSServer{Addr: addr, QPS: 2000})
		known[addr] = true
	}
	return servers
}

// --- Auto-tune ---

// autoTuneDNS probes DNS servers, benchmarks different pool configurations,
// and returns the fastest pool. Also auto-selects concurrency.
func autoTuneDNS(ctx context.Context, domain string, threshold time.Duration) (*DNSResolverPool, error) {
	sep := strings.Repeat("─", 50)
	fmt.Fprintf(os.Stderr, "\n[Auto-tune]\n%s\n", sep)

	// Step 0: Detect local DNS from /etc/resolv.conf and add to pool
	localDNS := detectLocalDNS()
	if len(localDNS) > 0 {
		names := make([]string, len(localDNS))
		for i, s := range localDNS {
			names[i] = strings.TrimSuffix(s.Addr, ":53")
		}
		fmt.Fprintf(os.Stderr, "  Local DNS: %s (QPS %d each)\n", strings.Join(names, ", "), localDNS[0].QPS)
	} else {
		fmt.Fprint(os.Stderr, "  Local DNS: none detected\n")
	}

	// Step 1: Probe all servers (including local)
	fmt.Fprint(os.Stderr, "  Probing DNS servers...\n")
	allPool := NewFlatDNSPool(dnsGlobal, dnsCN, localDNS)
	removed := allPool.primary.probeAndFilter(domain, threshold)
	aliveCount := len(allPool.primary.nodes)
	fmt.Fprintf(os.Stderr, "  %d alive, %d removed (>%s)\n", aliveCount, removed, threshold)

	if aliveCount == 0 {
		allPool.Close()
		return nil, fmt.Errorf("all DNS servers failed probe (check network)")
	}

	// Step 2: Classify alive servers into overseas/domestic for benchmark
	globalAddrs := make(map[string]bool, len(dnsGlobal))
	for _, s := range dnsGlobal {
		globalAddrs[s.Addr] = true
	}
	var aliveGlobal, aliveCN []dnsNode
	for _, node := range allPool.primary.nodes {
		if globalAddrs[node.addr] {
			aliveGlobal = append(aliveGlobal, node)
		} else {
			aliveCN = append(aliveCN, node)
		}
	}
	fmt.Fprintf(os.Stderr, "  Alive: %d global, %d cn\n", len(aliveGlobal), len(aliveCN))

	// Step 3: Validate DNS with a known-resolvable domain
	fmt.Fprint(os.Stderr, "  Validating DNS resolution...\n")
	validateDomain := "www.bilibili.com"
	silentLog := log.New(io.Discard, "", 0)
	valCtx, valCancel := context.WithTimeout(ctx, 2*time.Second)
	ip, valErr := allPool.Lookup(valCtx, validateDomain, silentLog)
	valCancel()
	if valErr != nil || ip == "" {
		fmt.Fprintf(os.Stderr, "  ⚠ Cannot resolve %s — DNS may be restricted\n", validateDomain)
	} else {
		fmt.Fprintf(os.Stderr, "  ✓ %s → %s\n", validateDomain, ip)
	}

	// Step 4: Benchmark each config. Concurrent queries scale with server count
	// (~30 per server) to avoid overwhelming small pools.
	type benchResult struct {
		name    string
		rate    float64
		pool    *DNSResolverPool
		servers int
	}
	var candidates []benchResult

	benchAndAdd := func(name string, pool *DNSResolverPool) {
		servers := pool.TotalServers()
		n := servers * 30
		if n < 50 {
			n = 50
		}
		if n > 500 {
			n = 500
		}
		domains := make([]string, n)
		for i := range domains {
			domains[i] = fmt.Sprintf("cn-test%d-zz-%s.%s", i, twoDigit[i%maxTwoDigit], domain)
		}
		rate := benchmarkDNSPool(ctx, pool, domains)
		fmt.Fprintf(os.Stderr, "  Benchmark %-16s %6.0f domains/s  (%d servers)\n", name+":", rate, servers)
		if rate <= 0 {
			pool.Close()
			return
		}
		candidates = append(candidates, benchResult{name, rate, pool, servers})
	}

	// Benchmark: flat pool (all servers, no cascade)
	benchAndAdd("all", allPool)

	if len(aliveCN) > 0 {
		cnPool := NewFlatDNSPool(dnsCN)
		cnPool.primary.probeAndFilter(domain, threshold)
		if len(cnPool.primary.nodes) > 0 {
			benchAndAdd("cn", cnPool)
		} else {
			cnPool.Close()
		}
	}

	if len(aliveGlobal) > 0 {
		globalPool := NewFlatDNSPool(dnsGlobal)
		globalPool.primary.probeAndFilter(domain, threshold)
		if len(globalPool.primary.nodes) > 0 {
			benchAndAdd("global", globalPool)
		} else {
			globalPool.Close()
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("all DNS benchmarks failed")
	}

	// Pick best: when rates are within 10%, prefer more servers (scales better at high concurrency)
	best := candidates[0]
	for _, c := range candidates[1:] {
		margin := best.rate * 0.1
		if c.rate > best.rate+margin {
			best = c // clearly faster
		} else if c.rate >= best.rate-margin && c.servers > best.servers {
			best = c // similar speed, more servers = better at scale
		}
	}

	// Close the non-winners
	for _, c := range candidates {
		if c.pool != best.pool {
			c.pool.Close()
		}
	}

	// Scale workers to server count: ~100 concurrent queries per server
	if flagConcurrency <= 0 {
		flagConcurrency = best.servers * 100
		if flagConcurrency < 300 {
			flagConcurrency = 300
		}
		if flagConcurrency > 2000 {
			flagConcurrency = 2000
		}
	}

	fmt.Fprintf(os.Stderr, "  Selected:  %s (%d servers, %d workers)\n",
		best.name, best.pool.TotalServers(), flagConcurrency)

	return best.pool, nil
}

// benchmarkDNSPool measures DNS lookup throughput by firing all test domains
// concurrently (simulating real scan load) with a timeout.
func benchmarkDNSPool(ctx context.Context, pool *DNSResolverPool, domains []string) float64 {
	benchCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	silentLogger := log.New(io.Discard, "", 0)
	start := time.Now()
	var wg sync.WaitGroup
	for _, d := range domains {
		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			pool.Lookup(benchCtx, domain, silentLogger)
		}(d)
	}
	wg.Wait()
	elapsed := time.Since(start)

	if elapsed < time.Millisecond {
		return 0
	}
	return float64(len(domains)) / elapsed.Seconds()
}

// --- Setup ---

func setupLogger() (*log.Logger, func(), error) {
	if flagDebug {
		f, err := os.OpenFile("scanner_errors.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
		if err != nil {
			return nil, nil, fmt.Errorf("open scanner_errors.log: %w", err)
		}
		return log.New(f, "", log.Ltime), func() { f.Close() }, nil
	}
	return log.New(io.Discard, "", 0), func() {}, nil
}

func autoConcurrency(strategy int, pool *DNSResolverPool) int {
	switch strategy {
	case 0:
		return 500 // Auto mode fallback; usually overridden by autoTuneDNS
	case 1, 2:
		return 300
	case 3:
		// System resolver — no custom rate limiting, bound by OS
		workers := runtime.GOMAXPROCS(0) * 32
		if workers < 128 {
			workers = 128
		}
		if workers > 512 {
			workers = 512
		}
		return workers
	default:
		return 100
	}
}

func printConfig(pool *DNSResolverPool, locations []string, total int) {
	sep := strings.Repeat("─", 50)
	fmt.Fprintf(os.Stderr, "\n[BiliCDN]\n%s\n", sep)
	fmt.Fprintf(os.Stderr, "  Domain:      %s\n", flagDomain)
	fmt.Fprintf(os.Stderr, "  Concurrency: %d workers\n", flagConcurrency)
	switch flagDNSStrategy {
	case 0: // Auto
		fmt.Fprintf(os.Stderr, "  DNS:         %s (%d servers, ~%d QPS)\n",
			dnsStrategyName(flagDNSStrategy), pool.TotalServers(), pool.TotalQPS())
	case 1: // Global
		fmt.Fprintf(os.Stderr, "  DNS:         %s (%d global + %d cn, ~%d QPS)\n",
			dnsStrategyName(flagDNSStrategy), len(pool.primary.nodes), len(pool.fallback.nodes), pool.TotalQPS())
	case 2: // CN
		fmt.Fprintf(os.Stderr, "  DNS:         %s (%d cn, ~%d QPS)\n",
			dnsStrategyName(flagDNSStrategy), pool.TotalServers(), pool.TotalQPS())
	case 3: // System
		fmt.Fprintf(os.Stderr, "  DNS:         %s (system resolver)\n", dnsStrategyName(flagDNSStrategy))
	}
	fmt.Fprintf(os.Stderr, "  Range:       block[%d-%d] server[%d-%d]\n", flagBlockStart, flagBlockEnd, flagServerStart, flagServerEnd)
	fmt.Fprintf(os.Stderr, "  Gotcha:      %v\n", flagGotcha)

	blocks := flagBlockEnd - flagBlockStart + 1
	servers := flagServerEnd - flagServerStart + 1
	std := len(locations) * len(standardISPs) * blocks * servers
	var ext int
	for _, nt := range nodeTypes {
		if nt.MaxNum > 0 {
			ext += len(locations) * len(nt.ISPs) * nt.MaxNum
		}
	}

	fmt.Fprintf(os.Stderr, "\n[Domains]  ~%d total\n%s\n", total, sep)
	fmt.Fprintf(os.Stderr, "  Locations:  %d (%d base + %d numbered)\n", len(locations), len(baseLocations), len(numberedLocations))
	fmt.Fprintf(os.Stderr, "  Standard:   %d\n", std)
	fmt.Fprintf(os.Stderr, "  Extended:   %d (bcache+v+live)\n", ext)
	if flagGotcha {
		numRange := gotchaNumberEnd - gotchaNumberStart + 1
		gotcha := len(gotchaPrefixes)*len(gotchaMiddles)*len(gotchaRegions)*numRange*len(gotchaSuffixes) +
			len(gotchaRegions)*numRange*len(gotchaSuffixes)
		fmt.Fprintf(os.Stderr, "  Gotcha:     %d\n", gotcha)
	}
	fmt.Fprintf(os.Stderr, "  UPOS:       %d (+%d commercial)\n", len(uposNodes), len(commercialCDN))
}

func dnsStrategyName(strategy int) string {
	switch strategy {
	case 0:
		return "Auto"
	case 1:
		return "Global"
	case 2:
		return "CN"
	case 3:
		return "System"
	default:
		return fmt.Sprintf("Unknown(%d)", strategy)
	}
}

func newHTTPClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:          maxIdleConns,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		IdleConnTimeout:       idleConnTimeout,
		DisableKeepAlives:     false,
		DisableCompression:    true,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
		ExpectContinueTimeout: 0,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
	}
	return &http.Client{
		Timeout:   requestTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func estimateTotalDomains(locations []string) int {
	blocks := flagBlockEnd - flagBlockStart + 1
	servers := flagServerEnd - flagServerStart + 1
	total := len(locations) * len(standardISPs) * blocks * servers

	for _, nt := range nodeTypes {
		if nt.MaxNum > 0 {
			total += len(locations) * len(nt.ISPs) * nt.MaxNum
		}
	}

	if flagGotcha {
		numRange := gotchaNumberEnd - gotchaNumberStart + 1
		total += len(gotchaPrefixes) * len(gotchaMiddles) * len(gotchaRegions) * numRange * len(gotchaSuffixes)
		total += len(gotchaRegions) * numRange * len(gotchaSuffixes)
	}

	total += len(uposNodes) + len(commercialCDN)
	return total
}

// --- Domain Generation ---

// generateAllJobs produces candidates in priority order and returns the count sent.
// If skip > 0, the first `skip` domains are skipped (for resume support).
func generateAllJobs(ctx context.Context, jobs chan<- string, locations []string, skip int) int {
	count := 0
	skipped := 0
	suffix := "." + flagDomain

	send := func(domain string) bool {
		if ctx.Err() != nil {
			return false
		}
		if skipped < skip {
			skipped++
			count++
			return true
		}
		select {
		case jobs <- domain:
			count++
			return true
		case <-ctx.Done():
			return false
		}
	}

	// 1. Standard: cn-{loc}-{isp}-{block}-{server}
	for _, loc := range locations {
		for _, isp := range standardISPs {
			prefix := "cn-" + loc + "-" + isp + "-"
			for b := flagBlockStart; b <= flagBlockEnd; b++ {
				bStr := twoDigit[b] + "-"
				for s := flagServerStart; s <= flagServerEnd; s++ {
					if !send(prefix + bStr + twoDigit[s] + suffix) {
						return count
					}
				}
			}
		}
	}

	// 2. Extended: bcache, v, live
	for _, nt := range nodeTypes {
		if nt.MaxNum == 0 {
			continue
		}
		for _, loc := range locations {
			for _, isp := range nt.ISPs {
				prefix := "cn-" + loc + "-" + isp + "-" + nt.Name + "-"
				for n := 1; n <= nt.MaxNum; n++ {
					if !send(prefix + twoDigit[n] + suffix) {
						return count
					}
				}
			}
		}
	}

	// 3. Gotcha
	if flagGotcha {
		for _, pfx := range gotchaPrefixes {
			for _, mid := range gotchaMiddles {
				for _, region := range gotchaRegions {
					var base string
					if mid == "" {
						base = pfx + "--" + region + "-gotcha"
					} else {
						base = pfx + "--" + mid + "--" + region + "-gotcha"
					}
					for n := gotchaNumberStart; n <= gotchaNumberEnd; n++ {
						ns := intStr[n]
						for _, sfx := range gotchaSuffixes {
							if !send(base + ns + sfx + suffix) {
								return count
							}
						}
					}
				}
			}
		}

		for _, region := range gotchaRegions {
			base := region + "-gotcha"
			for n := gotchaNumberStart; n <= gotchaNumberEnd; n++ {
				ns := intStr[n]
				for _, sfx := range gotchaSuffixes {
					if !send(base + ns + sfx + suffix) {
						return count
					}
				}
			}
		}
	}

	// 4. UPOS
	for _, node := range uposNodes {
		if !send(node + suffix) {
			return count
		}
	}

	// 5. Commercial CDN (full domains, verified via normal DNS+HTTP pipeline)
	for _, node := range commercialCDN {
		if !send(node) {
			return count
		}
	}

	return count
}

// --- HTTP Verification ---

// isHTTPAlive returns true if the server responded at all.
// CDN nodes typically return 403 for bare HEAD requests (no valid video path),
// which still proves the node is alive and reachable.
func isHTTPAlive(statusCode int) bool {
	return statusCode > 0
}

func httpCheck(ctx context.Context, client *http.Client, ip, host string) (int, error) {
	var lastErr error
	for range maxHTTPRetries {
		req, err := http.NewRequestWithContext(ctx, "HEAD", "http://"+ip, nil)
		if err != nil {
			return 0, err
		}
		req.Host = host

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return 0, err
			}
			// Small delay before retry — yield to let other workers proceed
			timer := time.NewTimer(50 * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return 0, ctx.Err()
			}
			continue
		}
		resp.Body.Close()
		return resp.StatusCode, nil
	}
	return 0, lastErr
}

func sendResult(ctx context.Context, results chan<- string, domain string, bar *ProgressBar) bool {
	select {
	case results <- domain:
		bar.RecordSuccess()
		return true
	case <-ctx.Done():
		return false
	}
}

// --- Workers ---

// resolved carries a DNS-resolved domain to the HTTP verification stage.
type resolved struct {
	domain string
	ip     string
}

func systemDNSOnlyWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan string, out chan<- resolved, logger *log.Logger, bar *ProgressBar) {
	defer wg.Done()
	for domain := range jobs {
		if ctx.Err() != nil {
			return
		}

		dnsCtx, cancel := context.WithTimeout(ctx, dnsTimeout)
		ips, err := sharedSystemResolver.LookupIPAddr(dnsCtx, domain)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Printf("FAIL dns    %s  %v", domain, err)
			bar.RecordFailure()
			continue
		}

		var ip string
		for _, addr := range ips {
			if addr.IP.To4() != nil {
				ip = addr.IP.String()
				break
			}
		}
		if ip == "" {
			logger.Printf("FAIL no-ip4 %s", domain)
			bar.RecordFailure()
			continue
		}

		select {
		case out <- resolved{domain, ip}:
		case <-ctx.Done():
			return
		}
	}
}

func customDNSOnlyWorker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan string, out chan<- resolved, logger *log.Logger, bar *ProgressBar, dnsPool *DNSResolverPool) {
	defer wg.Done()
	for domain := range jobs {
		if ctx.Err() != nil {
			return
		}

		ip, err := dnsPool.Lookup(ctx, domain, logger)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			bar.RecordFailure()
			continue
		}

		select {
		case out <- resolved{domain, ip}:
		case <-ctx.Done():
			return
		}
	}
}

func httpWorker(ctx context.Context, wg *sync.WaitGroup, in <-chan resolved, results chan<- string, client *http.Client, logger *log.Logger, bar *ProgressBar) {
	defer wg.Done()
	for r := range in {
		if ctx.Err() != nil {
			return
		}

		status, err := httpCheck(ctx, client, r.ip, r.domain)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Printf("FAIL http   %s (%s)  %v", r.domain, r.ip, err)
			bar.RecordFailure()
			continue
		}
		if !isHTTPAlive(status) {
			logger.Printf("FAIL %d  %s (%s)", status, r.domain, r.ip)
			bar.RecordFailure()
			continue
		}

		if !sendResult(ctx, results, r.domain, bar) {
			return
		}
	}
}

// --- I/O ---

func writeResults(ctx context.Context, file *os.File, results <-chan string, cancel context.CancelCauseFunc) (int, error) {
	w := bufio.NewWriterSize(file, 64*1024)
	count := 0
	for {
		select {
		case <-ctx.Done():
			w.Flush()
			file.Close()
			if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
				return count, cause
			}
			return count, nil
		case res, ok := <-results:
			if !ok {
				if err := w.Flush(); err != nil {
					cancel(err)
					file.Close()
					return count, err
				}
				if err := file.Close(); err != nil {
					cancel(err)
					return count, err
				}
				return count, nil
			}

			if _, err := w.WriteString(res); err != nil {
				cancel(err)
				file.Close()
				return count, err
			}
			if err := w.WriteByte('\n'); err != nil {
				cancel(err)
				file.Close()
				return count, err
			}
			count++

			if w.Buffered() >= 32*1024 {
				if err := w.Flush(); err != nil {
					cancel(err)
					file.Close()
					return count, err
				}
			}
		}
	}
}

func sortFileAtomic(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}
	raw := strings.Split(trimmed, "\n")
	sort.Strings(raw)

	// Dedup sorted lines
	lines := raw[:0]
	for i, line := range raw {
		if i == 0 || line != raw[i-1] {
			lines = append(lines, line)
		}
	}

	dir := filepath.Dir(filename)
	tmp, err := os.CreateTemp(dir, ".sort-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	w := bufio.NewWriterSize(tmp, 64*1024)
	for i, line := range lines {
		if _, err := w.WriteString(line); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return err
		}
		if i < len(lines)-1 {
			if err := w.WriteByte('\n'); err != nil {
				tmp.Close()
				os.Remove(tmpName)
				return err
			}
		}
	}
	if err := w.WriteByte('\n'); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}

	return os.Rename(tmpName, filename)
}

func replaceFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if _, statErr := os.Stat(dst); statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			return err
		}
		return statErr
	}

	if removeErr := os.Remove(dst); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
		return removeErr
	}
	return os.Rename(src, dst)
}
