//go:build !windows

package spell

import "errors"

// Other platforms keep the WebView's own spell checking.

var errUnsupported = errors.New("spell checking through the system API is only available on Windows")

func Supported() bool { return false }

func Languages() ([]string, error) { return nil, errUnsupported }

func Check(string, []string) ([]Error, error) { return nil, errUnsupported }

func Suggest(string, []string, int) ([]string, error) { return nil, errUnsupported }

func Add(string, []string) error { return errUnsupported }

func Ignore(string, []string) error { return errUnsupported }
