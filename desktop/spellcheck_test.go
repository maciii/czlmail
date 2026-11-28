package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetWebviewSpellcheckKeepsOtherPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Default", "Preferences")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	orig := `{"profile":{"exit_type":"Normal"},"intl":{"selected_languages":"en-US,en"},"spellcheck":{"dictionaries":["en-US"]},"ts":13379999999999999}`
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := setWebviewSpellcheck(path, "cs"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var got struct {
		Profile    map[string]string   `json:"profile"`
		Intl       map[string]string   `json:"intl"`
		Spellcheck map[string][]string `json:"spellcheck"`
		Browser    map[string]bool     `json:"browser"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Intl["accept_languages"] != "cs" || got.Intl["selected_languages"] != "" {
		t.Errorf("intl = %v", got.Intl)
	}
	if d := got.Spellcheck["dictionaries"]; len(d) != 1 || d[0] != "cs" {
		t.Errorf("dictionaries = %v", d)
	}
	if !got.Browser["enable_spellchecking"] || got.Profile["exit_type"] != "Normal" {
		t.Errorf("browser = %v, profile = %v", got.Browser, got.Profile)
	}
	if !strings.Contains(string(data), "13379999999999999") {
		t.Errorf("large number changed: %s", data)
	}

	// Back to following Windows: the language keys go, WebView2 fills them in again.
	if err := setWebviewSpellcheck(path, ""); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "accept_languages") || strings.Contains(string(data), "dictionaries") {
		t.Errorf("language keys left: %s", data)
	}
}

func TestSetWebviewSpellcheckWithoutProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "EBWebView", "Default", "Preferences")
	if err := setWebviewSpellcheck(path, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("created a profile for the default language")
	}
	if err := setWebviewSpellcheck(path, "de"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), `"accept_languages":"de"`) {
		t.Errorf("preferences = %s", data)
	}
}

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
