package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTargets(t *testing.T) {
	tests := []struct {
		name        string
		raw         []string
		want        []Target
		wantSkipped int
	}{
		{
			name: "bare domain",
			raw:  []string{"example.com"},
			want: []Target{
				"https://example.com",
			},
		},
		{
			name: "path as-is",
			raw:  []string{"https://x.io/health"},
			want: []Target{
				"https://x.io/health",
			},
		},
		{
			name: "query as-is",
			raw:  []string{"https://x.io/?q=1"},
			want: []Target{
				"https://x.io/?q=1",
			},
		},
		{
			name: "http and https",
			raw:  []string{"example.com", "http://example.com"},
			want: []Target{
				"https://example.com",
				"http://example.com",
			},
		},
		{
			name: "https as-is",
			raw:  []string{"https://x.io"},
			want: []Target{
				"https://x.io",
			},
		},
		{
			name: "http as-is",
			raw:  []string{"http://x.io"},
			want: []Target{
				"http://x.io",
			},
		},
		{
			name: "deduplication",
			raw:  []string{"example.com", "https://example.com", "https://example.com"},
			want: []Target{
				"https://example.com",
			},
		},
		{
			name: "first wins + order",
			raw:  []string{"example.com", "x.io", "https://example.com"},
			want: []Target{
				"https://example.com",
				"https://x.io",
			},
		},
		{
			name: "trim space",
			raw:  []string{"   example.com   "},
			want: []Target{
				"https://example.com",
			},
		},
		{
			name: "localhost",
			raw:  []string{"localhost", "https://localhost"},
			want: []Target{
				"https://localhost",
			},
		},
		{
			name: "full address with port as-is",
			raw:  []string{"https://example.com:8080"},
			want: []Target{
				"https://example.com:8080",
			},
		},
		{
			name: "mix keeps valid",
			raw:  []string{"ok.com", "ftp://x.io", "x.io"},
			want: []Target{
				"https://ok.com",
				"https://x.io",
			},
			wantSkipped: 1,
		},
		{
			name: "empty among valid",
			raw:  []string{"ok.com", "", "x.io"},
			want: []Target{
				"https://ok.com",
				"https://x.io",
			},
		},
		{
			name: "whitespace among valid",
			raw:  []string{"ok.com", "   "},
			want: []Target{
				"https://ok.com",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, skipped, err := ParseTargets(test.raw, false)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.wantSkipped, skipped)
		})
	}
}

func TestParseTargets_skipping(t *testing.T) {
	tests := []struct {
		name        string
		raw         []string
		wantSkipped int
	}{
		{
			name:        "empty ignored",
			raw:         []string{""},
			wantSkipped: 0,
		},
		{
			name:        "bare port skipped",
			raw:         []string{"example.com:8080"},
			wantSkipped: 1,
		},
		{
			name:        "invalid scheme",
			raw:         []string{"htt://x.io"},
			wantSkipped: 1,
		},
		{
			name:        "invalid scheme ftp",
			raw:         []string{"ftp://x.io"},
			wantSkipped: 1,
		},
		{
			name:        "empty host",
			raw:         []string{"https://"},
			wantSkipped: 1,
		},
		{
			name:        "empty input",
			raw:         []string{},
			wantSkipped: 0,
		},
		{
			name:        "whitespace ignored",
			raw:         []string{" "},
			wantSkipped: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, skipped, err := ParseTargets(test.raw, false)
			require.NoError(t, err)
			assert.Empty(t, got)
			assert.Equal(t, test.wantSkipped, skipped)
		})
	}
}

func TestParseTargets_failFast(t *testing.T) {
	tests := []struct {
		name        string
		raw         []string
		want        []Target
		wantSkipped int
		wantErr     error
	}{
		{
			name:    "fail-fast after valid",
			raw:     []string{"example.com", "ftp://x.io"},
			wantErr: ErrInvalidTarget,
		},
		{
			name:    "fail-fast first invalid",
			raw:     []string{"ftp://x.io", "ok.com"},
			wantErr: ErrInvalidTarget,
		},
		{
			name: "fail-fast ignores empty",
			raw:  []string{"ok.com", "", "x.io"},
			want: []Target{"https://ok.com", "https://x.io"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, skipped, err := ParseTargets(test.raw, true)
			if test.wantErr != nil {
				assert.ErrorIs(t, err, test.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.wantSkipped, skipped)
		})
	}
}
