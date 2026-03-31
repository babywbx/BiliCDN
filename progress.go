package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// ProgressBar tracks scan progress with two display modes:
// - TUI mode: single-line progress bar with \r overwrite (interactive terminal)
// - Quiet mode: periodic log lines (CI, pipes, nohup)
type ProgressBar struct {
	total      atomic.Uint64
	tested     atomic.Uint64
	successes  atomic.Uint64
	failures   atomic.Uint64
	startTime  time.Time
	quiet      bool
	displayMu  sync.Mutex
	stopSignal chan struct{}
	doneSignal chan struct{} // closed when run() exits
}

// NewProgressBar creates and starts a progress bar.
// If quiet is true or stderr is not a terminal, uses log mode.
func NewProgressBar(estimatedTotal int, quiet bool) *ProgressBar {
	if !quiet {
		// Auto-detect: if stderr is not a tty, force quiet mode
		quiet = !term.IsTerminal(int(os.Stderr.Fd()))
	}

	interval := progressUpdateInterval
	if quiet {
		interval = 10 * time.Second
	}

	bar := &ProgressBar{
		startTime:  time.Now(),
		quiet:      quiet,
		stopSignal: make(chan struct{}),
		doneSignal: make(chan struct{}),
	}
	bar.total.Store(uint64(estimatedTotal))
	go bar.run(interval)
	return bar
}

// SetTotal updates the total after exact count is known
func (p *ProgressBar) SetTotal(total int) {
	p.total.Store(uint64(total))
}

func (p *ProgressBar) run(interval time.Duration) {
	defer close(p.doneSignal)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopSignal:
			p.displayFinal()
			return
		case <-ticker.C:
			p.display()
		}
	}
}

// RecordSuccess increments success counter
func (p *ProgressBar) RecordSuccess() { p.successes.Add(1); p.tested.Add(1) }

// RecordFailure increments failure counter
func (p *ProgressBar) RecordFailure() { p.failures.Add(1); p.tested.Add(1) }

// Finish stops the display goroutine and waits for final output to complete
func (p *ProgressBar) Finish() {
	select {
	case <-p.stopSignal:
	default:
		close(p.stopSignal)
	}
	<-p.doneSignal // wait for displayFinal to finish writing
}

func (p *ProgressBar) display() {
	p.displayMu.Lock()
	defer p.displayMu.Unlock()

	if p.quiet {
		p.displayQuiet()
	} else {
		p.displayTUI()
	}
}

func (p *ProgressBar) displayFinal() {
	p.displayMu.Lock()
	defer p.displayMu.Unlock()

	if p.quiet {
		p.displayQuiet()
	} else {
		p.displayTUI()
		fmt.Fprint(os.Stderr, "\n")
	}
}

func (p *ProgressBar) displayTUI() {
	total := p.total.Load()
	tested := p.tested.Load()
	ok := p.successes.Load()
	fail := p.failures.Load()
	if total == 0 {
		return
	}

	percent := float64(tested) * 100 / float64(total)
	if percent > 100 {
		percent = 100
	}
	elapsed := time.Since(p.startTime)
	rate := float64(tested) / elapsed.Seconds()
	if elapsed.Seconds() == 0 {
		rate = 0
	}
	var eta time.Duration
	if rate > 0 && tested < total {
		eta = time.Duration(float64(total-tested)/rate) * time.Second
	}

	barWidth := termBarWidth()
	completedWidth := int(float64(barWidth) * percent / 100)
	bar := strings.Repeat("█", completedWidth) + strings.Repeat("░", barWidth-completedWidth)

	// Format: ████░░░░ 45.2% (1.8M/4.0M) | Ok: 312 | Fail: 1.8M | 4.2k/s [3m12s<4m01s]
	fmt.Fprintf(os.Stderr, "\r%s %.1f%% (%s/%s) | Ok: %s | Fail: %s | %s/s [%s<%s]",
		bar, percent,
		formatNum(tested), formatNum(total),
		formatNum(ok), formatNum(fail),
		formatNum(uint64(rate)),
		formatDuration(elapsed), formatDuration(eta))
}

func (p *ProgressBar) displayQuiet() {
	total := p.total.Load()
	tested := p.tested.Load()
	ok := p.successes.Load()
	fail := p.failures.Load()
	if total == 0 {
		return
	}

	percent := float64(tested) * 100 / float64(total)
	if percent > 100 {
		percent = 100
	}
	elapsed := time.Since(p.startTime)
	rate := float64(tested) / elapsed.Seconds()
	if elapsed.Seconds() == 0 {
		rate = 0
	}

	fmt.Fprintf(os.Stderr, "[%s] %.1f%% tested=%s ok=%s fail=%s rate=%s/s\n",
		formatDuration(elapsed),
		percent,
		formatNum(tested), formatNum(ok), formatNum(fail),
		formatNum(uint64(rate)))
}

// termBarWidth returns cached terminal bar width (refreshed every 5s)
var (
	cachedBarWidth     int
	cachedBarWidthTime time.Time
)

func termBarWidth() int {
	now := time.Now()
	if now.Sub(cachedBarWidthTime) < 5*time.Second && cachedBarWidth > 0 {
		return cachedBarWidth
	}
	cachedBarWidthTime = now
	cachedBarWidth = 30
	if w, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil {
		switch {
		case w > 120:
			cachedBarWidth = 50
		case w > 80:
			cachedBarWidth = 40
		}
	}
	return cachedBarWidth
}

// formatNum formats large numbers with K/M suffixes
func formatNum(n uint64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// formatDuration formats duration as compact string
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%02dm", h, m)
}
