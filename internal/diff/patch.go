package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var headerPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ParseHunks reads a single file's API patch and rejects incomplete hunks.
func ParseHunks(patch string) ([]Hunk, error) {
	var hunks []Hunk
	old, next, oldLeft, newLeft := 0, 0, 0, 0
	for _, line := range strings.Split(strings.TrimSuffix(patch, "\n"), "\n") {
		if match := headerPattern.FindStringSubmatch(line); match != nil {
			if oldLeft != 0 || newLeft != 0 {
				return nil, fmt.Errorf("incomplete diff hunk")
			}
			values := [4]int{}
			for i, field := range match[1:] {
				if field == "" {
					values[i] = 1
					continue
				}
				n, err := strconv.Atoi(field)
				if err != nil {
					return nil, fmt.Errorf("invalid hunk line number")
				}
				values[i] = n
			}
			old, oldLeft, next, newLeft = values[0], values[1], values[2], values[3]
			hunks = append(hunks, Hunk{Header: line})
			continue
		}
		if len(hunks) == 0 || len(line) == 0 {
			return nil, fmt.Errorf("invalid diff hunk")
		}
		entry := Line{Kind: line[0], Text: line[1:]}
		switch entry.Kind {
		case ' ':
			entry.Old, entry.New = old, next
			old++
			next++
			oldLeft--
			newLeft--
		case '-':
			entry.Old = old
			old++
			oldLeft--
		case '+':
			entry.New = next
			next++
			newLeft--
		case '\\':
			if line != `\ No newline at end of file` {
				return nil, fmt.Errorf("invalid end-of-file marker")
			}
		default:
			return nil, fmt.Errorf("invalid diff line")
		}
		if oldLeft < 0 || newLeft < 0 {
			return nil, fmt.Errorf("diff hunk exceeds its line counts")
		}
		h := &hunks[len(hunks)-1]
		h.Lines = append(h.Lines, entry)
	}
	if oldLeft != 0 || newLeft != 0 {
		return nil, fmt.Errorf("incomplete diff hunk")
	}
	return hunks, nil
}
