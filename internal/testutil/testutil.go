// Package testutil provides shared helpers for tests, notably the
// acceptance-test skip gate prescribed by the workspace Go conventions.
package testutil

import (
	"os"
	"testing"
)

// SkipUnlessAcc skips t unless ACC_OUROBOROS=1 is set in the environment.
// Acceptance tests spawn the real ouroboros binary and drive its MCP wire
// protocol end-to-end; they're opt-in so `go test ./...` stays fast and
// hermetic by default.
func SkipUnlessAcc(t *testing.T) {
	t.Helper()
	if os.Getenv("ACC_OUROBOROS") == "" {
		t.Skip("set ACC_OUROBOROS=1 to run acceptance tests")
	}
}
