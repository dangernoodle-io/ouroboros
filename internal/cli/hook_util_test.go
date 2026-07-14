package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- resolveHookProject ------------------------------------------------

func TestResolveHookProject_CwdInHub_Excluded(t *testing.T) {
	_, marketplaceDir, _ := upcWorkspace(t)
	assert.Equal(t, "", resolveHookProject("", marketplaceDir))
}

func TestResolveHookProject_CwdInOrdinaryRepo_ReturnsBaseName(t *testing.T) {
	_, _, ouroborosDir := upcWorkspace(t)
	assert.Equal(t, "ouroboros", resolveHookProject("", ouroborosDir))
}

func TestResolveHookProject_ProjectDirNonHub_CwdEmpty(t *testing.T) {
	_, _, ouroborosDir := upcWorkspace(t)
	assert.Equal(t, "ouroboros", resolveHookProject(ouroborosDir, ""))
}

func TestResolveHookProject_ProjectDirIsHub_FallsThroughToCwd(t *testing.T) {
	root, marketplaceDir, ouroborosDir := upcWorkspace(t)
	_ = root
	assert.Equal(t, "ouroboros", resolveHookProject(marketplaceDir, ouroborosDir))
}

func TestResolveHookProject_BothEmpty_ReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", resolveHookProject("", ""))
}

func TestResolveHookProject_BothNonGitPaths_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, "", resolveHookProject(filepath.Join(dir, "a"), filepath.Join(dir, "b")))
}

func TestResolveHookProject_ProjectDirNoGitRoot_FallsThroughToCwdGitRoot(t *testing.T) {
	nonGit := t.TempDir()
	_, _, ouroborosDir := upcWorkspace(t)
	assert.Equal(t, "ouroboros", resolveHookProject(nonGit, ouroborosDir))
}

func TestResolveHookProject_ProjectDirHub_CwdAlsoHub_ReturnsEmpty(t *testing.T) {
	_, marketplaceDir, _ := upcWorkspace(t)
	assert.Equal(t, "", resolveHookProject(marketplaceDir, marketplaceDir))
}

// sanity: temp fixture helper used above stays valid (reuses gitDir shape
// from upcWorkspace already exercised by claude_hooks_user_prompt_submit_test.go).
func TestResolveHookProject_Fixture_Sanity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "myproject")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	assert.Equal(t, "myproject", resolveHookProject("", root))
}
