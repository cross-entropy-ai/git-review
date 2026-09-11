package gitdiff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func parseRaw(data string) ([]File, error) {
	var files []File
	parts := strings.Split(data, "\x00")
	for i := 0; i < len(parts)-1; {
		fields := strings.Fields(parts[i])
		i++
		if len(fields) != 5 || !strings.HasPrefix(fields[0], ":") || i >= len(parts)-1 {
			return nil, fmt.Errorf("invalid Git raw diff record")
		}
		f := File{OldMode: fields[0][1:], NewMode: fields[1], OldOID: fields[2], NewOID: fields[3], Status: fields[4], Path: parts[i]}
		i++
		if strings.HasPrefix(f.Status, "R") || strings.HasPrefix(f.Status, "C") {
			if i >= len(parts)-1 {
				return nil, fmt.Errorf("missing rename destination")
			}
			f.OldPath, f.Path = f.Path, parts[i]
			i++
		}
		files = append(files, f)
	}
	return files, nil
}

func applyNumstat(files []File, data string) error {
	byPath := make(map[string]int, len(files))
	for i, f := range files {
		byPath[f.Path] = i
	}
	parts := strings.Split(data, "\x00")
	seen := 0
	for i := 0; i < len(parts)-1; {
		fields := strings.SplitN(parts[i], "\t", 3)
		i++
		if len(fields) != 3 {
			return fmt.Errorf("invalid Git numstat record")
		}
		path := fields[2]
		if path == "" {
			if i+1 >= len(parts)-1 {
				return fmt.Errorf("invalid rename numstat record")
			}
			path = parts[i+1]
			i += 2
		}
		index, ok := byPath[path]
		if !ok {
			return fmt.Errorf("statistics for unknown path %q", path)
		}
		f := &files[index]
		f.Binary = fields[0] == "-" || fields[1] == "-"
		if !f.Binary {
			var err error
			f.Added, err = strconv.Atoi(fields[0])
			if err != nil {
				return fmt.Errorf("invalid additions: %w", err)
			}
			f.Deleted, err = strconv.Atoi(fields[1])
			if err != nil {
				return fmt.Errorf("invalid deletions: %w", err)
			}
		}
		seen++
	}
	if seen != len(files) {
		return fmt.Errorf("Git returned statistics for %d of %d files", seen, len(files))
	}
	return nil
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func applyPatch(files []File, patch string) error {
	index := -1
	typeHalf := false
	oldLine, newLine := 0, 0
	var hunk *Hunk
	for _, line := range strings.Split(strings.TrimSuffix(patch, "\n"), "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			// Git emits deletion and addition sections for a file-type change
			// (for example, regular file -> symlink), but one raw/stat record.
			if index >= 0 && files[index].Status == "T" && !typeHalf {
				typeHalf = true
			} else {
				index++
				typeHalf = false
			}
			if index >= len(files) {
				return fmt.Errorf("Git returned more patches than files")
			}
			hunk = nil
			continue
		}
		if index < 0 {
			continue
		}
		f := &files[index]
		if match := hunkHeader.FindStringSubmatch(line); match != nil {
			oldLine, _ = strconv.Atoi(match[1])
			newLine, _ = strconv.Atoi(match[2])
			f.Hunks = append(f.Hunks, Hunk{Header: line})
			hunk = &f.Hunks[len(f.Hunks)-1]
			continue
		}
		if hunk == nil {
			if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || line == "" {
				continue
			}
			f.Metadata = append(f.Metadata, line)
			continue
		}
		if line == "" {
			continue
		}
		entry := Line{Kind: line[0], Text: line[1:]}
		switch entry.Kind {
		case ' ':
			entry.Old, entry.New = oldLine, newLine
			oldLine++
			newLine++
		case '-':
			entry.Old = oldLine
			oldLine++
		case '+':
			entry.New = newLine
			newLine++
		case '\\':
		default:
			return fmt.Errorf("unexpected patch line in %q", f.Path)
		}
		hunk.Lines = append(hunk.Lines, entry)
	}
	if index+1 != len(files) {
		return fmt.Errorf("Git returned patches for %d of %d files", index+1, len(files))
	}
	return nil
}
