package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// --- HTTP ---

func TestHTTPCheckDoesNotFollowRedirects(t *testing.T) {
	var redirectTargetHits atomic.Int32

	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectTargetHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer redirectTarget.Close()

	redirectSource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
	}))
	defer redirectSource.Close()

	status, err := httpCheck(
		context.Background(),
		newHTTPClient(),
		strings.TrimPrefix(redirectSource.URL, "http://"),
		"example.com",
	)
	if err != nil {
		t.Fatalf("httpCheck: %v", err)
	}
	if status != http.StatusFound {
		t.Fatalf("status = %d, want %d", status, http.StatusFound)
	}
	if hits := redirectTargetHits.Load(); hits != 0 {
		t.Fatalf("redirect target was contacted %d times", hits)
	}
}

func TestHTTPCheckReturnsStatusCode(t *testing.T) {
	codes := []int{200, 301, 403, 404, 500, 503}
	for _, code := range codes {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		status, err := httpCheck(context.Background(), newHTTPClient(), strings.TrimPrefix(srv.URL, "http://"), "test.com")
		srv.Close()
		if err != nil {
			t.Errorf("httpCheck (code %d): %v", code, err)
			continue
		}
		if status != code {
			t.Errorf("httpCheck: got %d, want %d", status, code)
		}
	}
}

