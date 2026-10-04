package ui

import "strings"

// glyphs is a 5x5 pixel font for the letters of GOLEM.
var glyphs = map[rune][5]string{
	'G': {".###.", "#....", "#.###", "#...#", ".###."},
	'O': {".###.", "#...#", "#...#", "#...#", ".###."},
	'L': {"#....", "#....", "#....", "#....", "#####"},
	'E': {"#####", "#....", "####.", "#....", "#####"},
	'M': {"#...#", "##.##", "#.#.#", "#...#", "#...#"},
}

// GolemText renders GOLEM in block letters. Each font pixel is px cells
// wide; letters are separated by a gap of px cells.
func GolemText(px int) string {
	rows := make([]string, 5)
	for i, r := range "GOLEM" {
		for y, line := range glyphs[r] {
			if i > 0 {
				rows[y] += strings.Repeat(" ", px)
			}
			for _, c := range line {
				if c == '#' {
					rows[y] += strings.Repeat("█", px)
				} else {
					rows[y] += strings.Repeat(" ", px)
				}
			}
		}
	}
	return strings.Join(rows, "\n")
}
