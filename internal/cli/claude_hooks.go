package cli

import (
	mcpkitcli "github.com/dangernoodle-io/mcpkit/cli"
	"github.com/dangernoodle-io/mcpkit/host/claudecode"
	"github.com/dangernoodle-io/mcpkit/host/claudecode/hooks"
)

// claudeHooksProvider returns the mcpkit cli.CommandProvider contributing
// the `claude` host namespace ("everything Claude Code's plugin protocol
// invokes against this binary"). Only Stop is registered today — the
// remaining 6 events (SubagentStop, SubagentStart, UserPromptSubmit,
// PostToolUse ×2, PreCompact) port over in follow-on PRs.
//
// SessionStart is intentionally NOT registered here: plugin/scripts/
// bootstrap.js stays Node — it's what installs this very binary, so a
// Go-native SessionStart hook would have nothing to invoke it on a fresh
// install (the chicken-and-egg installer problem).
func claudeHooksProvider() mcpkitcli.CommandProvider {
	return claudecode.NewProvider(hooks.NewRegistry().Stop(hookHandleStop))
}
