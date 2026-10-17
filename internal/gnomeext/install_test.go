// SPDX-License-Identifier: GPL-3.0-or-later

package gnomeext

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestStrv(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"@as []\n", nil},
		{"[]", nil},
		{"['a@b.c']\n", []string{"a@b.c"}},
		{"['a@b.c', 'appindicatorsupport@rgcjonas.gmail.com']", []string{"a@b.c", "appindicatorsupport@rgcjonas.gmail.com"}},
	}
	for _, tt := range tests {
		got, err := parseStrv(tt.in)
		if err != nil || !slices.Equal(got, tt.want) {
			t.Errorf("parseStrv(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
		if back, _ := parseStrv(formatStrv(got)); !slices.Equal(back, got) {
			t.Errorf("round trip of %q gave %q", got, back)
		}
	}
	for _, bad := range []string{"", "nope", "[a]", "['a', b]"} {
		if _, err := parseStrv(bad); err == nil {
			t.Errorf("parseStrv(%q) should fail", bad)
		}
	}
}

// The Go side and extension.js must agree on the D-Bus coordinates.
func TestExtensionMatchesConstants(t *testing.T) {
	js, err := os.ReadFile("extension.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{`'` + ObjectPath + `'`, `"` + Interface + `"`, `name="FocusedWindow"`} {
		if !strings.Contains(string(js), s) {
			t.Errorf("extension.js does not contain %s", s)
		}
	}

	var meta struct{ UUID string }
	data, _ := files.ReadFile("metadata.json")
	if err := json.Unmarshal(data, &meta); err != nil || meta.UUID != UUID {
		t.Errorf("metadata.json uuid = %q (%v), want %q", meta.UUID, err, UUID)
	}
}
