//go:build windows

package spell

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Windows Spell Checking API (spellcheck.h, Windows 8 and later) through plain COM calls.
// Every call runs on one locked OS thread with COM initialised (see run).

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")

	clsidSpellCheckerFactory = windows.GUID{Data1: 0x7AB36653, Data2: 0x1796, Data3: 0x484B, Data4: [8]byte{0xBD, 0xFA, 0xE7, 0x4F, 0x1D, 0xB7, 0xC1, 0xDC}}
	iidISpellCheckerFactory  = windows.GUID{Data1: 0x8E018A9D, Data2: 0x2415, Data3: 0x4677, Data4: [8]byte{0xBF, 0x08, 0x79, 0x4E, 0xA6, 0x1F, 0x94, 0xBB}}
)

const (
	clsctxInprocServer = 0x1
	sFalse             = 1
)

// Vtable slots after IUnknown (QueryInterface 0, AddRef 1, Release 2).
const (
	factorySupportedLanguages = 3
	factoryCreateSpellChecker = 5

	checkerCheck   = 4
	checkerSuggest = 5
	checkerAdd     = 6
	checkerIgnore  = 7

	enumNext = 3 // IEnumSpellingError::Next and IEnumString::Next

	errorStartIndex       = 3
	errorLength           = 4
	errorCorrectiveAction = 5
	errorReplacement      = 6
)

type comObject struct{ vtbl *[32]uintptr }

func (o *comObject) call(slot int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[slot], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}

func (o *comObject) release() {
	if o != nil {
		o.call(2)
	}
}

func failed(hr uintptr) bool { return int32(hr) < 0 }

func hrError(op string, hr uintptr) error {
	return fmt.Errorf("%s: HRESULT 0x%08X", op, uint32(hr))
}

func utf16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", " "))
	if err != nil {
		p, _ = windows.UTF16PtrFromString("")
	}
	return p
}

// takeString copies a string the API allocated and frees it.
func takeString(p *uint16) string {
	if p == nil {
		return ""
	}
	s := windows.UTF16PtrToString(p)
	windows.CoTaskMemFree(unsafe.Pointer(p))
	return s
}

// readEnumString reads and releases an IEnumString.
func readEnumString(enum *comObject, limit int) []string {
	defer enum.release()
	var out []string
	for limit <= 0 || len(out) < limit {
		var p *uint16
		var fetched uint32
		hr := enum.call(enumNext, 1, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&fetched)))
		if failed(hr) || hr == sFalse || fetched == 0 {
			break
		}
		out = append(out, takeString(p))
	}
	return out
}

type engine struct {
	factory  *comObject
	checkers map[string]*comObject
	err      error
}

var (
	startOnce sync.Once
	jobs      = make(chan func(*engine))
)

// run executes f on the COM thread. The checkers are created there and used only there.
func run(f func(*engine)) {
	startOnce.Do(func() {
		ready := make(chan struct{})
		go func() {
			runtime.LockOSThread()
			e := &engine{checkers: map[string]*comObject{}}
			if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil && !errors.Is(err, windows.Errno(sFalse)) {
				e.err = fmt.Errorf("CoInitializeEx: %w", err)
			} else if hr, _, _ := procCoCreateInstance.Call(
				uintptr(unsafe.Pointer(&clsidSpellCheckerFactory)), 0, clsctxInprocServer,
				uintptr(unsafe.Pointer(&iidISpellCheckerFactory)), uintptr(unsafe.Pointer(&e.factory)),
			); failed(hr) {
				e.err = hrError("create spell checker factory", hr)
			}
			close(ready)
			for job := range jobs {
				job(e)
			}
		}()
		<-ready
	})
	done := make(chan struct{})
	jobs <- func(e *engine) {
		defer close(done)
		f(e)
	}
	<-done
}

// checker returns the spell checker for a language, nil when Windows has none for it.
func (e *engine) checker(lang string) *comObject {
	if c, ok := e.checkers[lang]; ok {
		return c
	}
	var c *comObject
	if hr := e.factory.call(factoryCreateSpellChecker, uintptr(unsafe.Pointer(utf16(lang))), uintptr(unsafe.Pointer(&c))); failed(hr) {
		c = nil
	}
	e.checkers[lang] = c
	return c
}

