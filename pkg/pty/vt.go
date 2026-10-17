package pty

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode"
)

// ScreenGrid represents an in-memory 2D virtual terminal grid.
type ScreenGrid struct {
	Cols    int
	Rows    int
	CursorX int
	CursorY int
	cells   [][]rune
}

// NewScreenGrid initializes an empty terminal grid filled with spaces.
func NewScreenGrid(cols, rows int) *ScreenGrid {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	grid := &ScreenGrid{
		Cols:  cols,
		Rows:  rows,
		cells: make([][]rune, rows),
	}

	for r := 0; r < rows; r++ {
		grid.cells[r] = make([]rune, cols)
		for c := 0; c < cols; c++ {
			grid.cells[r][c] = ' '
		}
	}
	return grid
}

// Resize dynamically resizes the screen buffer, preserving existing contents where possible.
func (g *ScreenGrid) Resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}

	newCells := make([][]rune, rows)
	for r := 0; r < rows; r++ {
		newCells[r] = make([]rune, cols)
		for c := 0; c < cols; c++ {
			if r < len(g.cells) && c < len(g.cells[r]) {
				newCells[r][c] = g.cells[r][c]
			} else {
				newCells[r][c] = ' '
			}
		}
	}

	g.cells = newCells
	g.Cols = cols
	g.Rows = rows

	if g.CursorX >= cols {
		g.CursorX = cols - 1
	}
	if g.CursorY >= rows {
		g.CursorY = rows - 1
	}
}

// Feed parses incoming byte stream and updates terminal screen matrix.
func (g *ScreenGrid) Feed(data string) {
	runes := []rune(data)
	n := len(runes)
	i := 0

	for i < n {
		ch := runes[i]

		// Check for ESC character (\x1b or \033)
		if ch == 0x1b && i+1 < n {
			seqLen := g.handleEscape(runes[i:])
			i += seqLen
			continue
		}

		switch ch {
		case '\r':
			g.CursorX = 0
		case '\n':
			g.lineFeed()
		case '\b':
			if g.CursorX > 0 {
				g.CursorX--
			}
		case '\t':
			nextTab := (g.CursorX/8 + 1) * 8
			if nextTab >= g.Cols {
				nextTab = g.Cols - 1
			}
			g.CursorX = nextTab
		case 0x07: // Bell, ignore
		default:
			if unicode.IsPrint(ch) {
				g.putChar(ch)
			}
		}
		i++
	}
}

func (g *ScreenGrid) putChar(ch rune) {
	if g.CursorY >= g.Rows {
		g.lineFeed()
	}
	if g.CursorX >= g.Cols {
		g.CursorX = 0
		g.lineFeed()
	}

	if g.CursorY < g.Rows && g.CursorX < g.Cols {
		g.cells[g.CursorY][g.CursorX] = ch
		g.CursorX++
	}
}

func (g *ScreenGrid) lineFeed() {
	if g.CursorY < g.Rows-1 {
		g.CursorY++
		return
	}

	// Scroll up by 1 line
	for r := 0; r < g.Rows-1; r++ {
		copy(g.cells[r], g.cells[r+1])
	}
	for c := 0; c < g.Cols; c++ {
		g.cells[g.Rows-1][c] = ' '
	}
}

func (g *ScreenGrid) handleEscape(seq []rune) int {
	if len(seq) < 2 {
		return 1
	}

	if seq[1] == '[' {
		// CSI sequence: parse until final letter (usually 0x40-0x7e)
		idx := 2
		for idx < len(seq) {
			r := seq[idx]
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '~' {
				cmd := r
				params := string(seq[2:idx])
				g.executeCSI(cmd, params)
				return idx + 1
			}
			idx++
		}
		return idx
	}

	return 2
}

