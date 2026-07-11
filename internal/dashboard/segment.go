package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

// Provider produces fragments for one segment, given the caller's deadline
// ctx, the invocation context inv, and an open database handle. A Provider
// degrades gracefully: it returns (nil, nil) when it has nothing to say,
// never a panic. A builtin that spawns its own subprocess (e.g. githubSegment
// via `gh pr list`) must derive that subprocess's timeout from ctx
// (context.WithTimeout(ctx, ...) / exec.CommandContext(ctx, ...)) so it is cut
// when the aggregate refresh deadline fires, not just when its own internal
// cap elapses. Builtins that don't shell out may ignore ctx.
type Provider func(ctx context.Context, inv Context, db *sql.DB) ([]Fragment, error)

var builtins = map[string]Provider{
	"git":     gitSegment,
	"roadmap": roadmapSegment,
	"github":  githubSegment,
	"tickets": ticketsSegment,
	"kb":      kbSegment,
}

// ticketsRecentLimit bounds the recent-tickets card list. The "open" tile
// reports the open-item count up to the backlog ListItems clamp ceiling
// (500); a project with more than 500 open items renders 500 — an accepted
// limitation, since an at-a-glance tile doesn't need an exact count beyond
// that.
const ticketsRecentLimit = 5

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
// repo or git errors — never an error return. Its git subprocesses run under
// ctx so they're cut when the aggregate refresh deadline fires.
func gitSegment(ctx context.Context, inv Context, _ *sql.DB) ([]Fragment, error) {
	dir := inv.Repo
	if dir == "" {
		dir = inv.Cwd
	}
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, nil //nolint:nilerr // degrade gracefully: no resolvable dir is not a segment error
		}
		dir = wd
	}

	branchOut, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return nil, nil //nolint:nilerr // degrade gracefully: not a git repo is not a segment error
	}
	branch := strings.TrimSpace(string(branchOut))

	statusOut, err := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain").Output()
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

	if group, ok := gitWorktreesGroup(ctx, dir); ok {
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
func gitWorktreesGroup(ctx context.Context, dir string) (Group, bool) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "worktree", "list", "--porcelain").Output()
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

// roadmapSegment reports non-empty section counts from inv.Project's
// roadmap. It doesn't shell out, so it ignores ctx.
func roadmapSegment(_ context.Context, inv Context, db *sql.DB) ([]Fragment, error) {
	if inv.Project == "" {
		return nil, nil
	}

	rm, err := roadmap.Load(db, inv.Project)
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

// githubRepoPattern matches github.com owner/repo out of https, git@, and
// ssh:// remote URL forms, tolerating an optional trailing ".git".
var githubRepoPattern = regexp.MustCompile(`github\.com[:/]([^/]+)/(.+?)(?:\.git)?/?$`)

// parseGitHubRepo extracts "owner/repo" from a git remote URL, handling
// https://github.com/owner/repo(.git), git@github.com:owner/repo(.git), and
// ssh://git@github.com/owner/repo(.git) forms. It returns ok=false for any
// non-github.com host or an unparseable URL.
func parseGitHubRepo(remoteURL string) (string, bool) {
	m := githubRepoPattern.FindStringSubmatch(strings.TrimSpace(remoteURL))
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2], true
}

// githubPR is one entry parsed from `gh pr list --json number,title,author`.
type githubPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
}

