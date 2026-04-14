package kb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

func TestValidateDocument(t *testing.T) {
	tests := []struct {
		name    string
		doc     store.Document
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid decision",
			doc: store.Document{
				Type:    "decision",
				Project: "acme-corp",
				Title:   "Use PostgreSQL",
				Content: "Performance benefits",
			},
			wantErr: false,
		},
		{
			name: "valid fact",
			doc: store.Document{
				Type:    "fact",
				Project: "acme-corp",
				Title:   "Database Type",
				Content: "PostgreSQL v15",
			},
			wantErr: false,
		},
		{
			name: "valid note",
			doc: store.Document{
				Type:    "note",
				Project: "tk-test",
				Title:   "Session note",
				Content: "Important context",
			},
			wantErr: false,
		},
		{
			name: "valid plan",
			doc: store.Document{
				Type:    "plan",
				Project: "tk-test",
				Title:   "Roadmap",
				Content: "Q1 goals",
			},
			wantErr: false,
		},
		{
			name: "valid relation",
			doc: store.Document{
				Type:    "relation",
				Project: "tk-test",
				Title:   "Dependency",
				Content: "linked to X",
			},
			wantErr: false,
		},
		{
			name: "missing type",
			doc: store.Document{
				Project: "acme-corp",
				Title:   "Use PostgreSQL",
				Content: "Performance",
			},
			wantErr: true,
			errMsg:  "type is required",
		},
		{
			name: "invalid type",
			doc: store.Document{
				Type:    "bogus",
				Project: "acme-corp",
				Title:   "Use PostgreSQL",
				Content: "Performance",
			},
			wantErr: true,
			errMsg:  "invalid type",
		},
		{
			name: "missing project",
			doc: store.Document{
				Type:    "decision",
				Title:   "Use PostgreSQL",
				Content: "Performance",
			},
			wantErr: true,
			errMsg:  "project is required",
		},
		{
			name: "missing title",
			doc: store.Document{
				Type:    "decision",
				Project: "acme-corp",
				Content: "Performance",
			},
			wantErr: true,
			errMsg:  "title is required",
		},
		{
			name: "missing content",
			doc: store.Document{
				Type:    "decision",
				Project: "acme-corp",
				Title:   "Use PostgreSQL",
			},
			wantErr: true,
			errMsg:  "content is required",
		},
		{
			name: "content exactly 500 chars",
			doc: store.Document{
				Type:    "decision",
				Project: "acme-corp",
				Title:   "Use PostgreSQL",
				Content: strings.Repeat("x", 500),
			},
			wantErr: false,
		},
		{
			name: "content 501 chars",
			doc: store.Document{
				Type:    "decision",
				Project: "acme-corp",
				Title:   "Use PostgreSQL",
				Content: strings.Repeat("x", 501),
			},
			wantErr: true,
			errMsg:  "content exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDocument(tt.doc)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
