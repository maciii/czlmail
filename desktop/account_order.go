package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"slices"

	"github.com/woodchen-ink/czlmail/desktop/internal/accountid"
	"github.com/woodchen-ink/czlmail/desktop/internal/store"
)

// 账号与登录的先后 (设置 → 账户 → 可访问的邮箱里拖动): ListAccounts 按它排, 侧栏、「全部账号」、写信的发件身份跟着变。
// 存成一条同步的账户设置, 值是同步键 (见 account_settings_sync.go) 的 JSON 数组: 登录那一节用登录的同步键, 账号用账号的;
// 与名字、颜色一起经个人网盘的 account-settings.json 同步, 整条"最后修改者胜"。空值 (墓碑) 是默认顺序。
// 列表里没有的登录 (后来加的) 排在列出的后面, 没有的账号同理; 登录里个人账号始终在共享账号前面。

const (
	// accountOrderPrefix 是本机 app_settings 里账号先后的键前缀; 只有一条, 后接 accountOrderName。
	accountOrderPrefix = "accountOrder:"
	accountOrderName   = "list"
	accountOrderKey    = accountOrderPrefix + accountOrderName
	// maxAccountOrder 限制列表长度; 别的设备上的键要留着, 已删掉的账号的键也会留下, 不让它无限长。
	maxAccountOrder = 200
)

func encodeAccountOrder(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	b, _ := json.Marshal(keys)
	return string(b)
}

func decodeAccountOrder(v string) []string {
	var keys []string
	if v != "" {
		_ = json.Unmarshal([]byte(v), &keys)
	}
	return keys
}

// storedAccountOrder 读本机存的先后 (同步键); 没设或读不到时为空。
func (a *App) storedAccountOrder(st *store.Store) []string {
	v, err := st.StringSetting(a.ctx, accountOrderKey)
	if err != nil {
		return nil
	}
	return decodeAccountOrder(v)
}

// SetAccountOrder 保存账号与登录的先后。keys 是界面的本机键, 与 SetAccountLook 一样: 账号 id,
// 登录那一节是 "signin:" + 登录前缀 (主登录 "signin:")。空列表恢复默认顺序。
// 已存的列表里本机不认识的键 (别的设备上的登录) 接在后面留着。
func (a *App) SetAccountOrder(keys []string) error {
	st, err := a.currentStore()
	if err != nil {
		return err
	}
	var out []string
	if len(keys) > 0 {
		seen := map[string]bool{}
		for _, k := range keys {
			syncKey, err := a.accountLookSyncKey(k)
			if err != nil {
				return err
			}
			if !seen[syncKey] {
				seen[syncKey] = true
				out = append(out, syncKey)
			}
		}
		for _, k := range a.storedAccountOrder(st) {
			if !seen[k] && len(out) < maxAccountOrder {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	value := encodeAccountOrder(out)
	cur, err := st.StringSetting(a.ctx, accountOrderKey)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if cur == value {
		return nil
	}
	if err := a.putAccountSetting(st, accountOrderKey, value); err != nil {
		return err
	}
	a.emit(EventAccountsChanged)
	return nil
}

// sortAccounts 排 ListAccounts 的结果: 先按登录 (用户排过的在前, 其余按 defaultSignIn 的添加顺序),
// 登录里个人账号在前, 再按用户排的先后, 其余保留 list 原来的顺序 (库里的「默认邮箱在前、按名称」)。
// order 是同步键的先后; signIns 是本机的登录 (键是登录前缀), 不在其中的登录 (已移除、缓存还没删完) 排最后。
func sortAccounts(list []store.Account, order []string, signIns map[string]lookSignIn, defaultSignIn map[string]int) {
	pos := make(map[string]int, len(order))
	for i, k := range order {
		if _, ok := pos[k]; !ok {
			pos[k] = i
		}
	}
	rank := func(key string, ok bool, fallback int) int {
		if i, found := pos[key]; ok && found {
			return i
		}
		return len(pos) + fallback
	}
	type ranked struct {
		acc              store.Account
		signIn, inSignIn int
		personal         bool
	}
	rs := make([]ranked, len(list))
	for i, acc := range list {
		prefix, serverID := accountid.Split(acc.ID)
		si, known := signIns[prefix]
		def, ok := defaultSignIn[prefix]
		if !ok {
			def = len(defaultSignIn)
		}
		rs[i] = ranked{
			acc:      acc,
			signIn:   rank(si.syncKey("", true), known, def),
			inSignIn: rank(si.syncKey(serverID, false), known, i),
			personal: acc.IsPersonal,
		}
	}
	slices.SortStableFunc(rs, func(x, y ranked) int {
		personal := func(r ranked) int {
			if r.personal {
				return 0
			}
			return 1
		}
		return cmp.Or(cmp.Compare(x.signIn, y.signIn), cmp.Compare(personal(x), personal(y)), cmp.Compare(x.inSignIn, y.inSignIn))
	})
	for i, r := range rs {
		list[i] = r.acc
	}
}
