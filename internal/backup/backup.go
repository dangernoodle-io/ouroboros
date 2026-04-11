package backup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Backup struct {
	Mode      string
	RepoPath  string
	SparseDir string
}

func New(mode, repoPath, sparseDir string) *Backup {
	return &Backup{Mode: mode, RepoPath: repoPath, SparseDir: sparseDir}
}

func (b *Backup) IsEnabled() bool {
	return b.Mode == "dedicated" || b.Mode == "shared"
}

func (b *Backup) Commit(message string) error {
	if !b.IsEnabled() || b.RepoPath == "" {
		return nil
	}

	addPath := "."
	if b.Mode == "shared" && b.SparseDir != "" {
		addPath = b.SparseDir
	}

	cmd := exec.Command("git", "add", addPath)
	cmd.Dir = b.RepoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = b.RepoPath
	if cmd.Run() == nil {
		return nil // nothing to commit
	}

	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = b.RepoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	return nil
}

func (b *Backup) Init() error {
	if !b.IsEnabled() || b.RepoPath == "" {
		return nil
	}

	if err := os.MkdirAll(b.RepoPath, 0o755); err != nil {
		return err
	}

	gitDir := filepath.Join(b.RepoPath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return nil // already initialized
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = b.RepoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	if b.Mode == "shared" && b.SparseDir != "" {
		cmd = exec.Command("git", "sparse-checkout", "init")
		cmd.Dir = b.RepoPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("sparse-checkout init: %w", err)
		}

		cmd = exec.Command("git", "sparse-checkout", "add", b.SparseDir)
		cmd.Dir = b.RepoPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("sparse-checkout add: %w", err)
		}

		if err := os.MkdirAll(filepath.Join(b.RepoPath, b.SparseDir), 0o755); err != nil {
			return err
		}
	}

	return nil
}
