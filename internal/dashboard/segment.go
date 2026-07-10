package dashboard

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"dangernoodle.io/ouroboros/internal/roadmap"
)

// Provider produces fragments for one segment, given the invocation context
// and an open database handle. A Provider degrades gracefully: it returns
// (nil, nil) when it has nothing to say, never a panic.
type Provider func(ctx Context, db *sql.DB) ([]Fragment, error)

var builtins = map[string]Provider{
	"git":     gitSegment,
	"roadmap": roadmapSegment,
}

// Builtin looks up a built-in segment provider by name.
func Builtin(name string) (Provider, bool) {
	p, ok := builtins[name]
	return p, ok
}

// BuiltinNames returns every built-in segment name, sorted.
func BuiltinNames() []string {
	names := make([]string, 0, len(builtins))
	for name := range builtins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// gitSegment reports the current branch and uncommitted-file count for the
// resolved repo dir. It degrades to (nil, nil) whenever the dir isn't a git
// repo or git errors — never an error return.
func gitSegment(ctx Context, _ *sql.DB) ([]Fragment, error) {
	dir := ctx.Repo
	if dir == "" {
		dir = ctx.Cwd
	}
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, nil //nolint:nilerr // degrade gracefully: no resolvable dir is not a segment error
		}
		dir = wd
	}

	branchOut, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return nil, nil //nolint:nilerr // degrade gracefully: not a git repo is not a segment error
	}
	branch := strings.TrimSpace(string(branchOut))

	statusOut, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return nil, nil //nolint:nilerr // degrade gracefully: git status failure is not a segment error
	}
	uncommitted := 0
	for _, line := range strings.Split(string(statusOut), "\n") {
		if strings.TrimSpace(line) != "" {
			uncommitted++
		}
	}

	frags := []Fragment{
		NewTile("git", "branch", branch),
		NewTile("git", "uncommitted", strconv.Itoa(uncommitted)),
	}

	if group, ok := gitWorktreesGroup(dir); ok {
		frags = append(frags, group)
	}

	return frags, nil
}

// gitWorktree is one entry parsed from `git worktree list --porcelain`.
type gitWorktree struct {
	path   string
	branch string
}

// gitWorktreesGroup builds a "worktrees" Group fragment for dir's linked
// worktrees. It returns ok=false when git errors (old git, not a repo) or
// when there's only the one (main) worktree — not worth a group.
func gitWorktreesGroup(dir string) (Group, bool) {
	out, err := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return Group{}, false //nolint:nilerr // degrade gracefully: git worktree failure is not a segment error
	}

	worktrees := parseWorktreePorcelain(string(out))
	if len(worktrees) < 2 {
		return Group{}, false
	}

	cards := make([]Card, 0, len(worktrees))
	for i, wt := range worktrees {
		card := Card{
			Title: filepath.Base(wt.path),
			Desc:  wt.branch,
		}
		if i == 0 {
			card.State = "main"
		}
		cards = append(cards, card)
	}

	return Group{
		V:       schemaVersion,
		Type:    "group",
		Section: "git",
		Title:   "worktrees",
		Cards:   cards,
	}, true
}

// parseWorktreePorcelain parses `git worktree list --porcelain` output into
// gitWorktree entries, in the order git reports them (main tree first).
func parseWorktreePorcelain(out string) []gitWorktree {
	var worktrees []gitWorktree
	var cur *gitWorktree

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			worktrees = append(worktrees, gitWorktree{path: strings.TrimPrefix(line, "worktree ")})
			cur = &worktrees[len(worktrees)-1]
		case cur == nil:
			continue
		case strings.HasPrefix(line, "branch "):
			cur.branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.branch = "detached"
		case line == "bare":
			cur.branch = "bare"
		}
	}

	return worktrees
}

// roadmapSegment reports non-empty section counts from ctx.Project's
// roadmap.
func roadmapSegment(ctx Context, db *sql.DB) ([]Fragment, error) {
	if ctx.Project == "" {
		return nil, nil
	}

	rm, err := roadmap.Load(db, ctx.Project)
	if err != nil {
		return nil, err
	}

	type namedSection struct {
		name  string
		count int
	}
	sections := []namedSection{
		{"now", len(rm.Sections.Now)},
		{"next", len(rm.Sections.Next)},
		{"deferred", len(rm.Sections.Deferred)},
		{"parked", len(rm.Sections.Parked)},
		{"dropped", len(rm.Sections.Dropped)},
		{"done", len(rm.Sections.Done)},
	}

	var frags []Fragment
	for _, s := range sections {
		if s.count == 0 {
			continue
		}
		frags = append(frags, NewTile("roadmap", s.name, strconv.Itoa(s.count)))
	}
	return frags, nil
}
