package render

import (
	"fmt"
	"io"
	"math"
	"strings"
)

// PlotDots draws a braille dot-matrix chart of values (oldest at left).
// rows is the number of braille character rows (each row is 4 dots).
// gap[i]==true still plots sample i but does not connect it to i-1 (window reset).
func PlotDots(values []float64, gap []bool, rows int) []string {
	if rows < 1 {
		rows = 1
	}
	if len(values) == 0 {
		empty := make([]string, rows)
		for i := range empty {
			empty[i] = ""
		}
		return empty
	}
	px := len(values) // one sample per braille column
	if px%2 == 1 {
		px++
	}
	py := rows * 4
	dots := make([][]bool, py)
	for y := range dots {
		dots[y] = make([]bool, px)
	}

	minV, maxV := math.Inf(1), math.Inf(-1)
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	if minV > maxV {
		return blank(rows, px/2)
	}
	span := maxV - minV
	mapY := func(v float64) int {
		if span == 0 {
			return py / 2
		}
		y := int((v-minV)/span*float64(py-1) + 0.5)
		if y < 0 {
			y = 0
		}
		if y >= py {
			y = py - 1
		}
		return y
	}

	var prevX, prevY int
	have := false
	for i, v := range values {
		if i < len(gap) && gap[i] {
			have = false
		}
		x := i
		y := mapY(v)
		if have {
			line(dots, prevX, prevY, x, y)
		} else {
			set(dots, x, y)
		}
		prevX, prevY, have = x, y, true
	}
	return renderBraille(dots, rows, px/2)
}

func blank(rows, cols int) []string {
	line := ""
	if cols > 0 {
		line = chars(cols, '\u2800')
	}
	out := make([]string, rows)
	for i := range out {
		out[i] = line
	}
	return out
}

func chars(n int, r rune) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = r
	}
	return string(b)
}

func set(dots [][]bool, x, y int) {
	if y < 0 || y >= len(dots) || x < 0 || x >= len(dots[0]) {
		return
	}
	dots[len(dots)-1-y][x] = true // y=0 at bottom
}

func line(dots [][]bool, x0, y0, x1, y1 int) {
	dx := absInt(x1 - x0)
	dy := absInt(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	for {
		set(dots, x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Braille dot bits (ISO 11548 / Unicode):
//
//	(1) (4)
//	(2) (5)
//	(3) (6)
//	(7) (8)
var brailleDot = [4][2]int{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

const sparkRows = 4

// SparkBlock is one window's last-N samples for a terminal dot-matrix plot.
type SparkBlock struct {
	Head  string
	Title string
	First string
	Last  string
	N     int
	Vals  []float64
	Gaps  []bool
}

// RenderHistory writes grouped braille charts. Head is "Provider  account".
func RenderHistory(w io.Writer, blocks []SparkBlock, color bool) {
	if len(blocks) == 0 {
		fmt.Fprintln(w, "(no history)")
		return
	}
	prevHead := ""
	for i, b := range blocks {
		if b.Head != prevHead {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintln(w, colored(b.Head, cBold+cCyan, color))
			prevHead = b.Head
		}
		trend := b.Last
		if b.First != "" && b.First != b.Last {
			trend = b.First + " → " + b.Last
		}
		fmt.Fprintf(w, "  %s  %s  n=%d\n", b.Title, trend, b.N)
		lines := PlotDots(b.Vals, b.Gaps, sparkRows)
		maxS, minS := sparkBounds(b.Vals)
		for r, line := range lines {
			label := strings.Repeat(" ", 6)
			joint := "│"
			switch r {
			case 0:
				label = fmt.Sprintf("%6s", maxS)
				joint = "┤"
			case len(lines) - 1:
				label = fmt.Sprintf("%6s", minS)
				joint = "┤"
			}
			fmt.Fprintf(w, "    %s%s%s\n", colored(label, cGray, color), colored(joint, cGray, color), line)
		}
	}
}

func sparkBounds(vals []float64) (maxS, minS string) {
	minV, maxV := math.Inf(1), math.Inf(-1)
	for _, v := range vals {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	if minV > maxV {
		return "", ""
	}
	return fmt.Sprintf("%.4g", maxV), fmt.Sprintf("%.4g", minV)
}

func renderBraille(dots [][]bool, rows, cols int) []string {
	out := make([]string, rows)
	for r := 0; r < rows; r++ {
		runes := make([]rune, cols)
		for c := 0; c < cols; c++ {
			bits := 0
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					y := r*4 + dy
					x := c*2 + dx
					if y < len(dots) && x < len(dots[y]) && dots[y][x] {
						bits |= brailleDot[dy][dx]
					}
				}
			}
			runes[c] = rune(0x2800 + bits)
		}
		out[r] = string(runes)
	}
	return out
}
