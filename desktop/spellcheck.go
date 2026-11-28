package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/woodchen-ink/czlmail/desktop/internal/config"
	"github.com/woodchen-ink/czlmail/desktop/internal/spell"
)

// 拼写检查的语言。
//
// Windows 上的 WebView2 只用一种拼写检查语言: 启动时取 WebView2 配置里 intl.accept_languages 的第一个,
// 默认跟着 Windows 的显示语言。显示语言是英文、写的却是捷克语或德语时, 每个词都会被标成错的。
// WebView2 没有设置拼写语言的接口, Wails 也不能传浏览器参数 (--lang), 所以在 wails.Run 之前
// 把选定的语言写进 WebView2 的 Preferences 文件; 运行中改不了, 下次启动生效。
//
// 选择存在数据目录的 spellcheck.json 而不是缓存库: 与窗口位置一样是这台机器的事, 程序启动时缓存库还没打开。

const spellcheckFile = "spellcheck.json"

type spellcheckState struct {
	// Language 是用户选的语言 (如 "cs"、"en-US"), 空串表示跟随 Windows。
	Language string `json:"language,omitempty"`
	// Applied 是上次写进 WebView2 配置的语言, 改回「跟随 Windows」时据此把它去掉。
	Applied string `json:"applied,omitempty"`
	// Languages 是自己检查拼写用的语言 (Windows 拼写检查 API, 见 internal/spell);
	// 非空时编辑器关掉 WebView2 的检查, 一个词在任一语言里正确就不标。
	Languages []string `json:"languages,omitempty"`
}

// SpellcheckSettings 是设置界面看到的拼写检查语言。
type SpellcheckSettings struct {
	// Supported 只在 Windows 上为真; 其他平台的 WebView 用系统自己的拼写检查设置。
	Supported bool   `json:"supported"`
	Language  string `json:"language"`
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
	out := SpellcheckSettings{Supported: runtime.GOOS == "windows", Language: st.Language, Languages: []string{}, Available: []string{}}
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

// SetSpellcheckLanguage 记下选择, 下次启动时写进 WebView2 的配置。
func (a *App) SetSpellcheckLanguage(lang string) error {
	lang = strings.TrimSpace(lang)
	if !validSpellcheckLanguage(lang) {
		return errors.New("invalid spellcheck language")
	}
	s := loadSpellcheckState()
	s.Language = lang
	return saveSpellcheckState(s)
}

// webviewPreferencesPath 是 WebView2 默认用户数据目录里的 Preferences:
// Wails 没设 WebviewUserDataPath 时, go-webview2 用 %APPDATA%\<exe 文件名>。
func webviewPreferencesPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", errors.New("APPDATA not set")
	}
	return filepath.Join(appData, filepath.Base(exe), "EBWebView", "Default", "Preferences"), nil
}

// applySpellcheckLanguage 在 wails.Run 之前调用 (WebView2 运行时会覆盖这个文件)。
func applySpellcheckLanguage(log *slog.Logger) {
	if runtime.GOOS != "windows" {
		return
	}
	s := loadSpellcheckState()
	if s.Language == "" && s.Applied == "" {
		return // 从没选过: 不碰 WebView2 的配置。
	}
	path, err := webviewPreferencesPath()
	if err != nil {
		log.Warn("spellcheck preferences path", "err", err)
		return
	}
	if err := setWebviewSpellcheck(path, s.Language); err != nil {
		log.Warn("apply spellcheck language", "language", s.Language, "err", err)
		return
	}
	if s.Applied != s.Language {
		s.Applied = s.Language
		if err := saveSpellcheckState(s); err != nil {
			log.Warn("save spellcheck state", "err", err)
		}
	}
}

// setWebviewSpellcheck 改写 Preferences 里与拼写语言有关的几项, 其余原样保留。
// lang 为空时去掉这几项, WebView2 下次启动重新按 Windows 显示语言填。
func setWebviewSpellcheck(path, lang string) error {
	prefs := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber() // 保留大整数 (时间戳) 原样。
		if err := dec.Decode(&prefs); err != nil {
			return err
		}
	case errors.Is(err, fs.ErrNotExist):
		if lang == "" {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
	default:
		return err
	}
	intl := jsonObject(prefs, "intl")
	spellcheck := jsonObject(prefs, "spellcheck")
	if lang == "" {
		delete(intl, "accept_languages")
		delete(intl, "selected_languages")
		delete(spellcheck, "dictionaries")
	} else {
		// WebView2 只认第一个语言做拼写检查, 并按它重填 spellcheck.dictionaries。
		intl["accept_languages"] = lang
		delete(intl, "selected_languages")
		spellcheck["dictionaries"] = []string{lang}
		jsonObject(prefs, "browser")["enable_spellchecking"] = true
	}
	out, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	tmp := path + ".czlmail-tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// jsonObject 返回 m[key] 这个 JSON 对象, 没有 (或不是对象) 时新建。
func jsonObject(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	v := map[string]any{}
	m[key] = v
	return v
}
