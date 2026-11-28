//go:build windows

package main

import _ "embed"

// notifyIcon 是 Windows 通知上显示的应用图标。
//
//go:embed build/appicon.png
var notifyIcon []byte
