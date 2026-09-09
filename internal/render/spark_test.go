package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlotDotsRising(t *testing.T) {
	vals := []float64{0, 1, 2, 3, 4, 5, 6, 7}
	lines := PlotDots(vals, nil, 2)
	if len(lines) != 2 {
		t.Fatalf("rows = %d", len(lines))
	}
	joined := strings.Join(lines, "\n")
	if !containsBraille(joined) {
		t.Fatalf("expected braille dots:\n%s", joined)
	}
	// Last column should sit higher than the first: top row has a later cell set.
	top := []rune(lines[0])
	if len(top) < 2 {
		t.Fatalf("width %d", len(top))
	}
	if top[len(top)-1] == 0x2800 {
		t.Fatalf("rising series should light the top-right cell:\n%s", joined)
	}
}

func TestPlotDotsGapBreaksStroke(t *testing.T) {
	vals := []float64{8, 8, 1, 1}
	gap := []bool{false, false, true, false}
	lines := PlotDots(vals, gap, 2)
	plain := PlotDots(vals, nil, 2)
	if strings.Join(lines, "") == strings.Join(plain, "") {
		t.Fatal("reset gap should change the plot")
	}
}

func TestRenderHistoryEmpty(t *testing.T) {
	var buf bytes.Buffer
	RenderHistory(&buf, nil, false)
	if !strings.Contains(buf.String(), "(no history)") {
		t.Fatalf("%s", buf.String())
	}
}

func containsBraille(s string) bool {
	for _, r := range s {
		if r >= 0x2800 && r <= 0x28FF {
			return true
		}
	}
	return false
}
