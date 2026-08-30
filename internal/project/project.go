package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Identity describes a project independently of how it was selected by the CLI.
type Identity struct {
	ID         string
	Name       string
	Repository string
}

// Resolve identifies the project containing path. Git worktrees use their
// canonical repository root; other directories use their canonical path.
func Resolve(path string) (Identity, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Identity{}, fmt.Errorf("resolve project path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Identity{}, fmt.Errorf("resolve project symlinks: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return Identity{}, fmt.Errorf("inspect project path: %w", err)
	}
	if !info.IsDir() {
		return Identity{}, fmt.Errorf("project path %q is not a directory", canonical)
	}

	cmd := exec.Command("git", "-C", canonical, "rev-parse", "--show-toplevel")
	if output, gitErr := cmd.Output(); gitErr == nil {
		root := strings.TrimSpace(string(output))
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return Identity{}, fmt.Errorf("resolve Git root symlinks: %w", err)
		}
		canonical, err = filepath.Abs(root)
		if err != nil {
			return Identity{}, fmt.Errorf("resolve Git root: %w", err)
		}
	}
	canonical = filepath.Clean(canonical)
	sum := sha256.Sum256([]byte(canonical))
	return Identity{
		ID:         "project-" + hex.EncodeToString(sum[:16]),
		Name:       filepath.Base(canonical),
		Repository: canonical,
	}, nil
}
