package tui

import (
	"fmt"
	"math"
	"time"

	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/lipgloss"
)

type EquityPoint struct {
	Time   time.Time
	Equity float64
}

func (m *Model) recordEquityPoint(equity float64, at time.Time) {
	point := EquityPoint{
		Time:   at.UTC(),
		Equity: equity,
	}

	if len(m.equityHistory) > 0 {
		last := m.equityHistory[len(m.equityHistory)-1]
		if math.Abs(last.Equity-point.Equity) < 0.0001 && point.Time.Sub(last.Time) < 10*time.Second {
			return
		}
	}

	m.equityHistory = append(m.equityHistory, point)
	if m.equityMaxPoints > 0 && len(m.equityHistory) > m.equityMaxPoints {
		m.equityHistory = m.equityHistory[len(m.equityHistory)-m.equityMaxPoints:]
	}

	if m.onEquityPoint == nil {
		return
	}
	if err := m.onEquityPoint(point); err != nil {
		m.status = fmt.Sprintf("equity history persist failed: %v", err)
		m.statusError = true
	}
}

func buildEquitySparkline(points []EquityPoint, width, height int, style lipgloss.Style) string {
	if len(points) == 0 {
		return ""
	}
	if width < 8 {
		width = 8
	}
	if height < 3 {
		height = 3
	}
	values := sampleEquity(points, width)
	if len(values) == 0 {
		return ""
	}
	normalized, maxValue := normalizeSparklineValues(values)
	sl := sparkline.New(
		width,
		height,
		sparkline.WithStyle(style),
		sparkline.WithNoAutoMaxValue(),
		sparkline.WithMaxValue(maxValue),
	)
	sl.PushAll(normalized)
	sl.DrawBraille()
	return sl.View()
}

func sampleEquity(points []EquityPoint, width int) []float64 {
	if len(points) == 0 || width <= 0 {
		return nil
	}
	if len(points) <= width {
		out := make([]float64, len(points))
		for i, p := range points {
			out[i] = p.Equity
		}
		return out
	}

	out := make([]float64, width)
	lastIdx := len(points) - 1
	for i := 0; i < width; i++ {
		idx := int(math.Round(float64(i) * float64(lastIdx) / float64(width-1)))
		out[i] = points[idx].Equity
	}
	return out
}

func normalizeSparklineValues(values []float64) ([]float64, float64) {
	if len(values) == 0 {
		return nil, 1
	}

	minV := values[0]
	maxV := values[0]
	for _, v := range values[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	rangeV := maxV - minV
	if rangeV < 1e-9 {
		out := make([]float64, len(values))
		// Draw a centerline when equity is effectively flat.
		for i := range out {
			out[i] = 0.5
		}
		return out, 1
	}

	out := make([]float64, len(values))
	for i, v := range values {
		out[i] = v - minV
	}
	return out, rangeV
}