func Supported() bool {
	ok := false
	run(func(e *engine) { ok = e.err == nil })
	return ok
}

// Languages lists the languages Windows can check, as BCP 47 tags ("cs-CZ", "en-US").
func Languages() ([]string, error) {
	var out []string
	var err error
	run(func(e *engine) {
		if err = e.err; err != nil {
			return
		}
		var enum *comObject
		if hr := e.factory.call(factorySupportedLanguages, uintptr(unsafe.Pointer(&enum))); failed(hr) {
			err = hrError("supported languages", hr)
			return
		}
		out = readEnumString(enum, 0)
	})
	return out, err
}

func (e *engine) check(c *comObject, text string) ([]Error, error) {
	var enum *comObject
	if hr := c.call(checkerCheck, uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(&enum))); failed(hr) {
		return nil, hrError("check", hr)
	}
	defer enum.release()
	var out []Error
	for {
		var se *comObject
		hr := enum.call(enumNext, uintptr(unsafe.Pointer(&se)))
		if failed(hr) || hr == sFalse || se == nil {
			break
		}
		var start, length, action uint32
		var repl *uint16
		se.call(errorStartIndex, uintptr(unsafe.Pointer(&start)))
		se.call(errorLength, uintptr(unsafe.Pointer(&length)))
		se.call(errorCorrectiveAction, uintptr(unsafe.Pointer(&action)))
		se.call(errorReplacement, uintptr(unsafe.Pointer(&repl)))
		se.release()
		out = append(out, Error{Start: int(start), Length: int(length), Action: Action(action), Replacement: takeString(repl)})
	}
	return out, nil
}

// Check returns the ranges of text that none of the languages accepts.
// Languages Windows cannot check are skipped; with none left nothing is reported.
func Check(text string, langs []string) ([]Error, error) {
	var out []Error
	var err error
	run(func(e *engine) {
		if err = e.err; err != nil {
			return
		}
		var perLang [][]Error
		for _, lang := range langs {
			c := e.checker(lang)
			if c == nil {
				continue
			}
			errs, cerr := e.check(c, text)
			if cerr != nil {
				err = cerr
				return
			}
			perLang = append(perLang, errs)
		}
		out = Merge(perLang)
	})
	return out, err
}

// Suggest returns corrections for a word from all the languages, best first.
func Suggest(word string, langs []string, limit int) ([]string, error) {
	var out []string
	var err error
	run(func(e *engine) {
		if err = e.err; err != nil {
			return
		}
		var perLang [][]string
		for _, lang := range langs {
			c := e.checker(lang)
			if c == nil {
				continue
			}
			var enum *comObject
			if hr := c.call(checkerSuggest, uintptr(unsafe.Pointer(utf16(word))), uintptr(unsafe.Pointer(&enum))); failed(hr) || enum == nil {
				continue // S_FALSE with no list: the word is correct in this language
			}
			perLang = append(perLang, readEnumString(enum, limit))
		}
		out = MergeSuggestions(perLang, limit)
	})
	return out, err
}

// Add puts the word into the user's Windows dictionary of each language (shared with other apps).
func Add(word string, langs []string) error {
	return eachChecker(langs, func(c *comObject) uintptr {
		return c.call(checkerAdd, uintptr(unsafe.Pointer(utf16(word))))
	})
}

// Ignore accepts the word until the app exits.
func Ignore(word string, langs []string) error {
	return eachChecker(langs, func(c *comObject) uintptr {
		return c.call(checkerIgnore, uintptr(unsafe.Pointer(utf16(word))))
	})
}

func eachChecker(langs []string, f func(*comObject) uintptr) error {
	var err error
	run(func(e *engine) {
		if err = e.err; err != nil {
			return
		}
		for _, lang := range langs {
			if c := e.checker(lang); c != nil {
				if hr := f(c); failed(hr) {
					err = hrError("update dictionary", hr)
				}
			}
		}
	})
	return err
}
