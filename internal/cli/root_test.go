package cli

import (
	"testing"

	sheshacli "github.com/dangernoodle-io/shesha/cli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProvider is a minimal sheshacli.CommandProvider for exercising
// mustMountProviders' error path without depending on claudeProvider()'s
// real mount shape.
type stubProvider struct {
	mounts []sheshacli.Mount
}

func (s stubProvider) Mounts() []sheshacli.Mount { return s.mounts }

// TestMustMountProviders_Success proves the real claudeProvider() mounts
// cleanly (no panic) onto a fresh root, mirroring what init() does to
// rootCmd.
func TestMustMountProviders_Success(t *testing.T) {
	root := &cobra.Command{Use: "root"}

	require.NotPanics(t, func() {
		mustMountProviders(root, claudeProvider())
	})

	_, _, err := root.Find([]string{"claude", "hooks", "stop"})
	require.NoError(t, err)
}

// TestMustMountProviders_PanicsOnMountError proves the panic branch fires
// when MountProviders returns an error (an Under path naming no existing
// command) — restores coverage for the startup-config-bug guard in root.go.
func TestMustMountProviders_PanicsOnMountError(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	bad := stubProvider{
		mounts: []sheshacli.Mount{
			{Under: []string{"does-not-exist"}, Cmd: &cobra.Command{Use: "x"}},
		},
	}

	assert.PanicsWithValue(t,
		`ouroboros: mount claude provider: cli: MountAll: no command "does-not-exist" under root`,
		func() {
			mustMountProviders(root, bad)
		},
	)
}
