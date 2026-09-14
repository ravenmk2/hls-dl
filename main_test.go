package main

import "testing"

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name string
		raw  []string
		want map[string]string
	}{
		{
			name: "basic",
			raw:  []string{"Referer:https://example.com"},
			want: map[string]string{"Referer": "https://example.com"},
		},
		{
			name: "trims spaces",
			raw:  []string{" Cookie : uid=42 "},
			want: map[string]string{"Cookie": "uid=42"},
		},
		{
			name: "value may contain colon",
			raw:  []string{"X-Data:a:b:c"},
			want: map[string]string{"X-Data": "a:b:c"},
		},
		{
			name: "ignores malformed entries",
			raw:  []string{"NoColonHere", ":no-key", ""},
			want: map[string]string{"": "no-key"},
		},
		{
			name: "multiple",
			raw:  []string{"A:1", "B:2"},
			want: map[string]string{"A": "1", "B": "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHeaders(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("parseHeaders(%v) = %v, want %v", tt.raw, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseHeaders(%v)[%q] = %q, want %q", tt.raw, k, got[k], v)
				}
			}
		})
	}
}
