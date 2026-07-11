package app

import (
	"context"
	"database/sql"

	"github.com/dangernoodle-io/mcpkit/cli"
	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/config"
	"dangernoodle.io/ouroboros/internal/store"
)

// serverState is a lazy-init holder for the mcpkit-composed server's shared
// *sql.DB. The 5 capabilities (capabilities_v2.go) each close over
// *serverState instead of *sql.DB directly, and every V2 handler reads
// st.db AT CALL TIME rather than at construction time. This matters because
// NewServerCommand calls buildServerV2 eagerly, while cobra is still
// building the command tree — well before the server should actually open
// the DB or run schema migrations. Building the command tree (so that
// `ouroboros --help` or any domain subcommand works) must not have that
// side effect. st.db stays nil until the "server" command's OnStart hook
// runs, immediately before mcpkit.App.Run starts serving — by which point
// every handler that dereferences st.db is safe to call.
type serverState struct {
	db *sql.DB
}

// NewServerCommand builds the "server" cobra command that runs the
// mcpkit-composed buildServerV2 server over stdio. buildServerV2 itself is
// pure composition (no I/O), so it is safe to call here, at command-tree
// construction time, with an as-yet-empty *serverState — no handler runs
// until mcpkit.App.Run does, which cli.ServerCmd only calls after OnStart
// has populated st.db.
//
// OnStart reuses the exact config.Load() -> store.InitDB(cfg.DBPath) chain
// the old app.Serve used. OnShutdown closes the DB. Signal handling
// (SIGTERM/SIGINT) and graceful shutdown are built into cli.ServerCmd — not
// hand-rolled here (the old app.Serve's manual signal.Notify goroutine is
// gone).
func NewServerCommand(version string) *cobra.Command {
	st := &serverState{}

	application, err := buildServerV2(st, version)
	if err != nil {
		// buildServerV2 composes a static capability list; a build error here
		// is not realistically reachable (see its own doc comment), but if it
		// ever occurs, fail loudly when the command actually runs rather than
		// panicking during cobra command-tree construction (which would break
		// every subcommand, not just "server").
		return &cobra.Command{
			Use:   "server",
			Short: "run the ouroboros MCP server over stdio",
			RunE: func(*cobra.Command, []string) error {
				return err
			},
		}
	}

	return cli.ServerCmd(cli.Server{
		App:        application,
		Use:        "server",
		Short:      "run the ouroboros MCP server over stdio",
		OnStart:    serverOnStart(st),
		OnShutdown: serverOnShutdown(st),
	})
}

// serverOnStart builds the OnStart hook: the exact config.Load() ->
// store.InitDB(cfg.DBPath) chain the old app.Serve used, populating st.db.
// Extracted from NewServerCommand as a named, independently testable
// function -- the real chain can be exercised directly (real config load +
// real DB open/close) without going through mcpkit.App.Run/real stdio.
func serverOnStart(st *serverState) func(context.Context) error {
	return func(context.Context) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		db, err := store.InitDB(cfg.DBPath)
		if err != nil {
			return err
		}

		st.db = db
		return nil
	}
}

// serverOnShutdown builds the OnShutdown hook: closes st.db if OnStart
// populated it; a nil-safe no-op otherwise (e.g. if OnStart itself failed
// before assigning st.db, cli.ServerCmd never calls OnShutdown, but this
// stays defensive).
func serverOnShutdown(st *serverState) func(context.Context) error {
	return func(context.Context) error {
		if st.db != nil {
			return st.db.Close()
		}
		return nil
	}
}
