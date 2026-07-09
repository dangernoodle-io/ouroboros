package roadmap

import (
	"fmt"
	"html"
	"strings"
)

// stateBadge maps a section heading (as used by RenderMarkdown/renderSection)
// to its HTML badge label + a badge-modifier class, mirroring the Markdown
// heading vocabulary exactly so the two renders never disagree on state
// naming.
type stateBadge struct {
	label string
	class string
}

var sectionBadges = map[Section]stateBadge{
	SectionNow:      {"In flight", "now"},
	SectionNext:     {"Next", "next"},
	SectionDeferred: {"Deferred", "deferred"},
	SectionParked:   {"Parked", "parked"},
	SectionDropped:  {"Dropped", "dropped"},
	SectionDone:     {"Shipped", "done"},
}

// RenderHTML renders rm as a self-contained HTML fragment (a <title>, an
// inline <style> block, and a .wrap container — no <!doctype>/<html>/<head>,
// so it's embeddable as-is; a standalone viewer wraps it minimally, see the
// CLI's -o path). by/epicLabels/blockedEpics mirror RenderMarkdown exactly:
// by selects the grouping axis ("component" default, or "epic"), the other
// axis renders as an inline chip, epicLabels resolves an epic id to a
// display title, and blockedEpics marks epic ids whose epic item is itself
// blocked (rendered as a ⛔ on that epic's group header). All interpolated
// text is HTML-escaped. The palette is generic/brand-neutral: CSS custom
// properties, a prefers-color-scheme default, and data-theme overrides for
// explicit light/dark selection either way.
func RenderHTML(rm *Roadmap, by string, epicLabels map[string]string, blockedEpics map[string]bool) string {
	axis := normalizeAxis(by)

	var b strings.Builder
	b.WriteString("<title>Roadmap</title>\n")
	b.WriteString(htmlStyle)
	b.WriteString(`<div class="wrap">` + "\n")
	b.WriteString("<h1>Roadmap</h1>\n")

	renderHTMLSection(&b, SectionNow, rm.Sections.Now, false, axis, epicLabels, blockedEpics)
	renderHTMLSection(&b, SectionNext, rm.Sections.Next, false, axis, epicLabels, blockedEpics)
	renderHTMLSection(&b, SectionDeferred, rm.Sections.Deferred, true, axis, epicLabels, blockedEpics)
	renderHTMLSection(&b, SectionParked, rm.Sections.Parked, true, axis, epicLabels, blockedEpics)
	renderHTMLSection(&b, SectionDropped, rm.Sections.Dropped, false, axis, epicLabels, blockedEpics)
	renderHTMLSection(&b, SectionDone, rm.Sections.Done, false, axis, epicLabels, blockedEpics)

	b.WriteString("</div>\n")
	return b.String()
}

// WrapStandaloneHTML wraps a RenderHTML fragment in a minimal
// <!doctype html> document (charset meta + body) so a file written to disk
// opens cleanly as a standalone page in a browser. RenderHTML's output is
// deliberately fragment-only (no doctype/html/head) so it stays embeddable
// as-is elsewhere (e.g. published via an Artifact tool); this wrapper is for
// the CLI's -o file path only.
func WrapStandaloneHTML(fragment string) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<meta charset=\"utf-8\">\n<body>\n")
	b.WriteString(fragment)
	b.WriteString("</body>\n")
	return b.String()
}

func renderHTMLSection(b *strings.Builder, section Section, items []Item, parked bool, axis string, epicLabels map[string]string, blockedEpics map[string]bool) {
	badge := sectionBadges[section]

	fmt.Fprintf(b, "<section class=\"lane\">\n<h2>%s</h2>\n", html.EscapeString(badge.label))
	if len(items) == 0 {
		b.WriteString(`<p class="empty">none</p>` + "\n")
		b.WriteString("</section>\n")
		return
	}

	for _, g := range groupByAxis(items, axis) {
		if g.value != "" {
			renderHTMLGroupHeader(b, axis, g.value, epicLabels, blockedEpics)
		}
		b.WriteString(`<div class="cards">` + "\n")
		for _, it := range g.items {
			renderHTMLCard(b, it, badge, parked, axis, epicLabels)
		}
		b.WriteString("</div>\n")
	}
	b.WriteString("</section>\n")
}

