package dashboard

import (
	"fmt"
	"html"
	"strings"
)

// defaultSectionLabel is the heading used for fragments whose Section is
// empty. Rendered first so unlabeled fragments (e.g. an ad-hoc view with no
// section discipline) still show up predictably.
const defaultSectionLabel = "General"

// RenderHTML renders a view's accumulated fragments as a complete,
// self-contained HTML page (doctype/head/body — not a fragment; this is
// opened directly via file://, unlike roadmap.RenderHTML's embeddable
// fragment). Fragments are grouped by Section in first-appearance order;
// within a section, fragments are further grouped by Project (in
// first-appearance order) only when the section spans more than one distinct
// project. refreshSecs > 0 adds a meta-refresh tag; 0 (or less) omits it.
func RenderHTML(viewName string, frags []Fragment, refreshSecs int) string {
	var b strings.Builder

	b.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	b.WriteString(`<meta charset="utf-8">` + "\n")
	fmt.Fprintf(&b, "<title>dashboard · %s</title>\n", html.EscapeString(viewName))
	if refreshSecs > 0 {
		fmt.Fprintf(&b, `<meta http-equiv="refresh" content="%d">`+"\n", refreshSecs)
	}
	b.WriteString(dashboardStyle)
	b.WriteString("</head>\n<body>\n")
	fmt.Fprintf(&b, `<header class="dash-header"><h1>%s</h1></header>`+"\n", html.EscapeString(viewName))

	if len(frags) == 0 {
		b.WriteString(`<p class="empty">no data</p>` + "\n")
	} else {
		renderSections(&b, frags)
	}

	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// sectionGroup accumulates one section's fragments in first-appearance
// order, alongside the first-appearance order of the distinct projects
// within it.
type sectionGroup struct {
	name     string
	frags    []Fragment
	projects []string // first-appearance order
	seenProj map[string]bool
}

// renderSections groups frags by Section (first-appearance order, empty
// Section rendered under defaultSectionLabel) and renders each in turn.
func renderSections(b *strings.Builder, frags []Fragment) {
	var order []string
	groups := make(map[string]*sectionGroup)

	for _, f := range frags {
		name := Section(f)
		if name == "" {
			name = defaultSectionLabel
		}
		g, ok := groups[name]
		if !ok {
			g = &sectionGroup{name: name, seenProj: make(map[string]bool)}
			groups[name] = g
			order = append(order, name)
		}
		g.frags = append(g.frags, f)
		if proj := FragmentProject(f); proj != "" && !g.seenProj[proj] {
			g.seenProj[proj] = true
			g.projects = append(g.projects, proj)
		}
	}

	for _, name := range order {
		g := groups[name]
		fmt.Fprintf(b, "<section class=\"dash-section\">\n<h2>%s</h2>\n", html.EscapeString(g.name))
		if len(g.projects) > 1 {
			renderByProject(b, g.frags, g.projects)
		} else {
			for _, f := range g.frags {
				renderFragment(b, f)
			}
		}
		b.WriteString("</section>\n")
	}
}

// renderByProject sub-groups a section's fragments by project (first
// appearance order in projects), emitting a project sub-header before each
// group. Fragments with no project (e.g. unstamped ad-hoc data) are grouped
// under the empty-string key and rendered without a sub-header.
func renderByProject(b *strings.Builder, frags []Fragment, projects []string) {
	byProject := make(map[string][]Fragment)
	for _, f := range frags {
		p := FragmentProject(f)
		byProject[p] = append(byProject[p], f)
	}

	// Unattributed fragments (no project) render first, without a header.
	for _, f := range byProject[""] {
		renderFragment(b, f)
	}

	for _, p := range projects {
		fmt.Fprintf(b, "<h3 class=\"dash-project\">%s</h3>\n", html.EscapeString(p))
		for _, f := range byProject[p] {
			renderFragment(b, f)
		}
	}
}

// renderFragment dispatches one fragment to its type-specific renderer.
func renderFragment(b *strings.Builder, f Fragment) {
	switch v := f.(type) {
	case Tile:
		renderTile(b, v)
	case Bar:
		renderBar(b, v)
	case Group:
		renderGroup(b, v)
	case Note:
		renderNote(b, v)
	case HTML:
		// The deliberate escape-hatch: inserted verbatim, never escaped.
		b.WriteString(v.HTML)
		b.WriteString("\n")
	}
}

// accentClasses is the fixed set of known Tile.Accent values; anything else
// maps to the neutral default.
var accentClasses = map[string]bool{
	"good": true,
	"warn": true,
	"bad":  true,
	"info": true,
}

func renderTile(b *strings.Builder, t Tile) {
	accent := "neutral"
	if accentClasses[t.Accent] {
		accent = t.Accent
	}
	fmt.Fprintf(b, "<div class=\"tile tile-%s\">\n", html.EscapeString(accent))
	fmt.Fprintf(b, "<div class=\"tile-label\">%s</div>\n", html.EscapeString(t.Label))
	fmt.Fprintf(b, "<div class=\"tile-value\">%s</div>\n", html.EscapeString(t.Value))
	if t.Sub != "" {
		fmt.Fprintf(b, "<div class=\"tile-sub\">%s</div>\n", html.EscapeString(t.Sub))
	}
	b.WriteString("</div>\n")
}

func renderBar(b *strings.Builder, bar Bar) {
	b.WriteString("<div class=\"bar\">\n")
	fmt.Fprintf(b, "<div class=\"bar-label\">%s</div>\n", html.EscapeString(bar.Label))
	if bar.Max != nil && *bar.Max > 0 {
		pct := bar.Value / *bar.Max * 100
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		fmt.Fprintf(b, "<div class=\"bar-track\"><div class=\"bar-fill\" style=\"width: %.2f%%\"></div></div>\n", pct)
	} else {
		fmt.Fprintf(b, "<div class=\"bar-value-text\">%s</div>\n", html.EscapeString(fmt.Sprintf("%g", bar.Value)))
	}
	if bar.Note != "" {
		fmt.Fprintf(b, "<div class=\"bar-note\">%s</div>\n", html.EscapeString(bar.Note))
	}
	b.WriteString("</div>\n")
}

func renderGroup(b *strings.Builder, g Group) {
	b.WriteString("<div class=\"dash-group\">\n")
	if g.Title != "" {
		fmt.Fprintf(b, "<h3 class=\"dash-group-title\">%s</h3>\n", html.EscapeString(g.Title))
	}
	b.WriteString(`<div class="cards">` + "\n")
	for _, c := range g.Cards {
		renderCard(b, c)
	}
	b.WriteString("</div>\n</div>\n")
}

func renderCard(b *strings.Builder, c Card) {
	b.WriteString(`<div class="card">` + "\n")
	fmt.Fprintf(b, "<div class=\"card-head\"><span class=\"title\">%s</span>", html.EscapeString(c.Title))
	if c.State != "" {
		fmt.Fprintf(b, " <span class=\"badge\">%s</span>", html.EscapeString(c.State))
	}
	b.WriteString("</div>\n")
	if c.Desc != "" {
		fmt.Fprintf(b, "<p class=\"desc\">%s</p>\n", html.EscapeString(c.Desc))
	}
	if len(c.Chips) > 0 {
		b.WriteString(`<div class="chips">`)
		for _, chip := range c.Chips {
			fmt.Fprintf(b, `<span class="chip">%s</span>`, html.EscapeString(chip))
		}
		b.WriteString("</div>\n")
	}
	if c.Ref != "" {
		fmt.Fprintf(b, "<div class=\"ref\">%s</div>\n", html.EscapeString(c.Ref))
	}
	b.WriteString("</div>\n")
}

func renderNote(b *strings.Builder, n Note) {
	fmt.Fprintf(b, "<p class=\"note\">%s</p>\n", html.EscapeString(n.Text))
}

// dashboardStyle is the self-contained, dependency-free stylesheet: CSS
// custom properties, a prefers-color-scheme dark default, and explicit
// data-theme overrides either way, mirroring roadmap.RenderHTML's
// brand-neutral discipline.
const dashboardStyle = `<style>
:root {
  --bg: #f7f7f8;
  --fg: #1a1a1a;
  --muted: #666;
  --card-bg: #ffffff;
  --border: #d8d8dc;
  --chip-bg: #eceef1;
  --badge-bg: #e2e4e8;
  --accent: #46586b;
  --bar-track: #e2e4e8;
  --bar-fill: #46586b;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme]) {
    --bg: #16171a;
    --fg: #e8e8ea;
    --muted: #9a9aa2;
    --card-bg: #1f2023;
    --border: #34353a;
    --chip-bg: #2b2c30;
    --badge-bg: #33343a;
    --accent: #8fa8c2;
    --bar-track: #33343a;
    --bar-fill: #8fa8c2;
  }
}
:root[data-theme="dark"] {
  --bg: #16171a;
  --fg: #e8e8ea;
  --muted: #9a9aa2;
  --card-bg: #1f2023;
  --border: #34353a;
  --chip-bg: #2b2c30;
  --badge-bg: #33343a;
  --accent: #8fa8c2;
  --bar-track: #33343a;
  --bar-fill: #8fa8c2;
}
:root[data-theme="light"] {
  --bg: #f7f7f8;
  --fg: #1a1a1a;
  --muted: #666;
  --card-bg: #ffffff;
  --border: #d8d8dc;
  --chip-bg: #eceef1;
  --badge-bg: #e2e4e8;
  --accent: #46586b;
  --bar-track: #e2e4e8;
  --bar-fill: #46586b;
}
body { max-width: 960px; margin: 0 auto; padding: 1rem; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: var(--bg); color: var(--fg); }
.dash-header h1 { font-size: 1.4rem; margin: 0 0 1rem; color: var(--accent); }
.dash-section { margin-bottom: 1.5rem; }
.dash-section h2 { font-size: 1.05rem; border-bottom: 1px solid var(--border); padding-bottom: .25rem; margin: 0 0 .5rem; }
.dash-project { font-size: .85rem; color: var(--muted); text-transform: uppercase; letter-spacing: .03em; margin: .75rem 0 .35rem; }
.empty { color: var(--muted); font-style: italic; }
.tile { display: inline-block; background: var(--card-bg); border: 1px solid var(--border); border-radius: 8px; padding: .6rem .9rem; margin: 0 .5rem .5rem 0; min-width: 120px; }
.tile-label { font-size: .75rem; color: var(--muted); }
.tile-value { font-size: 1.3rem; font-weight: 600; }
.tile-sub { font-size: .75rem; color: var(--muted); }
.tile-good .tile-value { color: #2e7d32; }
.tile-warn .tile-value { color: #b8860b; }
.tile-bad .tile-value { color: #b23b3b; }
.tile-info .tile-value { color: var(--accent); }
.bar { margin-bottom: .6rem; }
.bar-label { font-size: .85rem; margin-bottom: .2rem; }
.bar-track { background: var(--bar-track); border-radius: 4px; height: .5rem; overflow: hidden; }
.bar-fill { background: var(--bar-fill); height: 100%; }
.bar-value-text { font-size: .85rem; color: var(--muted); }
.bar-note { font-size: .75rem; color: var(--muted); }
.dash-group { margin-bottom: 1rem; }
.dash-group-title { font-size: .95rem; margin: 0 0 .4rem; }
.cards { display: flex; flex-direction: column; gap: .5rem; }
.card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 8px; padding: .6rem .8rem; }
.card-head { display: flex; align-items: center; gap: .5rem; flex-wrap: wrap; }
.title { font-weight: 600; }
.badge { font-size: .7rem; padding: .1rem .5rem; border-radius: 999px; background: var(--badge-bg); color: var(--fg); }
.desc { margin: .4rem 0; font-size: .85rem; }
.chips { margin-top: .35rem; display: flex; flex-wrap: wrap; gap: .3rem; }
.chip { font-size: .72rem; padding: .1rem .45rem; border-radius: 5px; background: var(--chip-bg); color: var(--muted); }
.ref { margin-top: .3rem; font-size: .75rem; font-family: ui-monospace, SFMono-Regular, monospace; color: var(--muted); }
.note { font-size: .9rem; }
</style>
`
