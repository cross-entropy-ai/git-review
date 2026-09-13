// Package gitdiff reads local comparisons without changing Git state.
package gitdiff

import "github.com/cross-entropy-ai/git-review/internal/diff"

// Aliases keep Git parsing code on the shared comparison model.
type Comparison = diff.Comparison
type File = diff.File
type Hunk = diff.Hunk
type Line = diff.Line
type Mode = diff.Mode

const (
	ModeAuto        = diff.ModeAuto
	ModeWorkingTree = diff.ModeWorkingTree
	ModeCommitted   = diff.ModeCommitted
)

type Options struct {
	Dir     string
	Base    string
	Head    string
	Context int
	Mode    Mode
}