// githubSegment reports a project's open pull requests via the `gh` CLI,
// tokenless. It resolves the repo dir the same way gitSegment does, reads
// the origin remote, and degrades to (nil, nil) whenever the dir isn't a
// git repo, has no github.com origin, or `gh` fails/is unavailable/times
// out — never an error return. Its `gh` subprocess's timeout derives from
// ctx (the caller's aggregate refresh deadline), capped at 10s, so it's cut
// promptly when ctx fires instead of running to its own internal cap.
func githubSegment(ctx context.Context, inv Context, _ *sql.DB) ([]Fragment, error) {
	dir := inv.Repo
	if dir == "" {
		dir = inv.Cwd
	}
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, nil //nolint:nilerr // degrade gracefully: no resolvable dir is not a segment error
		}
		dir = wd
	}

	remoteOut, err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return nil, nil //nolint:nilerr // degrade gracefully: no origin remote is not a segment error
	}

	repo, ok := parseGitHubRepo(string(remoteOut))
	if !ok {
		return nil, nil
	}

	ghCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ghCtx, "gh", "pr", "list", "--repo", repo, "--state", "open",
		"--json", "number,title,author", "--limit", "20")
	configureProcessGroup(cmd)
	cmd.WaitDelay = 2 * time.Second

	out, err := cmd.Output()
	if err != nil {
		return nil, nil //nolint:nilerr // degrade gracefully: gh missing/unauthed/timeout is not a segment error
	}

	var prs []githubPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, nil //nolint:nilerr // degrade gracefully: unexpected gh output is not a segment error
	}

	frags := []Fragment{
		NewTile("github", "open PRs", strconv.Itoa(len(prs))),
	}

	if len(prs) == 0 {
		return frags, nil
	}

	cards := make([]Card, 0, len(prs))
	for _, pr := range prs {
		desc := pr.Author.Login
		if desc != "" {
			desc = "@" + desc
		}
		cards = append(cards, Card{
			Title: "#" + strconv.Itoa(pr.Number) + " " + pr.Title,
			Desc:  desc,
		})
	}

	frags = append(frags, Group{
		V:       schemaVersion,
		Type:    "group",
		Section: "github",
		Title:   "pull requests",
		Cards:   cards,
	})

	return frags, nil
}

// ticketsSegment reports inv.Project's newest open backlog tickets: an
// "open" tile with the open count (see ticketsRecentLimit for the clamp
// ceiling), plus a "recent tickets" group
// carrying up to ticketsRecentLimit cards (newest first). It degrades to
// (nil, nil) whenever inv.Project is unset, the project doesn't exist, or
// the backlog read fails — never an error return. It doesn't shell out, so
// it ignores ctx.
func ticketsSegment(_ context.Context, inv Context, db *sql.DB) ([]Fragment, error) {
	if inv.Project == "" {
		return nil, nil
	}

	proj, err := backlog.GetProjectByName(db, inv.Project)
	if err != nil {
		return nil, nil //nolint:nilerr // degrade gracefully: unknown project is not a segment error
	}

	status := "open"
	items, err := backlog.ListItems(db, backlog.ItemFilter{
		ProjectIDs:    []int64{proj.ID},
		Status:        &status,
		SortByCreated: true,
		Limit:         500,
	})
	if err != nil {
		return nil, nil //nolint:nilerr // degrade gracefully: backlog read failure is not a segment error
	}

	frags := []Fragment{
		NewTile("tickets", "open", strconv.Itoa(len(items))),
	}

	if len(items) == 0 {
		return frags, nil
	}

	recent := items
	if len(recent) > ticketsRecentLimit {
		recent = recent[:ticketsRecentLimit]
	}

	cards := make([]Card, 0, len(recent))
	for _, item := range recent {
		cards = append(cards, Card{
			Title: item.ID + " " + item.Title,
			Desc:  item.Priority,
		})
	}

	frags = append(frags, Group{
		V:       schemaVersion,
		Type:    "group",
		Section: "tickets",
		Title:   "recent tickets",
		Cards:   cards,
	})

	return frags, nil
}

// kbSegment reports inv.Project's total KB entry count as a single "entries"
// tile. It degrades to (nil, nil) whenever inv.Project is unset, the project
// doesn't exist, or the KB count read fails — never an error return. It
// doesn't shell out, so it ignores ctx.
//
// kbSegment counts KB documents whose `project` exactly matches the backlog
// project's canonical name. KB entries and backlog projects share the same
// repo-derived project string in practice, so this is accurate; a KB entry
// written under a case-divergent project string would be counted separately
// (store.CountDocumentsByType is case-sensitive, unlike GetProjectByName) —
// a known limitation tracked in the backlog.
func kbSegment(_ context.Context, inv Context, db *sql.DB) ([]Fragment, error) {
	if inv.Project == "" {
		return nil, nil
	}

	proj, err := backlog.GetProjectByName(db, inv.Project)
	if err != nil {
		return nil, nil //nolint:nilerr // degrade gracefully: unknown project is not a segment error
	}

	counts, err := store.CountDocumentsByType(db, []string{proj.Name})
	if err != nil {
		return nil, nil //nolint:nilerr // degrade gracefully: KB count failure is not a segment error
	}

	total := 0
	for _, tc := range counts {
		total += tc.Count
	}

	return []Fragment{
		NewTile("kb", "entries", strconv.Itoa(total)),
	}, nil
}
