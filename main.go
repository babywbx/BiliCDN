package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	// Subcommand routing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "convert":
			if err := runConvert(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "help", "--help":
			os.Args[1] = "-h"
		}
	}

	if err := parseFlags(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	if err := Run(); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags(args []string) error {
	fs := flag.NewFlagSet("bilicdn", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.IntVar(&flagConcurrency, "c", flagConcurrency, "Concurrent workers (0 = auto)")
	fs.StringVar(&flagDomain, "d", flagDomain, "Target domain")
	fs.IntVar(&flagDNSStrategy, "dns", flagDNSStrategy, "DNS strategy: 0=Auto, 1=Global, 2=CN, 3=System")
	fs.BoolVar(&flagDebug, "debug", flagDebug, "Write error log to scanner_errors.log")
	fs.BoolVar(&flagQuiet, "quiet", flagQuiet, "Log mode (no TUI, for CI/pipes)")
	fs.StringVar(&flagOutput, "o", flagOutput, "Output file path")
	fs.BoolVar(&flagGotcha, "gotcha", flagGotcha, "Enable gotcha pattern scanning")
	fs.BoolVar(&flagResume, "resume", flagResume, "Resume from last checkpoint")
	fs.StringVar(&flagDiff, "diff", flagDiff, "Recheck domains from previous file before scanning")
	fs.BoolVar(&flagRecheckOnly, "recheck-only", flagRecheckOnly, "Only recheck domains from -diff file, skip full scan")
	fs.IntVar(&flagHTTPRetries, "http-retries", flagHTTPRetries, "Override HTTP retry count (0 = default)")
	fs.DurationVar(&flagHTTPTimeout, "http-timeout", flagHTTPTimeout, "Override HTTP request timeout (0 = default)")
	fs.IntVar(&flagBlockStart, "bs", flagBlockStart, "Block range start")
	fs.IntVar(&flagBlockEnd, "be", flagBlockEnd, "Block range end")
	fs.IntVar(&flagServerStart, "ss", flagServerStart, "Server range start")
	fs.IntVar(&flagServerEnd, "se", flagServerEnd, "Server range end")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "BiliCDN - Bilibili CDN node discovery tool")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: bilicdn [flags]")
		fmt.Fprintln(os.Stderr, "  Generates candidate CDN domains, verifies via DNS+HTTP, outputs alive nodes.")
		fmt.Fprintln(os.Stderr, "")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	return validateFlags()
}

func validateFlags() error {
	flagDomain = strings.TrimSpace(flagDomain)
	if flagDomain == "" {
		return errors.New("-d must not be empty")
	}
	if flagConcurrency < 0 {
		return errors.New("-c must be >= 0")
	}
	if flagDNSStrategy < 0 || flagDNSStrategy > 3 {
		return fmt.Errorf("-dns must be 0, 1, 2, or 3 (got %d)", flagDNSStrategy)
	}
	if err := validateRange("-bs", flagBlockStart, "-be", flagBlockEnd); err != nil {
		return err
	}
	if flagRecheckOnly && flagDiff == "" {
		return errors.New("-recheck-only requires -diff")
	}
	return validateRange("-ss", flagServerStart, "-se", flagServerEnd)
}

func validateRange(startName string, start int, endName string, end int) error {
	if start < 0 || start > maxTwoDigit {
		return fmt.Errorf("%s must be 0-%d (got %d)", startName, maxTwoDigit, start)
	}
	if end < 0 || end > maxTwoDigit {
		return fmt.Errorf("%s must be 0-%d (got %d)", endName, maxTwoDigit, end)
	}
	if start > end {
		return fmt.Errorf("%s must be <= %s (got %d > %d)", startName, endName, start, end)
	}
	return nil
}
