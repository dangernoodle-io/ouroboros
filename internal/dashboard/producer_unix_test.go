//go:build unix

package dashboard

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureProcessGroup_NilProcessCancelIsProcessDone(t *testing.T) {
	cmd := exec.Command("true")
	configureProcessGroup(cmd)
	require.NotNil(t, cmd.Cancel)
	// Process is nil before Start — Cancel must not panic and must report done.
	assert.ErrorIs(t, cmd.Cancel(), os.ErrProcessDone)
}