func (g *ScreenGrid) executeCSI(cmd rune, params string) {
	parts := strings.Split(params, ";")

	switch cmd {
	case 'H', 'f': // Cursor Position: \033[row;colH (1-based)
		row := 1
		col := 1
		if len(parts) > 0 && parts[0] != "" {
			if parsedRow, err := strconv.Atoi(parts[0]); err == nil && parsedRow > 0 {
				row = parsedRow
			}
		}
		if len(parts) > 1 && parts[1] != "" {
			if parsedCol, err := strconv.Atoi(parts[1]); err == nil && parsedCol > 0 {
				col = parsedCol
			}
		}
		g.CursorY = row - 1
		if g.CursorY >= g.Rows {
			g.CursorY = g.Rows - 1
		}
		g.CursorX = col - 1
		if g.CursorX >= g.Cols {
			g.CursorX = g.Cols - 1
		}

	case 'J': // Erase in Display
		mode := 0
		if len(parts) > 0 && parts[0] != "" {
			mode, _ = strconv.Atoi(parts[0])
		}
		switch mode {
		case 2: // Clear entire screen and home cursor
			for r := 0; r < g.Rows; r++ {
				for c := 0; c < g.Cols; c++ {
					g.cells[r][c] = ' '
				}
			}
			g.CursorX = 0
			g.CursorY = 0
		case 0: // Clear from cursor to end of screen
			for c := g.CursorX; c < g.Cols; c++ {
				g.cells[g.CursorY][c] = ' '
			}
			for r := g.CursorY + 1; r < g.Rows; r++ {
				for c := 0; c < g.Cols; c++ {
					g.cells[r][c] = ' '
				}
			}
		}

	case 'K': // Erase in Line
		mode := 0
		if len(parts) > 0 && parts[0] != "" {
			mode, _ = strconv.Atoi(parts[0])
		}
		if g.CursorY < g.Rows {
			switch mode {
			case 0: // Clear from cursor to end of line
				for c := g.CursorX; c < g.Cols; c++ {
					g.cells[g.CursorY][c] = ' '
				}
			case 1: // Clear from beginning to cursor
				for c := 0; c <= g.CursorX && c < g.Cols; c++ {
					g.cells[g.CursorY][c] = ' '
				}
			case 2: // Clear entire line
				for c := 0; c < g.Cols; c++ {
					g.cells[g.CursorY][c] = ' '
				}
			}
		}

	case 'A': // Cursor Up
		count := 1
		if len(parts) > 0 && parts[0] != "" {
			if c, err := strconv.Atoi(parts[0]); err == nil && c > 0 {
				count = c
			}
		}
		g.CursorY -= count
		if g.CursorY < 0 {
			g.CursorY = 0
		}

	case 'B': // Cursor Down
		count := 1
		if len(parts) > 0 && parts[0] != "" {
			if c, err := strconv.Atoi(parts[0]); err == nil && c > 0 {
				count = c
			}
		}
		g.CursorY += count
		if g.CursorY >= g.Rows {
			g.CursorY = g.Rows - 1
		}

	case 'C': // Cursor Forward
		count := 1
		if len(parts) > 0 && parts[0] != "" {
			if c, err := strconv.Atoi(parts[0]); err == nil && c > 0 {
				count = c
			}
		}
		g.CursorX += count
		if g.CursorX >= g.Cols {
			g.CursorX = g.Cols - 1
		}

	case 'D': // Cursor Backward
		count := 1
		if len(parts) > 0 && parts[0] != "" {
			if c, err := strconv.Atoi(parts[0]); err == nil && c > 0 {
				count = c
			}
		}
		g.CursorX -= count
		if g.CursorX < 0 {
			g.CursorX = 0
		}
	}
}

// RenderText produces a clean, multiline string snapshot of the current terminal buffer.
func (g *ScreenGrid) RenderText() string {
	var lines []string
	for r := 0; r < g.Rows; r++ {
		line := string(g.cells[r])
		line = strings.TrimRight(line, " ")
		lines = append(lines, line)
	}

	// Trim trailing completely empty lines
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}

// RenderHTML generates a dark-mode styled HTML block representing terminal state.
func (g *ScreenGrid) RenderHTML() string {
	text := g.RenderText()
	escaped := html.EscapeString(text)
	return fmt.Sprintf("<pre style=\"background-color:#0d1117;color:#58a6ff;font-family:monospace;padding:12px;border-radius:6px;overflow-x:auto;\">%s</pre>", escaped)
}
