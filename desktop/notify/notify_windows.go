//go:build windows

package notify

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	toast "git.sr.ht/~jackmordaunt/go-toast/v2"
	"github.com/woodchen-ink/czlmail/desktop/internal/config"
	"golang.org/x/sys/windows/registry"
)

// AppUserModelID 决定通知归属哪个应用、点击时唤起谁; 操作中心里显示的名字与图标
// 取自 HKCU\Software\Classes\AppUserModelId\<ID> 下的 DisplayName 与 IconUri。
//
// go-toast 把 DisplayName 写成 ID 本身, 且已有的值不再覆盖, 于是通知上显示的是
// "net.czl.mail"。这里在交给 go-toast 之前自己写好名字、图标与唤起路径:
// 每次启动都写, 程序换了位置或升级后的旧记录也随之更正。
const appModelKey = `Software\Classes\AppUserModelId\`

type windowsNotifier struct {
	appID    string
	mu       sync.Mutex
	onAction func(string)
}

// New 构造平台通知器并完成注册。
//
// Windows 上不注册 AppUserModelID 的话 toast 根本不会显示 —— 不是报错,
// 是静默地什么都不发生, 这是最容易踩进去的坑。
func New(_ context.Context, app App) (Notifier, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("4001 locate executable: %w", err)
	}
	if app.ID == "" {
		app.ID = defaultAppID
	}
	guid := activatorGUID(app.ID)

	if err := registerApp(app, guid, exe); err != nil {
		return nil, fmt.Errorf("4004 register notification app: %w", err)
	}

	if err := toast.SetAppData(toast.AppData{
		AppID: app.ID,
		GUID:  guid,
		// 应用没在运行时, Windows 靠这个路径把它拉起来处理点击。
		ActivationExe: exe,
	}); err != nil {
		return nil, fmt.Errorf("4002 register notification app id: %w", err)
	}

	n := &windowsNotifier{appID: app.ID}
	toast.SetActivationCallback(func(args string, _ []toast.UserData) {
		n.mu.Lock()
		fn := n.onAction
		n.mu.Unlock()
		if fn != nil && args != "" {
			fn(args)
		}
	})
	return n, nil
}

// registerApp 写入通知上显示的名字、图标, 以及点击通知时唤起的程序。
func registerApp(app App, guid, exe string) error {
	values := map[string]string{"DisplayName": app.Name}
	if app.Name == "" {
		values["DisplayName"] = app.ID
	}
	// 图标写不出来只是少个图标, 不影响通知本身。
	if icon, err := writeIcon(app.Icon); err == nil && icon != "" {
		values["IconUri"] = icon
	}
	if err := setValues(appModelKey+app.ID, values); err != nil {
		return err
	}
	return setValues(`Software\Classes\CLSID\`+guid+`\LocalServer32`, map[string]string{"": exe})
}

// writeIcon 把图标存进数据目录: IconUri 只接受磁盘上的文件。内容没变时不重写。
func writeIcon(png []byte) (string, error) {
	if len(png) == 0 {
		return "", nil
	}
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "notification-icon.png")
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, png) {
		return path, nil
	}
	if err := os.WriteFile(path, png, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func setValues(path string, values map[string]string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	for name, v := range values {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	return nil
}

func (n *windowsNotifier) Notify(_ context.Context, msg Notification) error {
	notification := toast.Notification{
		AppID:               n.appID,
		Title:               msg.Title,
		Body:                msg.Body,
		ActivationType:      toast.Foreground,
		ActivationArguments: msg.ActionID,
		Duration:            toast.Short,
	}
	if err := notification.Push(); err != nil {
		return fmt.Errorf("4003 push notification: %w", err)
	}
	return nil
}

func (n *windowsNotifier) OnActivated(fn func(string)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onAction = fn
}

func (n *windowsNotifier) Close() error { return nil }
