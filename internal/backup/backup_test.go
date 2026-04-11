package backup

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBackup(t *testing.T) {
	b := New("dedicated", "/repo/path", "/sparse")
	assert.Equal(t, "dedicated", b.Mode)
	assert.Equal(t, "/repo/path", b.RepoPath)
	assert.Equal(t, "/sparse", b.SparseDir)
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected bool
	}{
		{"none mode", "none", false},
		{"dedicated mode", "dedicated", true},
		{"shared mode", "shared", true},
		{"unknown mode", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New(tt.mode, "/path", "")
			assert.Equal(t, tt.expected, b.IsEnabled())
		})
	}
}

func TestCommitDisabled(t *testing.T) {
	b := New("none", "/repo", "")
	err := b.Commit("test message")
	assert.NoError(t, err)
}

func TestCommitDisabledNoPath(t *testing.T) {
	b := New("dedicated", "", "")
	err := b.Commit("test message")
	assert.NoError(t, err)
}

func TestInitDisabled(t *testing.T) {
	b := New("none", "/repo", "")
	err := b.Init()
	assert.NoError(t, err)
}

func TestInitDisabledNoPath(t *testing.T) {
	b := New("dedicated", "", "")
	err := b.Init()
	assert.NoError(t, err)
}
