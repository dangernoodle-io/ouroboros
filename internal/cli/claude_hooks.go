package cli

import (
	sheshacli "github.com/dangernoodle-io/shesha/cli"
	"github.com/dangernoodle-io/shesha/host/claudecode"
	"github.com/dangernoodle-io/shesha/host/claudecode/hooks"
	"github.com/dangernoodle-io/shesha/host/claudecode/statusline"
	"github.com/muesli/termenv"
)

// claudeProvider returns the shesha cli.CommandProvider contributing the
// `claude` host namespace ("everything Claude Code's plugin protocol
// invokes against this binary"): `claude hooks` (Stop, SubagentStop,
// UserPromptSubmit, SubagentStart, PostToolUse, and PreCompact are
// registered today — PostToolUse dispatches its Edit-family branch (OU-275)
// and Bash/git-commit branch (OU-277); PreCompact is a pure log-only
// observability breadcrumb, see claude_hooks_pre_compact.go)
// plus `claude statusline` (OU-272), which replaces the old top-level
// `ouroboros statusline` command. WithAppPrefix("OUROBOROS") wires the
// session resolver's env-var override tier for parity with a future
// session-scoped consumer, even though the statusline provider itself is
// project-scoped, not session-scoped, today.
//
// SessionStart is intentionally NOT registered here: plugin/scripts/
// bootstrap.js stays Node — it's what installs this very binary, so a
// Go-native SessionStart hook would have nothing to invoke it on a fresh
// install (the chicken-and-egg installer problem).
func claudeProvider() sheshacli.CommandProvider {
	return claudecode.NewProvider(
		hooks.NewRegistry().
			Stop(hookHandleStop).
			SubagentStop(hookHandleSubagentStop).
			UserPromptSubmit(hookHandleUserPromptSubmit).
			SubagentStart(hookHandleSubagentStart).
			PostToolUse(hookHandlePostToolUse).
			PreCompact(hookHandlePreCompact),
		statusline.Command(
			ouroborosStatuslineProvider(),
			statusline.WithAppPrefix("OUROBOROS"),
			statusline.WithForceProfile(termenv.ANSI),
		),
	)
}
