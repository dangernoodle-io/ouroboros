package cli

import (
	"os"
	"path/filepath"
	"time"
)

// getCooldownDir builds a path under ${HOME}/.ouroboros/cooldowns/, joining
// any additional path segments (e.g. a per-hook subdir + a cooldown key).
// Faithful port of lib.js getCooldownDir: cooldown files live alongside the
// hook logs (not /tmp, which is shared multi-user and only sometimes
// survives a reboot).
func getCooldownDir(parts ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	segs := append([]string{home, ".ouroboros", "cooldowns"}, parts...)
	return filepath.Join(segs...)
}

// isWithinCooldown reports whether path exists and was last touched less
// than cooldownMs milliseconds ago. Fail-open: any stat error (including
// "file does not exist") is treated as "not within cooldown". Faithful port
// of lib.js isWithinCooldown.
func isWithinCooldown(path string, cooldownMs int64) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	elapsed := time.Since(info.ModTime())
	return elapsed < time.Duration(cooldownMs)*time.Millisecond
}

// touchFile creates path's parent directory (if needed) and updates path's
// mtime by rewriting it as an empty file. Best-effort: failures are
// swallowed, matching lib.js touchFile's fail-open behavior — a cooldown
// that fails to touch just re-fires next time, never blocks the hook.
func touchFile(path string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte{}, 0o644)
}
