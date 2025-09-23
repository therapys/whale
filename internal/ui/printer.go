package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	prettytable "github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"golang.org/x/term"

	dkr "github.com/therapys/whale/internal/docker"
)

// OutputFormat represents supported output formats.
type OutputFormat string

const (
	FormatTable OutputFormat = "table"
	FormatJSON  OutputFormat = "json"
)

// SortKey controls ordering of snapshots.
type SortKey string

const (
	SortCPU  SortKey = "cpu"
	SortMem  SortKey = "mem"
	SortName SortKey = "name"
)

// NetGroup represents a network name and its member containers.
type NetGroup struct {
	Network    string
	Containers []dkr.ContainerNetInfo
}

// SortSnapshots sorts in-place according to the provided key.
// CPU and memory are sorted descending; name ascending case-insensitively.
func SortSnapshots(snaps []dkr.ContainerSnapshot, key SortKey) {
	switch key {
	case SortMem:
		sort.Slice(snaps, func(i, j int) bool { return snaps[i].MemPercent > snaps[j].MemPercent })
	case SortName:
		sort.Slice(snaps, func(i, j int) bool {
			return strings.ToLower(snaps[i].Name) < strings.ToLower(snaps[j].Name)
		})
	case SortCPU:
		fallthrough
	default:
		sort.Slice(snaps, func(i, j int) bool { return snaps[i].CPUPercent > snaps[j].CPUPercent })
	}
}

// TruncateID returns a 12-char Docker-like ID when noTrunc is false.
func TruncateID(id string, noTrunc bool) string {
	if noTrunc || len(id) <= 12 {
		return id
	}
	return id[:12]
}

// TruncateName trims long names unless noTrunc is set. Keeps table tidy.
func TruncateName(name string, noTrunc bool, max int) string {
	if max <= 0 {
		max = 25
	}
	if noTrunc || len(name) <= max {
		return name
	}
	if max <= 1 {
		return name[:max]
	}
	return name[:max-1] + "…"
}

