package kb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAppendNotesText_EmptyBase verifies appending onto never-set/empty
// notes returns the addition alone, with no leading "\n\n" separator.
func TestAppendNotesText_EmptyBase(t *testing.T) {
	got := appendNotesText("", "first note")
	assert.Equal(t, "first note", got)
}

// TestAppendNotesText_NonEmptyBase verifies appending onto existing notes
// joins base and addition with a blank-line separator.
func TestAppendNotesText_NonEmptyBase(t *testing.T) {
	got := appendNotesText("existing note", "second note")
	assert.Equal(t, "existing note\n\nsecond note", got)
}
