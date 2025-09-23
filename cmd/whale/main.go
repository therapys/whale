package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/client"
	dkr "github.com/therapys/whale/internal/docker"
	"github.com/therapys/whale/internal/ui"
)

func main() {
	// Subcommand-like dispatch: whale [net] [flags]
	netMode := false
	if len(os.Args) > 1 && os.Args[1] == "net" {
		netMode = true
		// Remove subcommand before parsing flags
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}

	// Flags
	includeAll := flag.Bool("all", false, "Include stopped containers in the list")
	sortKey := flag.String("sort", "cpu", "Sort by: cpu, mem, name")
	format := flag.String("format", "table", "Output format: table or json")
	noTrunc := flag.Bool("no-trunc", false, "Do not truncate container IDs")
	watch := flag.Bool("watch", false, "Continuously refresh and stream live stats")
	interval := flag.Duration("interval", 2*time.Second, "Refresh interval for --watch")
	flag.Parse()

	var ctx context.Context
	var cancel context.CancelFunc
	if *watch {
		ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	}
	defer cancel()

	// Docker client
	cli, err := dkr.NewClient(ctx)
	if err != nil {
		fatal(err)
	}
	defer cli.Close()

	if netMode {
		if *watch {
			if strings.ToLower(*format) == "json" {
				fmt.Fprintln(os.Stderr, "Error: --watch is not supported with --format=json for networks")
				os.Exit(2)
			}
			if err := watchNetworks(ctx, cli, *includeAll, *noTrunc, *interval); err != nil {
				fatal(err)
			}
			return
		}
		groups, err := dkr.CollectNetworks(ctx, cli, *includeAll)
		if err != nil {
			fatal(err)
		}
		if err := ui.RenderNetworks(groups, *noTrunc, os.Stdout); err != nil {
			fatal(err)
		}
		return
	}

	if *watch {
		if strings.ToLower(*format) == "json" {
			fmt.Fprintln(os.Stderr, "Error: --watch is not supported with --format=json")
			os.Exit(2)
		}
		if err := watchContainers(ctx, cli, *includeAll, parseSortKey(*sortKey), *noTrunc, *interval); err != nil {
			fatal(err)
		}
		return
	}

	// One-shot mode
	snaps, err := dkr.CollectSnapshots(ctx, cli, *includeAll)
	if err != nil {
		fatal(err)
	}
	ui.SortSnapshots(snaps, parseSortKey(*sortKey))
	of := parseOutputFormat(*format)
	if err := ui.Render(snaps, of, *noTrunc, os.Stdout); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	// Normalize and print errors concisely for CLI users.
	msg := err.Error()
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "unknown error"
	}
	fmt.Fprintln(os.Stderr, "Error:", msg)
	os.Exit(1)
}

func parseSortKey(s string) ui.SortKey {
	switch strings.ToLower(s) {
	case "mem":
		return ui.SortMem
	case "name":
		return ui.SortName
	case "cpu":
		fallthrough
	default:
		return ui.SortCPU
	}
}

func parseOutputFormat(s string) ui.OutputFormat {
	switch strings.ToLower(s) {
	case "json":
		return ui.FormatJSON
	case "table":
		fallthrough
	default:
		return ui.FormatTable
	}
}

// watchContainers continuously refreshes and renders the container table.
func watchContainers(parent context.Context, cli *client.Client, includeAll bool, sortKey ui.SortKey, noTrunc bool, interval time.Duration) error {
	// Use a non-timed context so the loop runs until Ctrl+C.
	ctx := context.Background()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		// Collect and render
		snaps, err := dkr.CollectSnapshots(ctx, cli, includeAll)
		if err != nil {
			return err
		}
		ui.SortSnapshots(snaps, sortKey)
		ui.ClearScreen(os.Stdout)
		_ = ui.Render(snaps, ui.FormatTable, noTrunc, os.Stdout)

		select {
		case <-ticker.C:
			continue
		case <-parent.Done():
			return nil
		}
	}
}

// watchNetworks continuously refreshes and renders the networks table.
func watchNetworks(parent context.Context, cli *client.Client, includeAll bool, noTrunc bool, interval time.Duration) error {
	ctx := context.Background()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		groups, err := dkr.CollectNetworks(ctx, cli, includeAll)
		if err != nil {
			return err
		}
		ui.ClearScreen(os.Stdout)
		if err := ui.RenderNetworks(groups, noTrunc, os.Stdout); err != nil {
			return err
		}
		select {
		case <-ticker.C:
			continue
		case <-parent.Done():
			return nil
		}
	}
}
