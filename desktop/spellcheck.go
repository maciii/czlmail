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
}

// SpellcheckSettings 是设置界面看到的拼写检查语言。
type SpellcheckSettings struct {
	// Supported 只在 Windows 上为真; 其他平台的 WebView 用系统自己的拼写检查设置。
	Supported bool   `json:"supported"`
	Language  string `json:"language"`
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
	return SpellcheckSettings{Supported: runtime.GOOS == "windows", Language: loadSpellcheckState().Language}
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
