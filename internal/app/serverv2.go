package app

import (
	"github.com/dangernoodle-io/shesha"
	"github.com/dangernoodle-io/shesha/host/generic"
)

// serverInstructions is the MCP server instructions advertised to clients at
// initialize.
const serverInstructions = `Persist decisions and track work items across conversations.
query is the read tool (required domain: kb|backlog|roadmap); kb/backlog/roadmap are writes.

- Search before writing — avoid duplicates; kb upserts by type+project+category+title when id is absent, or updates in place when id is present (use this to retitle).
- Default response is summary; verbose=true only when full content/notes are needed.
- roadmap is a per-project singleton (now/next/deferred/parked/dropped/done sections); items carry two single-valued grouping axes, component and epic (an epic is a backlog item); query format=md|html&by=component|epic renders Markdown/HTML grouped on that axis, filterable by component/epic.
- Edges (blocks/relates/explains) link items/kb docs: backlog entries[].edges[] creates them inline at write time; kb content [[Title]] autolinks; query verbose=true surfaces an edges sidecar; CLI link/unlink/edges list for retrofits.
- Checkpoint after multi-step tasks; persist non-obvious decisions, update or delete stale ones.
- Never run sqlite3/raw SQL against the ouroboros DB file — on a tool failure, stop and report rather than improvising.`

// buildServerV2 composes the shesha-typed ouroboros MCP server: query
// (OU-1, collapsed from get+search at OU-323), kb write (OU-2), backlog
// write (OU-3), roadmap write (OU-4). It is pure composition — no I/O — so
// it is safe to call eagerly, before st.db is populated (see serverState's
// doc comment); every handler reads st.db AT CALL TIME, well after
// NewServerCommand's OnStart has opened it.
//
// version is a real production parameter (the server's advertised
// Info.Version, set from a build-time ldflags value); every test call site
// happens to pass the literal "test" today, which trips unparam once enough
// call sites share it -- not a signal the parameter is actually
// unused/constant.
//
//nolint:unparam // see above
func buildServerV2(st *serverState, version string) (*shesha.App, error) {
	return shesha.New(shesha.Info{Name: "ouroboros", Version: version, Instructions: serverInstructions}, generic.New(),
		queryCapability{st: st},
		kbCapability{st: st},
		backlogCapability{st: st},
		roadmapCapability{st: st},
	)
}
