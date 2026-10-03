package core

import (
	"strings"
	"testing"
)

func TestNormalizeSignupName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		required bool
		want     string
		ok       bool
	}{
		{name: "trim surrounding whitespace", input: "  Ada  ", required: true, want: "Ada", ok: true},
		{name: "reject whitespace only required name", input: " \t ", required: true, ok: false},
		{name: "allow empty optional name", input: "  ", required: false, want: "", ok: true},
		{name: "allow 64 unicode runes", input: strings.Repeat("界", maxSignupNameRunes), required: true, want: strings.Repeat("界", maxSignupNameRunes), ok: true},
		{name: "reject more than 64 unicode runes", input: strings.Repeat("界", maxSignupNameRunes+1), required: true, ok: false},
		{name: "reject control characters", input: "Ada\nLovelace", required: true, ok: false},
		{name: "reject invalid UTF-8", input: string([]byte{0xff}), required: true, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeSignupName(tt.input, tt.required)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("normalizeSignupName(%q, %t) = (%q, %t), want (%q, %t)", tt.input, tt.required, got, ok, tt.want, tt.ok)
			}
		})
	}
}
