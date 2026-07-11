package app

import (
	"database/sql"

	"github.com/dangernoodle-io/mcpkit"
	"github.com/dangernoodle-io/mcpkit/host/generic"
)

// buildServerV2 composes the mcpkit-typed successor to buildServer (OU-1:
// get+search only). It is NOT wired into app.Serve/cli — dark until cutover
// (OU-3), which will add the remaining write tools (kb/backlog/roadmap) and
// swap this in for buildServer.
//
// mcpkit.New has no server-instructions knob yet (as of the mcpkit pin this
// PR bumps to), so serverInstructions is not carried over here — a cutover
// follow-up once mcpkit grows that knob.
func buildServerV2(db *sql.DB, version string) (*mcpkit.App, error) {
	return mcpkit.New(mcpkit.Info{Name: "ouroboros", Version: version}, generic.New(),
		getCapability{db: db},
		searchCapability{db: db},
	)
}
