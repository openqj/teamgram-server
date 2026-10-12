package http

import (
	"reflect"
	"testing"
)

func TestBotAPITokenCandidates(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "canonical telegram token", raw: "bot123:secret", want: []string{"123:secret", "bot123:secret"}},
		{name: "legacy opaque token", raw: "botweb-e2e-token", want: []string{"botweb-e2e-token", "web-e2e-token"}},
		{name: "slash route token", raw: "web-e2e-token", want: []string{"web-e2e-token"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := botAPITokenCandidates(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("botAPITokenCandidates(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}
