// Package notify 发送系统原生通知。
//
// 不使用跨平台通知库: 它们在 Windows 上通常是拼一段 PowerShell 去调 toast,
// 既慢又拿不到点击回调。而邮件通知点开必须能跳到那封信, 回调是硬需求,
// 因此两个平台各写各的原生实现。
package notify

import "context"

// Notification 是一条待发送的通知。
type Notification struct {
	Title string
	Body  string
	// ActionID 在用户点击通知时原样回传, 用于定位到具体邮件。
	ActionID string
}

// App 描述通知归属的应用。目前只有 Windows 用到: 操作中心里显示的名字与图标取自这里,
// 不再是 AppUserModelID 本身 (net.czl.mail)。
type App struct {
	// ID 是 AppUserModelID。测试实例 (CZLMAIL_INSTANCE) 用自己的 ID,
	// 通知、点击唤起与系统里的通知设置都与正式版分开。
	ID string
	// Name 是通知上显示的应用名。
	Name string
	// Icon 是 PNG 格式的应用图标, 可为空。
	Icon []byte
}

// Notifier 发送通知。各平台的实现在带构建标签的文件里。
type Notifier interface {
	// Notify 发送一条通知。发送失败不应视为致命错误 ——
	// 用户可能在系统设置里关掉了通知, 那是他的选择。
	Notify(ctx context.Context, n Notification) error

	// OnActivated 注册点击回调。同一时刻只有一个回调生效。
	OnActivated(fn func(actionID string))

	// Close 释放平台资源。
	Close() error
}
