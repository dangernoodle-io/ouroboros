package roadmap_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/roadmap"
)

func TestRenderHTMLIsAFragmentWithTitleStyleAndWrap(t *testing.T) {
	html := roadmap.RenderHTML(&roadmap.Roadmap{}, "component", nil, nil)

	assert.Contains(t, html, "<title>")
	assert.Contains(t, html, "<style>")
	assert.Contains(t, html, `class="wrap"`)
	assert.NotContains(t, html, "<!doctype")
	assert.NotContains(t, html, "<html")
	assert.NotContains(t, html, "<head")
}

func TestRenderHTMLThemeVars(t *testing.T) {
	html := roadmap.RenderHTML(&roadmap.Roadmap{}, "component", nil, nil)

	assert.Contains(t, html, "prefers-color-scheme: dark")
	assert.Contains(t, html, `:root[data-theme="dark"]`)
	assert.Contains(t, html, `:root[data-theme="light"]`)
	assert.Contains(t, html, "--accent")
}

func TestRenderHTMLStateBadgesPerSection(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "now item"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNext, roadmap.Item{Title: "next item"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionDeferred, roadmap.Item{Title: "deferred item"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionParked, roadmap.Item{Title: "parked item", Why: "waiting", ResumeTrigger: "when unblocked"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionDropped, roadmap.Item{Title: "dropped item"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionDone, roadmap.Item{Title: "done item"})
	require.NoError(t, err)

	html := roadmap.RenderHTML(rm, "component", nil, nil)

	assert.Contains(t, html, "badge-now\">In flight</span>")
	assert.Contains(t, html, "badge-next\">Next</span>")
	assert.Contains(t, html, "badge-deferred\">Deferred</span>")
	assert.Contains(t, html, "badge-parked\">Parked</span>")
	assert.Contains(t, html, "badge-dropped\">Dropped</span>")
	assert.Contains(t, html, "badge-done\">Shipped</span>")
}

func TestRenderHTMLGroupByComponentVsEpicRegroupsSameItems(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x", Component: "widget", Epic: "BB-1"})
	require.NoError(t, err)

	byComponent := roadmap.RenderHTML(rm, "component", map[string]string{"BB-1": "WiFi map"}, nil)
	assert.Contains(t, byComponent, "component: widget")
	assert.Contains(t, byComponent, `chip chip-axis">epic: WiFi map</span>`)
	assert.NotContains(t, byComponent, "<h3 class=\"lane-header\">epic:")

	byEpic := roadmap.RenderHTML(rm, "epic", map[string]string{"BB-1": "WiFi map"}, nil)
	assert.Contains(t, byEpic, "epic: WiFi map")
	assert.Contains(t, byEpic, `chip chip-axis">component: widget</span>`)
	assert.NotContains(t, byEpic, "<h3 class=\"lane-header\">component:")

	// Same item titles present in both renders — regrouping, not filtering.
	assert.Contains(t, byComponent, ">x</span>")
	assert.Contains(t, byEpic, ">x</span>")
}

func TestRenderHTMLBodyPreservesNewlines(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{
		Title: "multi-line", Body: "first line\nsecond line",
	})
	require.NoError(t, err)

	html := roadmap.RenderHTML(rm, "component", nil, nil)
	assert.Contains(t, html, "white-space: pre-wrap")
	assert.Contains(t, html, "<p class=\"body\">first line\nsecond line</p>")
}

func TestRenderHTMLKBAndTicketChips(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{
		Title:  "chippy",
		KB:     []int{1, 2},
		Ticket: []string{"tk-test-000"},
	})
	require.NoError(t, err)

	html := roadmap.RenderHTML(rm, "component", nil, nil)
	assert.Contains(t, html, `chip chip-kb">kb: 1, 2</span>`)
	assert.Contains(t, html, `chip chip-ticket">ticket: tk-test-000</span>`)
}

func TestRenderHTMLChipsAreEscaped(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{
		Title:     `<script>alert(1)</script>`,
		Component: `"onmouseover=alert(1)`,
		Ticket:    []string{"<b>tk</b>"},
		BlockedBy: []roadmap.Blocker{{Project: "<i>bb</i>", Ref: "B<1>", Note: "note & <tag>"}},
	})
	require.NoError(t, err)

	html := roadmap.RenderHTML(rm, "component", nil, nil)
	assert.NotContains(t, html, "<script>alert(1)</script>")
	assert.Contains(t, html, "&lt;script&gt;")
	assert.NotContains(t, html, `"onmouseover=alert(1)`)
	assert.Contains(t, html, "&#34;onmouseover=alert(1)")
	assert.Contains(t, html, "&lt;b&gt;tk&lt;/b&gt;")
	assert.Contains(t, html, "&lt;i&gt;bb&lt;/i&gt;")
	assert.Contains(t, html, "B&lt;1&gt;")
	assert.Contains(t, html, "note &amp; &lt;tag&gt;")
}

func TestRenderHTMLBlockedByCallout(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{
		Title:     "waiting on upstream",
		BlockedBy: []roadmap.Blocker{{Project: "breadboard", Ref: "B1-716", Note: "needs the sensor API"}},
	})
	require.NoError(t, err)

	html := roadmap.RenderHTML(rm, "component", nil, nil)
	assert.Contains(t, html, `class="depmap"`)
	assert.Contains(t, html, "⛔ blocked by breadboard:B1-716")
	assert.Contains(t, html, "needs the sensor API")
}

func TestRenderHTMLParkedItemGateShowsWhyAndResume(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionParked, roadmap.Item{
		Title:         "parked work",
		Why:           "waiting on decision",
		ResumeTrigger: "when the ticket resolves",
	})
	require.NoError(t, err)

	html := roadmap.RenderHTML(rm, "component", nil, nil)
	assert.Contains(t, html, `class="gate"`)
	assert.Contains(t, html, "Why: waiting on decision")
	assert.Contains(t, html, "Resume trigger: when the ticket resolves")
}

func TestRenderHTMLNonParkedItemHasNoGate(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "active", Why: "n/a"})
	require.NoError(t, err)

	html := roadmap.RenderHTML(rm, "component", nil, nil)
	assert.NotContains(t, html, `class="gate"`)
}

func TestRenderHTMLBlockedEpicHeaderMarker(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x", Epic: "BB-1"})
	require.NoError(t, err)

	blocked := roadmap.RenderHTML(rm, "epic", map[string]string{"BB-1": "WiFi map"}, map[string]bool{"BB-1": true})
	assert.Contains(t, blocked, "epic: WiFi map")
	assert.Contains(t, blocked, `class="blocked-marker"`)
	assert.Contains(t, blocked, "⛔")

	unblocked := roadmap.RenderHTML(rm, "epic", map[string]string{"BB-1": "WiFi map"}, nil)
	assert.NotContains(t, unblocked, `class="blocked-marker"`)
}

func TestRenderHTMLEmptySection(t *testing.T) {
	html := roadmap.RenderHTML(&roadmap.Roadmap{}, "component", nil, nil)
	assert.Contains(t, html, `class="empty">none</p>`)
}

func TestWrapStandaloneHTML(t *testing.T) {
	fragment := roadmap.RenderHTML(&roadmap.Roadmap{}, "component", nil, nil)
	doc := roadmap.WrapStandaloneHTML(fragment)

	assert.Contains(t, doc, "<!doctype html>")
	assert.Contains(t, doc, `<meta charset="utf-8">`)
	assert.Contains(t, doc, "<body>")
	assert.Contains(t, doc, "</body>")
	assert.Contains(t, doc, fragment)
}
