//go:build ios || android

package notify

import (
	"context"
	"errors"
	"sync"
)

// iOS 的本地通知由原生壳发 (UNUserNotificationCenter): Go 这边只把通知交出去,
// 用户点开时原生壳再把 ActionID 送回来 (Activated)。

var native struct {
	mu        sync.Mutex
	post      func(Notification) error
	activated func(string)
}

// SetPoster 由 iOS 入口在启动时注册发送通知的原生实现。
func SetPoster(fn func(Notification) error) {
	native.mu.Lock()
	native.post = fn
	native.mu.Unlock()
}

// Activated 由 iOS 入口在用户点开通知时调用。
func Activated(actionID string) {
	native.mu.Lock()
	fn := native.activated
	native.mu.Unlock()
	if fn != nil {
		fn(actionID)
	}
}

type iosNotifier struct{}

func New(context.Context, App) (Notifier, error) { return iosNotifier{}, nil }

func (iosNotifier) Notify(_ context.Context, n Notification) error {
	native.mu.Lock()
	post := native.post
	native.mu.Unlock()
	if post == nil {
		return errors.New("native notifier not registered")
	}
	return post(n)
}

func (iosNotifier) OnActivated(fn func(string)) {
	native.mu.Lock()
	native.activated = fn
	native.mu.Unlock()
}

func (iosNotifier) Close() error { return nil }

// Unregister 只在 Windows 上有事可做 (通知登记在注册表里)。
func Unregister(string) error { return nil }
