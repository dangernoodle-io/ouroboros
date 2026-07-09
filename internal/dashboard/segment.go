package dashboard

import (
	"database/sql"
	"os"
	"os/exec"
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

	return []Fragment{
		NewTile("git", "branch", branch),
		NewTile("git", "uncommitted", strconv.Itoa(uncommitted)),
	}, nil
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
