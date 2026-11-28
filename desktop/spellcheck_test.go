package main

import (
	"strings"
	"testing"
)

func TestValidSpellcheckLanguage(t *testing.T) {
	for _, ok := range []string{"", "cs", "en-US"} {
		if !validSpellcheckLanguage(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"cs,en", "../x", "cs\n", strings.Repeat("a", 20)} {
		if validSpellcheckLanguage(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
