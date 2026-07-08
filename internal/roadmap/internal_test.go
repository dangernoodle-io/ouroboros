package roadmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ── white-box coverage for unexported helpers unreachable via the public API ─

func TestSectionSliceUnknownSectionReturnsNil(t *testing.T) {
	rm := &Roadmap{}
	assert.Nil(t, sectionSlice(rm, Section("bogus")))
}

func TestTruncateUTF8ShortStringReturnsUnchanged(t *testing.T) {
	// truncateUTF8 is only ever called via renderContent with a string
	// already known to exceed maxBytes, so the "already fits" early return
	// is dead through the public entry point; exercise it directly here.
	assert.Equal(t, "short", truncateUTF8("short", 100))
}
