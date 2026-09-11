// Package gitdiff reads a committed, merge-base comparison without changing Git state.
package gitdiff

// Comparison is an immutable snapshot of the two resolved revisions.
type Comparison struct {
	Root      string
	GitDir    string
	Base      string
	Head      string
	BaseOID   string
	HeadOID   string
	MergeBase string
	Files     []File
	Added     int
	Deleted   int
}

// File describes a changed path and its unified diff.
type File struct {
	Path     string
	OldPath  string
	Status   string
	OldMode  string
	NewMode  string
	OldOID   string
	NewOID   string
	Added    int
	Deleted  int
	Binary   bool
	Metadata []string
	Hunks    []Hunk
}

// Hunk preserves Git's range header and each line's old/new position.
type Hunk struct {
	Header string
	Lines  []Line
}

type Line struct {
	Kind byte // ' ', '+', '-', or '\\' for a missing final newline.
	Text string
	Old  int
	New  int
}

type Options struct {
	Dir     string
	Base    string
	Head    string
	Context int
}
