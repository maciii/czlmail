//go:build windows

package notify

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// 「删除所有数据」要把通知登记 (显示名、唤起路径) 从注册表里删干净。用测试专用的 ID, 不碰正式版的登记。
func TestUnregisterRemovesRegistration(t *testing.T) {
	const id = "net.czl.mail.go-test-unregister"
	if err := registerApp(App{ID: id, Name: "test"}, activatorGUID(id), `C:\nowhere\czlmail.exe`); err != nil {
		t.Fatal(err)
	}
	if err := Unregister(id); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{appModelKey + id, `Software\Classes\CLSID\` + activatorGUID(id)} {
		k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.READ)
		if err == nil {
			k.Close()
			t.Errorf("%s still exists", path)
		} else if !errors.Is(err, registry.ErrNotExist) {
			t.Errorf("%s: %v", path, err)
		}
	}
	// 再删一次: 不存在不算错。
	if err := Unregister(id); err != nil {
		t.Errorf("second unregister: %v", err)
	}
}
