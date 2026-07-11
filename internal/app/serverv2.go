package app

import (
	"database/sql"

	"github.com/dangernoodle-io/mcpkit"
	"github.com/dangernoodle-io/mcpkit/host/generic"
)

// buildServerV2 composes the mcpkit-typed successor to buildServer (OU-1:
// get+search; OU-2 adds kb write; OU-3 adds backlog write). It is NOT wired
// into app.Serve/cli — dark until cutover, which will add the remaining
// write tool (roadmap) and swap this in for buildServer.
//
// mcpkit.New has no server-instructions knob yet (as of the mcpkit pin this
// PR bumps to), so serverInstructions is not carried over here — a cutover
// follow-up once mcpkit grows that knob.
//
// version is a real production parameter (the server's advertised
// Info.Version, e.g. set from a build-time ldflags value at cutover); every
// dark-path test call happens to pass the literal "test" today, which trips
// unparam once enough call sites share it -- not a signal the parameter is
// actually unused/constant.
//
//nolint:unparam // see above
func buildServerV2(db *sql.DB, version string) (*mcpkit.App, error) {
	return mcpkit.New(mcpkit.Info{Name: "ouroboros", Version: version}, generic.New(),
		getCapability{db: db},
		searchCapability{db: db},
		kbCapability{db: db},
		backlogCapability{db: db},
	)
}
