package util

import (
	"strings"
)

// Lines returns the first n lines with no more than l runes. A line of greater than l runes
// will be truncated to l runes and will be the last line.
func Lines(s string, n, l int) string {
	var buf strings.Builder

	lc := 0
	rc := 0
	for _, r := range s {
		if r == '\n' {
			lc += 1
			if lc >= n {
				break
			}

			rc = 0
			buf.WriteRune(r)
		} else {
			rc += 1
			if rc > l {
				buf.WriteString("...")
				break
			}

			buf.WriteRune(r)
		}
	}

	return buf.String()
}
