// Package review stores viewed-file progress for an exact comparison snapshot.
package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cross-entropy-ai/git-review/internal/gitdiff"
)

func Path(c *gitdiff.Comparison) string {
	sum := sha256.Sum256([]byte(c.MergeBase + "\x00" + c.HeadOID))
	return filepath.Join(c.GitDir, "git-review", hex.EncodeToString(sum[:12])+".json")
}

func Load(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]bool), nil
	}
	if err != nil {
		return nil, err
	}
	var viewed map[string]bool
	if err := json.Unmarshal(data, &viewed); err != nil {
		return nil, fmt.Errorf("read review progress: %w", err)
	}
	if viewed == nil {
		viewed = make(map[string]bool)
	}
	return viewed, nil
}

// Save replaces the progress file atomically; a failed write keeps the old state.
func Save(path string, viewed map[string]bool) error {
	data, err := json.MarshalIndent(viewed, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".progress-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