func TestHTTPCheckRetries(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	_, err := httpCheck(context.Background(), newHTTPClient(), strings.TrimPrefix(srv.URL, "http://"), "test.com")
	if err != nil {
		t.Fatalf("httpCheck: %v", err)
	}
	// Should succeed on first attempt (no retry needed)
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestHTTPCheckCanceledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := httpCheck(ctx, newHTTPClient(), strings.TrimPrefix(srv.URL, "http://"), "test.com")
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestIsHTTPAlive(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{0, false},
		{200, true},
		{301, true},
		{403, true},
		{404, true},
		{500, true},
		{959, true},
	}
	for _, tt := range tests {
		got := isHTTPAlive(tt.code)
		if got != tt.want {
			t.Errorf("isHTTPAlive(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

// --- Domain generation ---

func TestGenerateAllJobsCount(t *testing.T) {
	restore := snapshotState()
	defer restore()

	flagDomain = "test.com"
	flagBlockStart = 1
	flagBlockEnd = 1
	flagServerStart = 1
	flagServerEnd = 1
	flagGotcha = false
	baseLocations = []string{"bj"}
	numberedLocations = nil

	jobs := make(chan string, 10000)
	ctx := context.Background()
	count := generateAllJobs(ctx, jobs, allLocations(), 0)
	close(jobs)

	// Standard: 1 loc × len(standardISPs) × 1 block × 1 server
	// Extended: 1 loc × len(extendedISPs) × (bcache 25 + v 25 + live 10)
	// UPOS + Misc + External
	received := 0
	for range jobs {
		received++
	}
	if received != count {
		t.Errorf("generated %d but count = %d", received, count)
	}
	if count == 0 {
		t.Error("generated 0 domains")
	}
}

func TestGenerateAllJobsCancellation(t *testing.T) {
	restore := snapshotState()
	defer restore()

	flagDomain = "test.com"
	flagBlockStart = 1
	flagBlockEnd = 10
	flagServerStart = 1
	flagServerEnd = 50
	flagGotcha = true
	baseLocations = []string{"bj", "sh", "gz"}
	numberedLocations = nil

	ctx, cancel := context.WithCancel(context.Background())
	jobs := make(chan string, 10)

	// Cancel after receiving a few jobs
	go func() {
		received := 0
		for range jobs {
			received++
			if received >= 5 {
				cancel()
				return
			}
		}
	}()

	count := generateAllJobs(ctx, jobs, allLocations(), 0)
	close(jobs)
	// Should stop early
	if count >= estimateTotalDomains(allLocations()) {
		t.Error("generator did not stop on cancellation")
	}
}

func TestEstimateTotalDomains(t *testing.T) {
	restore := snapshotState()
	defer restore()

	flagBlockStart = 1
	flagBlockEnd = 1
	flagServerStart = 1
	flagServerEnd = 1
	flagGotcha = false
	baseLocations = []string{"bj"}
	numberedLocations = nil

	total := estimateTotalDomains(allLocations())
	if total <= 0 {
		t.Errorf("estimateTotalDomains = %d, want > 0", total)
	}
}

func TestEstimateTotalDomainsMatchesGeneration(t *testing.T) {
	restore := snapshotState()
	defer restore()

	locations := allLocations()
	estimated := estimateTotalDomains(locations)

	jobs := make(chan string, 1024)
	done := make(chan struct{})
	go func() {
		for range jobs {
		}
		close(done)
	}()

	generated := generateAllJobs(context.Background(), jobs, locations, 0)
	close(jobs)
	<-done

	if estimated != generated {
		t.Fatalf("estimateTotalDomains = %d, generateAllJobs = %d", estimated, generated)
	}
}

// --- I/O ---

func TestWriteResultsCancelsContextOnFlushError(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())

	file, err := os.Create(filepath.Join(t.TempDir(), "domains.txt"))
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	results := make(chan string, 1)
	results <- "a.example.com"
	close(results)

	count, err := writeResults(ctx, file, results, cancel)
	if err == nil {
		t.Fatal("expected writeResults to fail")
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	cause := context.Cause(ctx)
	if cause == nil {
		t.Fatal("expected context to be canceled")
	}
}

func TestWriteResultsNormalFlow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.txt")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	results := make(chan string, 3)
	results <- "b.example.com"
	results <- "a.example.com"
	results <- "a.example.com"
	close(results)

	count, err := writeResults(ctx, file, results, cancel)
	if err != nil {
		t.Fatalf("writeResults: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "b.example.com\na.example.com\na.example.com\n" {
		t.Fatalf("streamed output = %q", string(data))
	}
}

func TestSortFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unsorted.txt")
	if err := os.WriteFile(path, []byte("c\na\nb\na\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := sortFileAtomic(path); err != nil {
		t.Fatalf("sortFileAtomic: %v", err)
	}

	data, _ := os.ReadFile(path)
	want := "a\nb\nc\n"
	if string(data) != want {
		t.Errorf("sorted = %q, want %q", data, want)
	}
}

func TestSortFileAtomicEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sortFileAtomic(path); err != nil {
		t.Fatalf("sortFileAtomic empty: %v", err)
	}
}

func TestOutputFileCommitSortsAndDedups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	out, err := newOutputFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Cleanup()

	if _, err := out.file.WriteString("c.example.com\na.example.com\nb.example.com\na.example.com\n"); err != nil {
		t.Fatal(err)
	}
	if err := out.file.Close(); err != nil {
		t.Fatal(err)
	}
	out.file = nil

	if err := out.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "a.example.com\nb.example.com\nc.example.com\n" {
		t.Fatalf("committed output = %q", string(data))
	}
}

func TestResumeMergeSortsAndDedupsResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domains.txt")
	if err := os.WriteFile(path, []byte("c.example.com\na.example.com\na.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := newOutputFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Cleanup()

	if err := copyExistingResults(path, out.file); err != nil {
		t.Fatalf("copyExistingResults: %v", err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	results := make(chan string, 3)
	results <- "b.example.com"
	results <- "a.example.com"
	results <- "d.example.com"
	close(results)

	count, err := writeResults(ctx, out.file, results, cancel)
	if err != nil {
		t.Fatalf("writeResults: %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}

	out.file = nil
	if err := out.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read merged output: %v", err)
	}
	if string(data) != "a.example.com\nb.example.com\nc.example.com\nd.example.com\n" {
		t.Fatalf("merged output = %q", string(data))
	}
}

func TestOutputFileCleanupOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	out, err := newOutputFile(path)
	if err != nil {
		t.Fatal(err)
	}

	tmpPath := out.tempPath
	out.Cleanup()

	// Temp file should be removed
	if _, err := os.Stat(tmpPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("temp file not cleaned up")
	}
}

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	os.WriteFile(src, []byte("new"), 0o644)
	os.WriteFile(dst, []byte("old"), 0o644)

	if err := replaceFile(src, dst); err != nil {
		t.Fatalf("replaceFile: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "new" {
		t.Errorf("dst = %q, want %q", data, "new")
	}
}

// --- Run integration ---

func TestRunReturnsCanceledWithoutReplacingOutput(t *testing.T) {
	restore := snapshotState()
	defer restore()

	outputPath := filepath.Join(t.TempDir(), "domains.txt")
	const existing = "old.example.com\n"
	if err := os.WriteFile(outputPath, []byte(existing), 0644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}

	flagOutput = outputPath
	flagQuiet = true
	flagGotcha = false
	flagConcurrency = 1
	flagDNSStrategy = 3 // System: skip DNS pool probe
	flagBlockStart = 1
	flagBlockEnd = 1
	flagServerStart = 1
	flagServerEnd = 1

	baseLocations = []string{"bj"}
	numberedLocations = nil
	dnsGlobal = nil
	dnsCN = nil
	newSignalContextFunc = func() (context.Context, context.CancelCauseFunc, func()) {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(context.Canceled)
		return ctx, cancel, func() {}
	}

	err := Run()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}

	data, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatalf("read output file: %v", readErr)
	}
	if string(data) != existing {
		t.Fatalf("output file = %q, want %q", string(data), existing)
	}
	if _, statErr := os.Stat(outputPath + ".lock"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected lock file to be removed, stat err = %v", statErr)
	}
}

func TestRunRemovesLockWhenLoggerSetupFails(t *testing.T) {
	restore := snapshotState()
	defer restore()

	outputPath := filepath.Join(t.TempDir(), "domains.txt")
	flagOutput = outputPath
	setupLoggerFunc = func() (*log.Logger, func(), error) {
		return nil, nil, errors.New("boom")
	}

	err := Run()
	if err == nil || err.Error() != "boom" {
		t.Fatalf("Run error = %v, want boom", err)
	}
	if _, statErr := os.Stat(outputPath + ".lock"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected lock file to be removed, stat err = %v", statErr)
	}
}

// --- Flag parsing ---

func TestParseFlagsDefaults(t *testing.T) {
	restore := snapshotState()
	defer restore()

	if err := parseFlags([]string{}); err != nil {
		t.Fatalf("parseFlags default: %v", err)
	}
	if flagDomain != "bilivideo.com" {
		t.Errorf("default domain = %q", flagDomain)
	}
}

func TestParseFlagsCustom(t *testing.T) {
	restore := snapshotState()
	defer restore()

	err := parseFlags([]string{"-d", "example.com", "-c", "100", "-dns", "2"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if flagDomain != "example.com" {
		t.Errorf("domain = %q", flagDomain)
	}
	if flagConcurrency != 100 {
		t.Errorf("concurrency = %d", flagConcurrency)
	}
	if flagDNSStrategy != 2 {
		t.Errorf("dns = %d", flagDNSStrategy)
	}
}

func TestParseFlagsInvalidDNS(t *testing.T) {
	restore := snapshotState()
	defer restore()

	err := parseFlags([]string{"-dns", "9"})
	if err == nil {
		t.Fatal("expected error for invalid dns strategy")
	}
}

func TestParseFlagsInvalidRange(t *testing.T) {
	restore := snapshotState()
	defer restore()

	err := parseFlags([]string{"-bs", "5", "-be", "2"}) // start > end
	if err == nil {
		t.Fatal("expected error for invalid range")
	}
}

func TestParseFlagsExtraArgs(t *testing.T) {
	restore := snapshotState()
	defer restore()

	err := parseFlags([]string{"unexpected"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
}

func TestValidateRangeNegative(t *testing.T) {
	err := validateRange("-bs", -1, "-be", 5)
	if err == nil {
		t.Fatal("expected error for negative start")
	}
}

func TestValidateRangeTooLarge(t *testing.T) {
	err := validateRange("-bs", 0, "-be", maxTwoDigit+1)
	if err == nil {
		t.Fatal("expected error for end > maxTwoDigit")
	}
}

// --- Strategy names ---

func TestDNSStrategyName(t *testing.T) {
	tests := []struct {
		strategy int
		want     string
	}{
		{0, "Auto"},
		{1, "Global"},
		{2, "CN"},
		{3, "System"},
		{99, "Unknown(99)"},
	}
	for _, tt := range tests {
		got := dnsStrategyName(tt.strategy)
		if got != tt.want {
			t.Errorf("dnsStrategyName(%d) = %q, want %q", tt.strategy, got, tt.want)
		}
	}
}

// --- Send result ---

func TestSendResult(t *testing.T) {
	bar := NewProgressBar(10, true)
	defer bar.Finish()

	results := make(chan string, 1)
	ctx := context.Background()

	ok := sendResult(ctx, results, "test.com", bar)
	if !ok {
		t.Fatal("sendResult returned false")
	}
	got := <-results
	if got != "test.com" {
		t.Errorf("got %q, want test.com", got)
	}
}

func TestSendResultCanceled(t *testing.T) {
	bar := NewProgressBar(10, true)
	defer bar.Finish()

	results := make(chan string) // unbuffered, will block
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok := sendResult(ctx, results, "test.com", bar)
	if ok {
		t.Fatal("sendResult should return false on canceled context")
	}
}

// --- Helpers ---

func TestNewHTTPClient(t *testing.T) {
	client := newHTTPClient()
	if client == nil {
		t.Fatal("newHTTPClient returned nil")
	}
	if client.Timeout == 0 {
		t.Error("client timeout not set")
	}
}

func TestAutoConcurrency(t *testing.T) {
	pool := NewDNSResolverPool(nil, nil)
	defer pool.Close()

	tests := []struct {
		strategy int
		min      int
		max      int
	}{
		{0, 300, 1500}, // Auto
		{1, 300, 300},  // Global
		{2, 300, 300},  // CN
		{3, 128, 512},  // System
		{9, 100, 100},  // Unknown
	}
	for _, tt := range tests {
		got := autoConcurrency(tt.strategy, pool)
		if got < tt.min || got > tt.max {
			t.Errorf("autoConcurrency(%d) = %d, want [%d, %d]", tt.strategy, got, tt.min, tt.max)
		}
	}
}

func TestSetupLoggerDebug(t *testing.T) {
	restore := snapshotState()
	defer restore()
	flagDebug = true

	// Change to temp dir so scanner_errors.log is created there
	origDir, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	logger, closeLog, err := setupLogger()
	if err != nil {
		t.Fatalf("setupLogger: %v", err)
	}
	defer closeLog()

	if logger == nil {
		t.Fatal("logger is nil")
	}
	logger.Println("test message")
	closeLog()

	data, err := os.ReadFile(filepath.Join(dir, "scanner_errors.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "test message") {
		t.Error("log file missing message")
	}
}

func TestSetupLoggerNoDebug(t *testing.T) {
	restore := snapshotState()
	defer restore()
	flagDebug = false

	logger, closeLog, err := setupLogger()
	if err != nil {
		t.Fatalf("setupLogger: %v", err)
	}
	defer closeLog()

	if logger == nil {
		t.Fatal("logger is nil")
	}
}

func TestOutputFileCommitWhileOpen(t *testing.T) {
	dir := t.TempDir()
	out, err := newOutputFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Cleanup()

	// Commit while file is still open should error
	err = out.Commit()
	if err == nil {
		t.Fatal("expected error committing open file")
	}
}

func TestNewSignalContext(t *testing.T) {
	ctx, _, cleanup := newSignalContext()
	defer cleanup()

	if ctx.Err() != nil {
		t.Fatal("context should not be canceled initially")
	}
}

// snapshotState saves and restores all global mutable state for test isolation.
func snapshotState() func() {
	savedFlagConcurrency := flagConcurrency
	savedFlagDomain := flagDomain
	savedFlagDNSStrategy := flagDNSStrategy
	savedFlagDebug := flagDebug
	savedFlagQuiet := flagQuiet
	savedFlagOutput := flagOutput
	savedFlagGotcha := flagGotcha
	savedFlagResume := flagResume
	savedFlagBlockStart := flagBlockStart
	savedFlagBlockEnd := flagBlockEnd
	savedFlagServerStart := flagServerStart
	savedFlagServerEnd := flagServerEnd

	savedBaseLocations := append([]string(nil), baseLocations...)
	savedNumberedLocations := append([]string(nil), numberedLocations...)
	savedDNSOverseas := append([]DNSServer(nil), dnsGlobal...)
	savedDNSDomestic := append([]DNSServer(nil), dnsCN...)

	savedSignalContextFunc := newSignalContextFunc
	savedSetupLoggerFunc := setupLoggerFunc

	return func() {
		flagConcurrency = savedFlagConcurrency
		flagDomain = savedFlagDomain
		flagDNSStrategy = savedFlagDNSStrategy
		flagDebug = savedFlagDebug
		flagQuiet = savedFlagQuiet
		flagOutput = savedFlagOutput
		flagGotcha = savedFlagGotcha
		flagResume = savedFlagResume
		flagBlockStart = savedFlagBlockStart
		flagBlockEnd = savedFlagBlockEnd
		flagServerStart = savedFlagServerStart
		flagServerEnd = savedFlagServerEnd

		baseLocations = savedBaseLocations
		numberedLocations = savedNumberedLocations
		dnsGlobal = savedDNSOverseas
		dnsCN = savedDNSDomestic

		newSignalContextFunc = savedSignalContextFunc
		setupLoggerFunc = savedSetupLoggerFunc
	}
}