// renderHTMLGroupHeader emits the group heading for a non-empty axis value,
// mirroring renderGroupHeader's Markdown labeling exactly (component: X, or
// epic: <label> with a ⛔ marker when blocked).
func renderHTMLGroupHeader(b *strings.Builder, axis, value string, epicLabels map[string]string, blockedEpics map[string]bool) {
	if axis == "epic" {
		label := epicLabels[value]
		if label == "" {
			label = value
		}
		marker := ""
		if blockedEpics[value] {
			marker = ` <span class="blocked-marker">⛔</span>`
		}
		fmt.Fprintf(b, "<h3 class=\"lane-header\">epic: %s%s</h3>\n", html.EscapeString(label), marker)
		return
	}
	fmt.Fprintf(b, "<h3 class=\"lane-header\">component: %s</h3>\n", html.EscapeString(value))
}

func renderHTMLCard(b *strings.Builder, it Item, badge stateBadge, parked bool, axis string, epicLabels map[string]string) {
	b.WriteString(`<div class="card">` + "\n")
	fmt.Fprintf(b, "<div class=\"card-head\"><span class=\"title\">%s</span> <span class=\"badge badge-%s\">%s</span></div>\n",
		html.EscapeString(it.Title), badge.class, html.EscapeString(badge.label))

	var chips []string
	if chip := htmlOtherAxisChip(it, axis, epicLabels); chip != "" {
		chips = append(chips, chip)
	}
	if len(it.KB) > 0 {
		chips = append(chips, fmt.Sprintf(`<span class="chip chip-kb">kb: %s</span>`, html.EscapeString(joinInts(it.KB))))
	}
	if len(it.Ticket) > 0 {
		chips = append(chips, fmt.Sprintf(`<span class="chip chip-ticket">ticket: %s</span>`, html.EscapeString(strings.Join(it.Ticket, ", "))))
	}
	if len(chips) > 0 {
		b.WriteString(`<div class="chips">` + strings.Join(chips, " ") + "</div>\n")
	}

	if it.Body != "" {
		fmt.Fprintf(b, "<p class=\"body\">%s</p>\n", html.EscapeString(it.Body))
	}

	for _, bl := range it.BlockedBy {
		line := fmt.Sprintf("%s:%s", html.EscapeString(bl.Project), html.EscapeString(bl.Ref))
		if bl.Note != "" {
			line += " — " + html.EscapeString(bl.Note)
		}
		fmt.Fprintf(b, "<div class=\"depmap\">⛔ blocked by %s</div>\n", line)
	}

	if parked && (it.Why != "" || it.ResumeTrigger != "") {
		b.WriteString(`<div class="gate">` + "\n")
		if it.Why != "" {
			fmt.Fprintf(b, "<div class=\"gate-why\">Why: %s</div>\n", html.EscapeString(it.Why))
		}
		if it.ResumeTrigger != "" {
			fmt.Fprintf(b, "<div class=\"gate-resume\">Resume trigger: %s</div>\n", html.EscapeString(it.ResumeTrigger))
		}
		b.WriteString("</div>\n")
	}

	b.WriteString("</div>\n")
}

// htmlOtherAxisChip is htmlOtherAxisChip's Markdown counterpart otherAxisChip
// rendered as an escaped HTML chip span instead of a backtick-wrapped
// literal, using the same label resolution (an epic chip shows
// epicLabels[it.Epic], falling back to the raw id) so a component chip and
// an epic group header never disagree on how an epic is displayed.
func htmlOtherAxisChip(it Item, axis string, epicLabels map[string]string) string {
	if axis == "epic" {
		if it.Component != "" {
			return fmt.Sprintf(`<span class="chip chip-axis">component: %s</span>`, html.EscapeString(it.Component))
		}
		return ""
	}
	if it.Epic != "" {
		label := epicLabels[it.Epic]
		if label == "" {
			label = it.Epic
		}
		return fmt.Sprintf(`<span class="chip chip-axis">epic: %s</span>`, html.EscapeString(label))
	}
	return ""
}