// HumanizeBytes formats bytes using IEC units (KiB, MiB, GiB).
func HumanizeBytes(b uint64) string {
	const (
		KiB = 1024
		MiB = 1024 * KiB
		GiB = 1024 * MiB
		TiB = 1024 * GiB
	)
	switch {
	case b >= TiB:
		return fmt.Sprintf("%.2fTiB", float64(b)/float64(TiB))
	case b >= GiB:
		return fmt.Sprintf("%.2fGiB", float64(b)/float64(GiB))
	case b >= MiB:
		return fmt.Sprintf("%.2fMiB", float64(b)/float64(MiB))
	case b >= KiB:
		return fmt.Sprintf("%.2fKiB", float64(b)/float64(KiB))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// Render renders to stdout using the requested format.
func Render(snaps []dkr.ContainerSnapshot, format OutputFormat, noTrunc bool, w io.Writer) error {
	switch format {
	case FormatJSON:
		return renderJSON(snaps, w)
	case FormatTable:
		fallthrough
	default:
		renderTable(snaps, noTrunc, w)
		return nil
	}
}

// RenderNetworks prints containers grouped by network in a readable table.
func RenderNetworks(groups map[string][]dkr.ContainerNetInfo, noTrunc bool, w io.Writer) error {
	// Prepare a deterministic order of networks
	networkNames := make([]string, 0, len(groups))
	for n := range groups {
		networkNames = append(networkNames, n)
	}
	sort.Strings(networkNames)

	tw := prettytable.NewWriter()
	if w == nil {
		tw.SetOutputMirror(os.Stdout)
	} else {
		tw.SetOutputMirror(w)
	}
	styleN := prettytable.StyleRounded
	styleN.Options.SeparateRows = true
	styleN.Color.Header = text.Colors{text.FgHiWhite, text.Bold}
	tw.SetStyle(styleN)
	width := detectTerminalWidth(w)
	if width > 0 {
		tw.SetAllowedRowLength(width)
	}
	tw.SetTitle(fmt.Sprintf("whale — networks: %d — %s", len(networkNames), time.Now().Format(time.Kitchen)))
	tw.AppendHeader(prettytable.Row{"NETWORK", "NAME", "ID", "STATUS"})
	// Wider NAME when grouped view
	nameMax := 40
	if width > 0 {
		if width-40 > 40 { // heuristic
			nameMax = width - 40
		}
		if nameMax > 60 {
			nameMax = 60
		}
	}
	tw.SetColumnConfigs([]prettytable.ColumnConfig{
		{Name: "NETWORK", WidthMax: 24, AutoMerge: true},
		{Name: "NAME", WidthMax: nameMax},
		{Name: "ID", WidthMax: 12},
		{Name: "STATUS", WidthMax: 24},
	})

	if len(networkNames) == 0 {
		tw.AppendFooter(prettytable.Row{"no networks", "", "", ""})
		tw.Render()
		return nil
	}

	for _, netName := range networkNames {
		containers := groups[netName]
		coloredNet := text.Colors{text.FgCyan}.Sprint(netName)
		for _, c := range containers {
			name := TruncateName(c.Name, noTrunc, nameMax)
			id := TruncateID(c.ID, noTrunc)
			status := colorStatus(c.Status)
			tw.AppendRow(prettytable.Row{coloredNet, name, id, status})
		}
	}
	tw.Render()
	return nil
}

func renderJSON(snaps []dkr.ContainerSnapshot, w io.Writer) error {
	// Convert to a machine-friendly structure with snake_case keys
	type row struct {
		Name       string  `json:"name"`
		ID         string  `json:"id"`
		Status     string  `json:"status"`
		CPUPercent float64 `json:"cpu_percent"`
		MemUsage   uint64  `json:"mem_usage"`
		MemLimit   uint64  `json:"mem_limit"`
		MemPercent float64 `json:"mem_percent"`
		NetRx      uint64  `json:"net_rx"`
		NetTx      uint64  `json:"net_tx"`
		BlockRead  uint64  `json:"block_read"`
		BlockWrite uint64  `json:"block_write"`
		PIDs       int     `json:"pids"`
	}
	rows := make([]row, 0, len(snaps))
	for _, s := range snaps {
		rows = append(rows, row{
			Name:       s.Name,
			ID:         s.ID,
			Status:     s.Status,
			CPUPercent: round1(s.CPUPercent),
			MemUsage:   s.MemUsage,
			MemLimit:   s.MemLimit,
			MemPercent: round1(s.MemPercent),
			NetRx:      s.NetRx,
			NetTx:      s.NetTx,
			BlockRead:  s.BlockRead,
			BlockWrite: s.BlockWrite,
			PIDs:       s.PIDs,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func renderTable(snaps []dkr.ContainerSnapshot, noTrunc bool, w io.Writer) {
	tw := prettytable.NewWriter()
	if w == nil {
		tw.SetOutputMirror(os.Stdout)
	} else {
		tw.SetOutputMirror(w)
	}
	// Use Unicode rounded borders + subtle row separation and a title
	style := prettytable.StyleRounded
	style.Options.SeparateRows = true
	style.Color.Header = text.Colors{text.FgHiWhite, text.Bold}
	tw.SetStyle(style)
	tw.SetTitle(fmt.Sprintf("whale — %d containers — %s", len(snaps), time.Now().Format(time.Kitchen)))
	// Detect terminal width and hint the writer to wrap as needed
	width := detectTerminalWidth(w)
	if width > 0 {
		tw.SetAllowedRowLength(width)
	}
	// Configure columns to scale with terminal width
	nameMax := 25
	if width <= 0 {
		width = 120
	}
	// Estimate fixed widths for non-NAME columns (content only)
	idMax := 12
	if noTrunc {
		idMax = 64
	}
	// Percent columns (CPU %, MEM %) are dynamic: number (6) + space (1) + bar ([..] length)
	cpuBarWidth := 10
	memBarWidth := 10
	percentDigits := 6 // e.g., "100.0"
	percentColWidthCPU := percentDigits + 1 + boolToInt(cpuBarWidth > 0)*(cpuBarWidth+2)
	// Merge MEM usage/limit and percent into a single MEM column width
	memColWidth := 26 + 1 + percentDigits + boolToInt(memBarWidth > 0)*(memBarWidth+2)
	netWidth := 22
	blkWidth := 22
	// total width model (borders + paddings + content widths) for 8 columns
	calcTotal := func() int {
		cols := 8
		sep := cols + 1
		pad := cols * 2
		return sep + pad + nameMax + idMax + 24 + percentColWidthCPU + memColWidth + netWidth + blkWidth + 5
	}
	// Adjust to fit terminal width by shrinking bars, then NAME, then NET/BLOCK, then MEM USAGE.
	// Coarse pass: shrink bars based on width tiers
	if width <= 80 {
		cpuBarWidth, memBarWidth = 2, 2
	} else if width <= 100 {
		cpuBarWidth, memBarWidth = 4, 4
	} else if width <= 120 {
		cpuBarWidth, memBarWidth = 8, 8
	}
	percentColWidthCPU = percentDigits + 1 + boolToInt(cpuBarWidth > 0)*(cpuBarWidth+2)
	memColWidth = 26 + 1 + percentDigits + boolToInt(memBarWidth > 0)*(memBarWidth+2)
	for total := calcTotal(); total > width; total = calcTotal() {
		switch {
		case cpuBarWidth > 0 || memBarWidth > 0:
			if cpuBarWidth > 0 {
				cpuBarWidth--
			}
			if memBarWidth > 0 {
				memBarWidth--
			}
			percentColWidthCPU = percentDigits + 1 + boolToInt(cpuBarWidth > 0)*(cpuBarWidth+2)
			memColWidth = 26 + 1 + percentDigits + boolToInt(memBarWidth > 0)*(memBarWidth+2)
		case nameMax > 12:
			nameMax--
		case netWidth > 16:
			netWidth--
		case blkWidth > 16:
			blkWidth--
		case memColWidth > 20:
			memColWidth--
		default:
			// nothing else to shrink
			break
		}
		// avoid infinite loop by breaking when nothing changes
		if calcTotal() == total {
			break
		}
	}
	// Recompute NAME width as the remainder to ensure total fits the terminal
	remainder := width - (8 + 1) /*separators*/ - (8 * 2) /*padding*/ - idMax - 24 - percentColWidthCPU - memColWidth - netWidth - blkWidth - 5
	if remainder < 12 {
		remainder = 12
	}
	if remainder > 60 {
		remainder = 60
	}
	nameMax = remainder

	tw.SetColumnConfigs([]prettytable.ColumnConfig{
		{Name: "NAME", WidthMax: nameMax},
		{Name: "ID", WidthMax: idMax},
		{Name: "STATUS", WidthMax: 24},
		{Name: "CPU %", Align: text.AlignRight, WidthMax: percentColWidthCPU},
		{Name: "MEM", WidthMax: memColWidth},
		{Name: "NET I/O", WidthMax: netWidth},
		{Name: "BLOCK I/O", WidthMax: blkWidth},
		{Name: "PIDS", Align: text.AlignRight, WidthMax: 5},
	})
	tw.AppendHeader(prettytable.Row{"NAME", "ID", "STATUS", "CPU %", "MEM", "NET I/O", "BLOCK I/O", "PIDS"})
	if len(snaps) == 0 {
		tw.AppendFooter(prettytable.Row{"no containers", "", "", "", "", "", "", "", ""})
		tw.Render()
		return
	}
	for _, s := range snaps {
		// Trim name to computed max
		name := TruncateName(s.Name, noTrunc, nameMax)
		id := TruncateID(s.ID, noTrunc)
		if noTrunc {
			// insert zero-width spaces so the long ID can wrap within the ID column
			id = softWrapToken(id, 12)
		}
		cpu := dashIfZeroPercent(s.CPUPercent)
		memUsage := "—"
		memLimit := "—"
		if s.MemLimit > 0 {
			memUsage = HumanizeBytes(s.MemUsage)
			memLimit = HumanizeBytes(s.MemLimit)
		}
		memPct := dashIfZeroPercent(s.MemPercent)
		netIO := printableIO(s.NetRx, s.NetTx)
		blkIO := printableIO(s.BlockRead, s.BlockWrite)
		pids := "—"
		if s.PIDs > 0 {
			pids = fmt.Sprintf("%d", s.PIDs)
		}

		// If stats couldn't be read, show blanks for numeric fields.
		if strings.EqualFold(s.Status, "ERROR") {
			cpu = ""
			memUsage, memLimit, memPct = "", "", ""
			netIO, blkIO = "", ""
			pids = ""
		}

		// Color coding
		status := colorStatus(s.Status)
		cpu = formatPercent(cpu, s.CPUPercent, cpuBarWidth)
		memPct = formatPercent(memPct, s.MemPercent, memBarWidth)

		// Build MEM combined cell: "usage / limit  <percent and bar>"
		memCombined := fmt.Sprintf("%s / %s", memUsage, memLimit)
		if memPct != "" {
			memCombined = fmt.Sprintf("%s  %s", memCombined, memPct)
		}
		tw.AppendRow(prettytable.Row{
			name,
			id,
			status,
			cpu,
			memCombined,
			netIO,
			blkIO,
			pids,
		})
	}
	tw.Render()
}

func detectTerminalWidth(w io.Writer) int {
	// Try to get terminal width from the writer if it's a file (stdout typically)
	if w == nil {
		if term.IsTerminal(int(os.Stdout.Fd())) {
			if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
				return width
			}
		}
		return 0
	}
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if width, _, err := term.GetSize(int(f.Fd())); err == nil {
			return width
		}
	}
	return 0
}

func printableIO(rx, tx uint64) string {
	if rx == 0 && tx == 0 {
		return "—"
	}
	return fmt.Sprintf("%s / %s", HumanizeBytes(rx), HumanizeBytes(tx))
}

func dashIfZeroPercent(p float64) string {
	if p == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f", p)
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func colorStatus(status string) string {
	s := strings.ToLower(status)
	switch {
	case s == "error":
		return text.Colors{text.FgHiRed}.Sprint(status)
	case strings.Contains(s, "up") || strings.Contains(s, "running"):
		return text.Colors{text.FgGreen}.Sprint(status)
	case strings.Contains(s, "paused"):
		return text.Colors{text.FgYellow}.Sprint(status)
	case strings.Contains(s, "exit") || strings.Contains(s, "dead") || strings.Contains(s, "stopped"):
		return text.Colors{text.FgRed}.Sprint(status)
	default:
		return status
	}
}

func colorPercentString(val string, pct float64) string {
	if val == "" || val == "—" {
		return val
	}
	bar := percentageBar(pct, 10)
	colored := val
	switch {
	case pct >= 80.0:
		colored = text.Colors{text.FgHiRed}.Sprint(val)
		bar = text.Colors{text.FgHiRed}.Sprint(bar)
	case pct >= 50.0:
		colored = text.Colors{text.FgYellow}.Sprint(val)
		bar = text.Colors{text.FgYellow}.Sprint(bar)
	default:
		colored = text.Colors{text.FgGreen}.Sprint(val)
		bar = text.Colors{text.FgGreen}.Sprint(bar)
	}
	return fmt.Sprintf("%s %s", colored, bar)
}

// formatPercent applies color and optionally appends a sized bar depending on width.
func formatPercent(val string, pct float64, barWidth int) string {
	if val == "" || val == "—" {
		return val
	}
	colored := val
	switch {
	case pct >= 80.0:
		colored = text.Colors{text.FgHiRed}.Sprint(val)
	case pct >= 50.0:
		colored = text.Colors{text.FgYellow}.Sprint(val)
	default:
		colored = text.Colors{text.FgGreen}.Sprint(val)
	}
	if barWidth <= 0 {
		return colored
	}
	bar := percentageBar(pct, barWidth)
	// Color the bar to match
	switch {
	case pct >= 80.0:
		bar = text.Colors{text.FgHiRed}.Sprint(bar)
	case pct >= 50.0:
		bar = text.Colors{text.FgYellow}.Sprint(bar)
	default:
		bar = text.Colors{text.FgGreen}.Sprint(bar)
	}
	return fmt.Sprintf("%s %s", colored, bar)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// softWrapToken inserts zero-width spaces every n characters to allow wrapping
// inside narrow columns without visually changing the token when not wrapping.
func softWrapToken(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		b.WriteRune(r)
		if (i+1)%n == 0 {
			b.WriteRune('\u200b') // zero-width space
		}
	}
	return b.String()
}

// ClearScreen clears the terminal and moves the cursor to the top-left.
// This is used by watch/stream modes to redraw the table in-place.
func ClearScreen(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	// ANSI escape sequence: clear entire screen (2J) and move cursor home (H)
	// We avoid additional sequences (like hiding cursor) to keep behavior simple.
	_, _ = io.WriteString(w, "\x1b[2J\x1b[H")
}

func percentageBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	// Smooth bar using partial blocks (eight-levels) + light separators
	// characters: ▏▎▍▌▊▉█; we use ramp of 1/8 steps
	ramp := []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉'}
	total := pct / 100.0 * float64(width)
	full := int(total)
	frac := total - float64(full)
	idx := int(frac*float64(len(ramp)-1) + 0.5)
	if idx >= len(ramp) {
		idx = len(ramp) - 1
	}
	var b strings.Builder
	for i := 0; i < full && i < width; i++ {
		b.WriteRune('█')
	}
	if full < width {
		b.WriteRune(ramp[idx])
		for i := full + 1; i < width; i++ {
			b.WriteRune(' ')
		}
	}
	// Wrap bar in light borders for aesthetics
	return "[" + b.String() + "]"
}
