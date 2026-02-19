package tui

import (
	"fmt"
	"math"
	"strings"
	"time"
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

func buildEquitySparkline(points []EquityPoint, width int) string {
	if len(points) == 0 {
		return ""
	}
	if width < 8 {
		width = 8
	}
	values := sampleEquity(points, width)
	if len(values) == 0 {
		return ""
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
	if math.Abs(maxV-minV) < 0.0001 {
		return strings.Repeat("-", len(values))
	}

	levels := []byte("._:-=+*#%@")
	out := make([]byte, len(values))
	for i, v := range values {
		norm := (v - minV) / (maxV - minV)
		idx := int(math.Round(norm * float64(len(levels)-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(levels) {
			idx = len(levels) - 1
		}
		out[i] = levels[idx]
	}
	return string(out)
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
