package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/woodchen-ink/czlmail/desktop/internal/config"
	"github.com/woodchen-ink/czlmail/desktop/internal/spell"
)

// 拼写检查的语言。
//
// Windows 上 WebView2 自带的拼写检查只认一种语言 (跟着 Windows 显示语言), 也没有接口改它。
// 用户选了语言时, 编辑器关掉 WebView2 的检查, 改由 Windows 拼写检查 API 按所选的几种语言检查 (internal/spell),
// 一个词在任一语言里正确就不标; 不选时照旧用 WebView2 自带的检查。
// 不去改 WebView2 的 Preferences (intl.accept_languages): 它同时决定 navigator.languages, 会带偏界面的「跟随系统语言」。
//
// 选择存在数据目录的 spellcheck.json 而不是缓存库: 与窗口位置一样是这台机器的事。

const spellcheckFile = "spellcheck.json"

type spellcheckState struct {
	// Languages 是自己检查拼写用的语言 (Windows 拼写检查 API, 见 internal/spell);
	// 非空时编辑器关掉 WebView2 的检查, 一个词在任一语言里正确就不标。
	Languages []string `json:"languages,omitempty"`
}

// SpellcheckSettings 是设置界面看到的拼写检查语言。
type SpellcheckSettings struct {
	// Supported 只在 Windows 上为真; 其他平台的 WebView 用系统自己的拼写检查设置。
	Supported bool `json:"supported"`
	// Languages 是多语言检查选中的语言, Available 是 Windows 能检查的全部语言 (BCP 47)。
	Languages []string `json:"languages"`
	Available []string `json:"available"`
}

func spellcheckStatePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, spellcheckFile), nil
}

func loadSpellcheckState() spellcheckState {
	var s spellcheckState
	path, err := spellcheckStatePath()
	if err != nil {
		return s
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

func saveSpellcheckState(s spellcheckState) error {
	path, err := spellcheckStatePath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// validSpellcheckLanguage 接受 BCP 47 形式的语言标记 ("cs"、"en-US"), 空串表示跟随 Windows。
func validSpellcheckLanguage(lang string) bool {
	if len(lang) > 16 {
		return false
	}
	for _, r := range lang {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-') {
			return false
		}
	}
	return true
}

func (a *App) GetSpellcheckSettings() SpellcheckSettings {
	st := loadSpellcheckState()
	out := SpellcheckSettings{Supported: runtime.GOOS == "windows", Languages: []string{}, Available: []string{}}
	if !out.Supported {
		return out
	}
	if langs, err := spell.Languages(); err == nil {
		out.Available = langs
		out.Languages = append(out.Languages, st.Languages...)
	} else {
		a.log.Warn("spell checker languages", "err", err)
	}
	return out
}

// SetSpellcheckLanguages 选定多语言检查的语言; 空列表回到 WebView2 自带的单语言检查。
func (a *App) SetSpellcheckLanguages(langs []string) error {
	clean := []string{}
	for _, l := range langs {
		if l = strings.TrimSpace(l); l == "" || !validSpellcheckLanguage(l) {
			return errors.New("invalid spellcheck language")
		}
		clean = append(clean, l)
	}
	s := loadSpellcheckState()
	s.Languages = clean
	if err := saveSpellcheckState(s); err != nil {
		return err
	}
	a.emit(EventSpellcheckChanged)
	return nil
}

// SpellCheckItem 是编辑器送来检查的一段文字; Langs 为空时用设置里的语言。
type SpellCheckItem struct {
	Text  string   `json:"text"`
	Langs []string `json:"langs"`
}

// spellLangs 是一段文字要用的语言: 编辑器标了语言的用标的, 否则用设置里的。
func spellLangs(langs []string) []string {
	if len(langs) > 0 {
		return langs
	}
	return loadSpellcheckState().Languages
}

// CheckSpelling 检查编辑器里的若干段文字, 返回每段里没有一种语言认可的范围 (UTF-16 下标)。
func (a *App) CheckSpelling(items []SpellCheckItem) ([][]spell.Error, error) {
	out := make([][]spell.Error, len(items))
	for i, it := range items {
		errs, err := spell.Check(it.Text, spellLangs(it.Langs))
		if err != nil {
			return nil, err
		}
		if errs == nil {
			errs = []spell.Error{}
		}
		out[i] = errs
	}
	return out, nil
}

// SpellSuggestions 是一个词在这些语言里的改法, 最多 8 个。
func (a *App) SpellSuggestions(word string, langs []string) ([]string, error) {
	out, err := spell.Suggest(word, spellLangs(langs), 8)
	if out == nil {
		out = []string{}
	}
	return out, err
}

// SpellAddWord 把词加进 Windows 的用户词典 (每种语言各一份, 其他程序也用)。
func (a *App) SpellAddWord(word string, langs []string) error {
	return spell.Add(word, spellLangs(langs))
}

// SpellIgnoreWord 在本次运行里不再标这个词。
func (a *App) SpellIgnoreWord(word string, langs []string) error {
	return spell.Ignore(word, spellLangs(langs))
}
