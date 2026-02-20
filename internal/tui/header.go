package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

var (
	logoOnce  sync.Once
	logoLines []string
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
		return lipgloss.JoinVertical(lipgloss.Left, logo, summary)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, logo, strings.Repeat(" ", gap), summary)
}

func (m Model) buildAccountSummary() string {
	lines := []string{
		headerMetric("Cash", m.snapshot.Account.Cash),
		headerMetric("Buying Power", m.snapshot.Account.BuyingPower),
		headerMetric("Equity", m.snapshot.Account.Equity),
	}
	return strings.Join(lines, "\n")
}

func headerMetric(name string, value float64) string {
	return fmt.Sprintf("%s %s", headerLabelStyle.Render(name+":"), headerValueStyle.Render(fmt.Sprintf("$%.2f", value)))
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
