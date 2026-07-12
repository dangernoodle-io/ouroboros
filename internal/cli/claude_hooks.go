package cli

import (
	mcpkitcli "github.com/dangernoodle-io/mcpkit/cli"
	"github.com/dangernoodle-io/mcpkit/host/claudecode"
	"github.com/dangernoodle-io/mcpkit/host/claudecode/hooks"
	"github.com/dangernoodle-io/mcpkit/host/claudecode/statusline"
)

// claudeProvider returns the mcpkit cli.CommandProvider contributing the
// `claude` host namespace ("everything Claude Code's plugin protocol
// invokes against this binary"): `claude hooks` (Stop, SubagentStop,
// UserPromptSubmit, and SubagentStart are registered today — the remaining
// 3 events (PostToolUse ×2, PreCompact) port over in follow-on PRs)
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
func claudeProvider() mcpkitcli.CommandProvider {
	return claudecode.NewProvider(
		hooks.NewRegistry().
			Stop(hookHandleStop).
			SubagentStop(hookHandleSubagentStop).
			UserPromptSubmit(hookHandleUserPromptSubmit).
			SubagentStart(hookHandleSubagentStart),
		statusline.Command(ouroborosStatuslineProvider(), statusline.WithAppPrefix("OUROBOROS")),
	)
}
