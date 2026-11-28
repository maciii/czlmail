package notify

import (
	"crypto/sha1"
	"fmt"
)

// defaultAppID 是正式版的 AppUserModelID。
const defaultAppID = "net.czl.mail"

// defaultActivatorGUID 是正式版用于把 COM 激活回调路由回本进程的类 ID。
//
// 必须是一个固定不变的值: Windows 会把它写进注册表, 每次构建换一个新 GUID 会
// 在系统里堆下一串死条目, 且旧通知的点击将无处可去。
const defaultActivatorGUID = "{8F3A2C41-6D5E-4B7A-9C18-2E4D7A9B0C31}"

// activatorGUID 给每个 AppUserModelID 一个固定的类 ID。
//
// 测试实例若与正式版共用同一个类 ID, 注册表里的唤起路径会被先启动的那个占住:
// 点测试实例的通知可能拉起正式版, 反之亦然。由 ID 推导 (RFC 4122 第 5 版的格式)
// 保证同一实例每次都得到同一个值, 不会在注册表里堆积死条目。
func activatorGUID(appID string) string {
	if appID == defaultAppID {
		return defaultActivatorGUID
	}
	sum := sha1.Sum([]byte(defaultActivatorGUID + appID))
	sum[6] = sum[6]&0x0f | 0x50
	sum[8] = sum[8]&0x3f | 0x80
	return fmt.Sprintf("{%X-%X-%X-%X-%X}", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
