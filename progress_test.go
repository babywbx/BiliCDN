package main

import (
	"testing"
	"time"
)

func TestFormatNum(t *testing.T) {
	tests := []struct {
		n    uint64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{9999, "9999"},
		{10000, "10.0K"},
		{12345, "12.3K"},
		{999999, "1000.0K"},
		{1000000, "1.0M"},
		{1500000, "1.5M"},
	}
	for _, tt := range tests {
		got := formatNum(tt.n)
		if got != tt.want {
			t.Errorf("formatNum(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m00s"},
		{90 * time.Second, "1m30s"},
		{3600 * time.Second, "1h00m"},
		{3661 * time.Second, "1h01m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestProgressBarLifecycle(t *testing.T) {
	bar := NewProgressBar(100, true) // quiet mode
	bar.RecordSuccess()
	bar.RecordFailure()
	bar.SetTotal(200)
	bar.Finish()
	bar.Finish() // double finish should not panic
}

func TestProgressBarZeroTotal(t *testing.T) {
	bar := NewProgressBar(0, true)
	bar.RecordSuccess()
	bar.Finish()
}

func TestTermBarWidth(t *testing.T) {
	w := termBarWidth()
	if w < 20 || w > 100 {
		t.Errorf("termBarWidth = %d, expected 20-100", w)
	}
	// Second call should use cache
	w2 := termBarWidth()
	if w2 != w {
		t.Errorf("cached termBarWidth = %d, want %d", w2, w)
	}
}