// htmlStyle is the self-contained, dependency-free stylesheet: no external
// CSS/JS/fonts, responsive (max-width wrap that reflows on narrow
// viewports), and theme-aware in both directions — a prefers-color-scheme
// default plus explicit data-theme="dark"/"light" overrides. The accent is a
// single restrained CSS var, not a brand palette.
const htmlStyle = `<style>
:root {
  --bg: #f7f7f8;
  --fg: #1a1a1a;
  --muted: #666;
  --card-bg: #ffffff;
  --border: #d8d8dc;
  --chip-bg: #eceef1;
  --badge-bg: #e2e4e8;
  --gate-bg: #fff8e6;
  --gate-border: #e0c974;
  --depmap-bg: #fbeaea;
  --depmap-border: #d98c8c;
  --accent: #46586b;
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
    --gate-bg: #332c14;
    --gate-border: #7a6425;
    --depmap-bg: #3a2222;
    --depmap-border: #7a3d3d;
    --accent: #8fa8c2;
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
  --gate-bg: #332c14;
  --gate-border: #7a6425;
  --depmap-bg: #3a2222;
  --depmap-border: #7a3d3d;
  --accent: #8fa8c2;
}
:root[data-theme="light"] {
  --bg: #f7f7f8;
  --fg: #1a1a1a;
  --muted: #666;
  --card-bg: #ffffff;
  --border: #d8d8dc;
  --chip-bg: #eceef1;
  --badge-bg: #e2e4e8;
  --gate-bg: #fff8e6;
  --gate-border: #e0c974;
  --depmap-bg: #fbeaea;
  --depmap-border: #d98c8c;
  --accent: #46586b;
}
.wrap { max-width: 960px; margin: 0 auto; padding: 1rem; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: var(--bg); color: var(--fg); }
.wrap h1 { font-size: 1.4rem; margin: 0 0 1rem; color: var(--accent); }
.lane { margin-bottom: 1.5rem; }
.lane h2 { font-size: 1.05rem; border-bottom: 1px solid var(--border); padding-bottom: .25rem; margin: 0 0 .5rem; }
.lane-header { font-size: .85rem; color: var(--muted); text-transform: uppercase; letter-spacing: .03em; margin: .75rem 0 .35rem; }
.empty { color: var(--muted); font-style: italic; margin: 0 0 .5rem; }
.cards { display: flex; flex-direction: column; gap: .5rem; }
.card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 8px; padding: .6rem .8rem; }
.card-head { display: flex; align-items: center; gap: .5rem; flex-wrap: wrap; }
.title { font-weight: 600; }
.badge { font-size: .7rem; padding: .1rem .5rem; border-radius: 999px; background: var(--badge-bg); color: var(--fg); }
.chips { margin-top: .35rem; display: flex; flex-wrap: wrap; gap: .3rem; }
.chip { font-size: .72rem; padding: .1rem .45rem; border-radius: 5px; background: var(--chip-bg); color: var(--muted); }
.body { margin: .4rem 0; font-size: .85rem; color: var(--fg); white-space: pre-wrap; }
.depmap { margin-top: .4rem; font-size: .8rem; padding: .3rem .5rem; border-radius: 5px; background: var(--depmap-bg); border: 1px solid var(--depmap-border); }
.gate { margin-top: .4rem; font-size: .8rem; padding: .3rem .5rem; border-radius: 5px; background: var(--gate-bg); border: 1px solid var(--gate-border); }
.blocked-marker { color: #b23b3b; }
@media (max-width: 480px) {
  .wrap { padding: .5rem; }
  .card-head { flex-direction: column; align-items: flex-start; }
}
</style>
`
