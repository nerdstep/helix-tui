package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	logoOnce  sync.Once
	logoLines []string
	nyLocOnce sync.Once
	nyLoc     *time.Location
)

func (m Model) buildHeader() string {
	logo := renderGradientLogo(loadLogoLines())
	summary := m.buildAccountSummary()
	if logo == "" {
		return summary
	}
	if summary == "" {
		return logo
	}

	headerWidth := m.width
	if headerWidth <= 0 {
		headerWidth = 132
	}
	logoWidth := lipgloss.Width(logo)
	summaryWidth := lipgloss.Width(summary)
	gap := headerWidth - logoWidth - summaryWidth
	if gap < 2 {
		gap = 2
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, logo, strings.Repeat(" ", gap), summary)
}

func (m Model) buildAccountSummary() string {
	now := time.Now()
	ny := now.In(newYorkLocation())
	marketLabel, marketOpen := marketSession(now)
	marketValue := mutedStyle.Render(marketLabel)
	if marketOpen {
		marketValue = positiveStyle.Render(marketLabel)
	} else if strings.Contains(marketLabel, "PRE") || strings.Contains(marketLabel, "AFTER") {
		marketValue = warnStyle.Render(marketLabel)
	}

	grid := [][]string{
		{
			headerMetric("Cash", m.snapshot.Account.Cash),
			headerMetric("Buying Power", m.snapshot.Account.BuyingPower),
			headerMetric("Equity", m.snapshot.Account.Equity),
		},
		{
			headerText("Market", marketValue),
			headerText("Clock", now.Local().Format("15:04:05 MST")),
			headerText("NY", ny.Format("15:04:05 MST")),
		},
		{
			headerText("Alpaca", m.headerAlpacaEnvStatus()),
			headerText("Agent", m.headerAgentStatus()),
			headerText("Last Sync", m.headerLastSyncStatus()),
		},
	}
	return renderHeaderGrid(grid, 2)
}

func headerMetric(name string, value float64) string {
	return headerText(name, headerValueStyle.Render(fmt.Sprintf("$%.2f", value)))
}

func headerText(name string, value string) string {
	return fmt.Sprintf("%s %s", headerLabelStyle.Render(name+":"), value)
}

func renderHeaderGrid(rows [][]string, gap int) string {
	if len(rows) == 0 {
		return ""
	}
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	if maxCols == 0 {
		return ""
	}

	colWidths := make([]int, maxCols)
	for _, row := range rows {
		for col := 0; col < len(row); col++ {
			if w := lipgloss.Width(row[col]); w > colWidths[col] {
				colWidths[col] = w
			}
		}
	}

	separator := strings.Repeat(" ", gap)
	rendered := make([]string, 0, len(rows))
	for _, row := range rows {
		cols := make([]string, 0, maxCols)
		for col := 0; col < maxCols; col++ {
			cell := ""
			if col < len(row) {
				cell = row[col]
			}
			cols = append(cols, lipgloss.NewStyle().
				Width(colWidths[col]).
				MaxWidth(colWidths[col]).
				Render(cell))
		}
		rendered = append(rendered, strings.Join(cols, separator))
	}
	return strings.Join(rendered, "\n")
}

func (m Model) headerAgentStatus() string {
	event := latestEventByType(m.snapshot.Events, "agent_mode")
	if event == nil {
		return headerValueStyle.Render("manual")
	}
	fields := parseEventFields(event.Details)
	mode := strings.TrimSpace(fields["mode"])
	agentType := strings.TrimSpace(fields["agent"])
	switch {
	case mode == "" && agentType == "":
		return headerValueStyle.Render("manual")
	case agentType == "":
		return headerValueStyle.Render(mode)
	case mode == "":
		return headerValueStyle.Render(agentType)
	default:
		return headerValueStyle.Render(fmt.Sprintf("%s/%s", mode, agentType))
	}
}

func (m Model) headerLastSyncStatus() string {
	event := latestEventByType(m.snapshot.Events, "sync")
	if event == nil {
		return mutedStyle.Render("n/a")
	}
	age := time.Since(event.Time)
	if age < 0 {
		age = 0
	}
	return headerValueStyle.Render(fmt.Sprintf("%s (%s ago)", formatLocalClock(event.Time), age.Round(time.Second)))
}

func (m Model) headerAlpacaEnvStatus() string {
	event := latestEventByType(m.snapshot.Events, "alpaca_config")
	if event == nil {
		return mutedStyle.Render("Unknown")
	}
	fields := parseEventFields(event.Details)
	switch strings.ToLower(strings.TrimSpace(fields["env"])) {
	case "live":
		return warnStyle.Render("Live")
	case "paper":
		return positiveStyle.Render("Paper")
	default:
		return mutedStyle.Render("Unknown")
	}
}

func parseEventFields(details string) map[string]string {
	fields := map[string]string{}
	for _, part := range strings.Fields(details) {
		if !strings.Contains(part, "=") {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return fields
}

func marketSession(now time.Time) (label string, marketOpen bool) {
	nyNow := now.In(newYorkLocation())
	if nyNow.Weekday() == time.Saturday || nyNow.Weekday() == time.Sunday {
		return "CLOSED (weekend)", false
	}

	minuteOfDay := nyNow.Hour()*60 + nyNow.Minute()
	switch {
	case minuteOfDay >= 9*60+30 && minuteOfDay < 16*60:
		return "OPEN (regular)", true
	case minuteOfDay >= 4*60 && minuteOfDay < 9*60+30:
		return "PRE (04:00-09:30 ET)", false
	case minuteOfDay >= 16*60 && minuteOfDay < 20*60:
		return "AFTER (16:00-20:00 ET)", false
	default:
		return "CLOSED", false
	}
}

func newYorkLocation() *time.Location {
	nyLocOnce.Do(func() {
		loc, err := time.LoadLocation("America/New_York")
		if err != nil {
			nyLoc = time.Local
			return
		}
		nyLoc = loc
	})
	return nyLoc
}

func loadLogoLines() []string {
	logoOnce.Do(func() {
		content := readLogoText()
		content = strings.TrimRight(content, "\r\n")
		if content == "" {
			logoLines = nil
			return
		}
		logoLines = strings.Split(content, "\n")
	})
	return append([]string{}, logoLines...)
}

func readLogoText() string {
	for _, path := range logoCandidates() {
		b, err := os.ReadFile(path)
		if err == nil {
			return string(b)
		}
	}
	return ""
}

func logoCandidates() []string {
	paths := []string{"logo.txt"}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return paths
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	paths = append([]string{filepath.Join(moduleRoot, "logo.txt")}, paths...)
	return paths
}

func renderGradientLogo(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		color := gradientColorHex([3]int{0x36, 0xA4, 0xFF}, [3]int{0x6D, 0xF0, 0xE8}, i, len(lines)-1)
		out = append(out, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(line))
	}
	return strings.Join(out, "\n")
}

func gradientColorHex(start [3]int, end [3]int, index int, maxIndex int) string {
	if maxIndex <= 0 {
		return fmt.Sprintf("#%02x%02x%02x", start[0], start[1], start[2])
	}
	r := lerpChannel(start[0], end[0], index, maxIndex)
	g := lerpChannel(start[1], end[1], index, maxIndex)
	b := lerpChannel(start[2], end[2], index, maxIndex)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func lerpChannel(start int, end int, index int, maxIndex int) int {
	delta := end - start
	return start + (delta*index)/maxIndex
}
