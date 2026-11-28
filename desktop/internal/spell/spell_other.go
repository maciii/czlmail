//go:build !windows

package spell

import "errors"

// 其他平台沿用 WebView 自带的拼写检查。

var errUnsupported = errors.New("spell checking through the system API is only available on Windows")

func Supported() bool { return false }

func Languages() ([]string, error) { return nil, errUnsupported }

func Check(string, []string) ([]Error, error) { return nil, errUnsupported }

func Suggest(string, []string, int) ([]string, error) { return nil, errUnsupported }

func Add(string, []string) error { return errUnsupported }

func Ignore(string, []string) error { return errUnsupported }
