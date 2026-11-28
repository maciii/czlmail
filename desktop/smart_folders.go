package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/woodchen-ink/czlmail/desktop/internal/accountid"
	"github.com/woodchen-ink/czlmail/desktop/internal/store"
)

// 筛选文件夹 (同 Outlook 的搜索文件夹、Apple Mail 的智能邮箱): 用户自己组合条件 —— 未读、待办旗标、标签、
// 有附件、发件人、账号、在哪些文件夹里找 —— 侧栏「全部账号」里排在「未读」下面, 点开是跨账号合流的列表。
// 与「未读」视图一样只看本地缓存, 不向服务器补取。
//
// 定义跨设备同步与标签相同: 个人网盘的 CZL Mail/smart-folders.json, 本机每条一行 app_settings (smartFolder:<id>),
// 逐条"最后修改者胜", 删除写空值 (墓碑)。账号条件存同步键 (会话地址 + 服务器上的账号 id, 见 account_settings_sync.go),
// 界面用本机账号 id; 本机没有的登录的账号原样保留, 不因在这台设备上改了别的条件而丢掉。

const (
	smartFoldersFileName = "smart-folders.json"
	smartFoldersFileType = "czlmail-smart-folders"

	// smartFolderPrefix 是本机 app_settings 里筛选文件夹的键前缀, 后接 id。
	smartFolderPrefix = "smartFolder:"
	// smartViewPrefix 是统一视图里筛选文件夹的 view: "smart:" + id。
	smartViewPrefix = "smart:"
)

// 筛选文件夹在哪些文件夹里找。
const (
	// smartScopeDefault 同「未读」视图: 不含垃圾邮件、已删除、已发送、草稿与定时发送。
	smartScopeDefault = ""
	smartScopeInbox   = "inbox"
	// smartScopeAll 是除垃圾邮件与已删除以外的全部文件夹。
	smartScopeAll = "all"
)

// SmartFolder 是一个筛选文件夹, 各条件同时满足; 没设的条件不筛选。
type SmartFolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Unread 只列未读。
	Unread bool `json:"unread"`
	// Flag 是待办旗标: "flagged" 待办, "completed" 已完成, 空串不限。
	Flag string `json:"flag"`
	// Tags 是标签的关键字, 邮件有其中任意一个即可。
	Tags []string `json:"tags"`
	// Attachment 只列有附件的。
	Attachment bool `json:"attachment"`
	// From 是发件人名字或地址里要包含的文字。
	From string `json:"from"`
	// Accounts 是本机账号 id; 与 AccountsElsewhere 都为空表示并入「全部账号」的所有账号。
	Accounts []string `json:"accounts"`
	// AccountsElsewhere 是选了的、但不在这台设备上的登录里的账号数 (保存时原样保留)。
	AccountsElsewhere int `json:"accountsElsewhere"`
	// Scope 是在哪些文件夹里找: "" 同「未读」, "inbox" 只在收件箱, "all" 除垃圾邮件与已删除外的全部。
	Scope string `json:"scope"`
}

// SmartFolderCount 是筛选文件夹里本地缓存的邮件数与其中的未读数, Accounts 按本机账号 id 分开数 (没有的账号不出现)。
type SmartFolderCount struct {
	Total    int                            `json:"total"`
	Unread   int                            `json:"unread"`
	Accounts map[string]store.FilteredCount `json:"accounts"`
}

// smartFolderValue 是一条定义的存储值 (也是同步文件里的一条); 名字为空是墓碑。
type smartFolderValue struct {
	Name       string   `json:"name,omitempty"`
	Unread     bool     `json:"unread,omitempty"`
	Flag       string   `json:"flag,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Attachment bool     `json:"attachment,omitempty"`
	From       string   `json:"from,omitempty"`
	// Accounts 是账号的同步键。
	Accounts []string `json:"accounts,omitempty"`
	Scope    string   `json:"scope,omitempty"`
	// Created 是新建的时间 (Unix 毫秒), 侧栏按它排先后; 改条件不变。
	Created int64 `json:"created,omitempty"`
}

// normalizeSmartFolder 去掉首尾空白、限制长度, 不认识的取值丢掉, 标签去重排序。
func normalizeSmartFolder(v smartFolderValue) smartFolderValue {
	v.Name = strings.Join(strings.Fields(v.Name), " ")
	if r := []rune(v.Name); len(r) > 64 {
		v.Name = string(r[:64])
	}
	if v.Flag != "flagged" && v.Flag != "completed" {
		v.Flag = ""
	}
	if v.Scope != smartScopeInbox && v.Scope != smartScopeAll {
		v.Scope = smartScopeDefault
	}
	v.From = strings.TrimSpace(v.From)
	if r := []rune(v.From); len(r) > 128 {
		v.From = string(r[:128])
	}
	var tags []string
	for _, kw := range v.Tags {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if validateLabel(kw) == nil && !slices.Contains(tags, kw) {
			tags = append(tags, kw)
		}
	}
	slices.Sort(tags)
	v.Tags = tags
	var accounts []string
	for _, k := range v.Accounts {
		if k != "" && !slices.Contains(accounts, k) {
			accounts = append(accounts, k)
		}
	}
	slices.Sort(accounts)
	v.Accounts = accounts
	return v
}

func encodeSmartFolder(v smartFolderValue) string {
	if v.Name == "" {
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func decodeSmartFolder(s string) smartFolderValue {
	var v smartFolderValue
	if s != "" {
		_ = json.Unmarshal([]byte(s), &v)
	}
	return normalizeSmartFolder(v)
}

// smartAccountKey 拆开账号的同步键: 会话地址与服务器上的账号 id。登录那一节的键 (#@用户) 不是账号。
func smartAccountKey(key string) (endpoint, serverID string, ok bool) {
	endpoint, serverID, ok = strings.Cut(key, "#")
	return endpoint, serverID, ok && serverID != "" && !strings.HasPrefix(serverID, "@")
}

// localSmartAccounts 把同步键换成本机账号 id; 同一台服务器登了两个人时只取靠前的登录 (同收藏的文件夹)。
// 对不上本机任何登录的键不出现。
func localSmartAccounts(keys []string, signIns []lookSignIn) []string {
	out := []string{}
	for _, key := range keys {
		endpoint, serverID, ok := smartAccountKey(key)
		if !ok {
			continue
		}
		for _, si := range signIns {
			if si.endpoint == endpoint {
				out = append(out, accountid.Join(si.id, serverID))
				break
			}
		}
	}
	return out
}

func (v smartFolderValue) local(id string, signIns []lookSignIn) SmartFolder {
	f := SmartFolder{
		ID: id, Name: v.Name, Unread: v.Unread, Flag: v.Flag, Tags: v.Tags,
		Attachment: v.Attachment, From: v.From, Accounts: []string{}, Scope: v.Scope,
	}
	if f.Tags == nil {
		f.Tags = []string{}
	}
	for _, key := range v.Accounts {
		if acc := localSmartAccounts([]string{key}, signIns); len(acc) > 0 {
			f.Accounts = append(f.Accounts, acc[0])
		} else {
			f.AccountsElsewhere++
		}
	}
	return f
}

// GetSmartFolders 按新建的先后列出筛选文件夹。
func (a *App) GetSmartFolders() ([]SmartFolder, error) {
	st, err := a.currentStore()
	if err != nil {
		return nil, err
	}
	rows, err := st.TimedSettingsWithPrefix(a.ctx, smartFolderPrefix)
	if err != nil {
		return nil, err
	}
	return smartFolderList(rows, a.lookSignIns()), nil
}

func smartFolderList(rows map[string]store.TimedSetting, signIns []lookSignIn) []SmartFolder {
	type entry struct {
		id string
		v  smartFolderValue
	}
	var list []entry
	for id, row := range rows {
		if v := decodeSmartFolder(row.Value); v.Name != "" {
			list = append(list, entry{id, v})
		}
	}
	slices.SortFunc(list, func(x, y entry) int {
		return cmp.Or(cmp.Compare(x.v.Created, y.v.Created), strings.Compare(x.id, y.id))
	})
	out := make([]SmartFolder, len(list))
	for i, e := range list {
		out[i] = e.v.local(e.id, signIns)
	}
	return out
}

// SaveSmartFolder 新建 (ID 为空) 或修改一个筛选文件夹, 返回保存后的定义。名字不能为空。
func (a *App) SaveSmartFolder(f SmartFolder) (SmartFolder, error) {
	st, err := a.currentStore()
	if err != nil {
		return SmartFolder{}, err
	}
	v := normalizeSmartFolder(smartFolderValue{
		Name: f.Name, Unread: f.Unread, Flag: f.Flag, Tags: f.Tags, Attachment: f.Attachment, From: f.From, Scope: f.Scope,
	})
	if v.Name == "" {
		return SmartFolder{}, fmt.Errorf("2252 smart folder name is empty")
	}
	signIns := a.lookSignIns()
	id := strings.TrimSpace(f.ID)
	if id == "" {
		id = newSmartFolderID()
		v.Created = time.Now().UnixMilli()
	} else {
		cur, err := st.StringSetting(a.ctx, smartFolderPrefix+id)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return SmartFolder{}, err
		}
		old := decodeSmartFolder(cur)
		if old.Name == "" {
			// 别的设备刚删掉了它: 照样保存, 当作重新建一个。
			old.Created = time.Now().UnixMilli()
		}
		v.Created = old.Created
		// 本机没有的登录的账号原样留着。
		for _, key := range old.Accounts {
			if len(localSmartAccounts([]string{key}, signIns)) == 0 {
				v.Accounts = append(v.Accounts, key)
			}
		}
	}
	for _, acc := range f.Accounts {
		if strings.HasPrefix(acc, signInLookPrefix) {
			continue
		}
		key, err := a.accountLookSyncKey(acc)
		if err != nil {
			return SmartFolder{}, err
		}
		v.Accounts = append(v.Accounts, key)
	}
	v = normalizeSmartFolder(v)
	if err := a.putSmartFolder(st, id, encodeSmartFolder(v)); err != nil {
		return SmartFolder{}, err
	}
	return v.local(id, signIns), nil
}

// newSmartFolderID 是新筛选文件夹的 id: 随机的 16 位十六进制, 两台设备各自新建的不会撞上。
func newSmartFolderID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// DeleteSmartFolder 删除一个筛选文件夹 (只是定义, 邮件不受影响); 留墓碑, 删除才能传到别的设备。
func (a *App) DeleteSmartFolder(id string) error {
	st, err := a.currentStore()
	if err != nil {
		return err
	}
	return a.putSmartFolder(st, strings.TrimSpace(id), "")
}

func (a *App) putSmartFolder(st *store.Store, id, value string) error {
	if id == "" || strings.ContainsAny(id, ":/") {
		return fmt.Errorf("2253 unknown smart folder %q", id)
	}
	key := smartFolderPrefix + id
	cur, err := st.StringSetting(a.ctx, key)
	switch {
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return err
	case err == nil && cur == value, errors.Is(err, store.ErrNotFound) && value == "":
		return nil
	}
	if err := st.SetStringSetting(a.ctx, key, value); err != nil {
		return err
	}
	a.emit(EventSettingsChanged)
	a.syncSmartFoldersSoon()
	return nil
}

// smartFolderByID 读一个筛选文件夹的定义 (本机账号 id)。
func (a *App) smartFolderByID(st *store.Store, id string) (SmartFolder, error) {
	cur, err := st.StringSetting(a.ctx, smartFolderPrefix+id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return SmartFolder{}, err
	}
	v := decodeSmartFolder(cur)
	if v.Name == "" {
		return SmartFolder{}, fmt.Errorf("2253 unknown smart folder %q", id)
	}
	return v.local(id, a.lookSignIns()), nil
}

// smartMailboxes 是算筛选文件夹用的账号与文件夹快照: 一次数多个筛选文件夹时只读一遍。
type smartMailboxes struct {
	accounts []store.Account
	excluded []string
	boxes    map[string][]store.Mailbox
}

func (a *App) loadSmartMailboxes(st *store.Store) (smartMailboxes, error) {
	accounts, err := st.Accounts(a.ctx)
	if err != nil {
		return smartMailboxes{}, err
	}
	snap := smartMailboxes{accounts: accounts, excluded: a.accountListSetting(st, settingUnifiedExcluded), boxes: map[string][]store.Mailbox{}}
	for _, acc := range accounts {
		boxes, err := st.Mailboxes(a.ctx, acc.ID)
		if err != nil {
			return smartMailboxes{}, err
		}
		snap.boxes[acc.ID] = boxes
	}
	return snap, nil
}

// smartFolderQuery 是筛选文件夹要找的文件夹与条件; only 不为空时只看这一个本机账号。
// 选了账号却一个都不在本机 (别的设备上的登录) 时没有文件夹, 列表为空, 而不是退回所有账号。
func smartFolderQuery(snap smartMailboxes, f SmartFolder, only string) ([]store.MailboxRef, store.EmailFilter) {
	filter := store.EmailFilter{Unread: f.Unread, Flag: f.Flag, Keywords: f.Tags, Attachment: f.Attachment, From: f.From}
	picked := len(f.Accounts) > 0 || f.AccountsElsewhere > 0
	var out []store.MailboxRef
	for _, acc := range snap.accounts {
		if only != "" && acc.ID != only ||
			picked && !slices.Contains(f.Accounts, acc.ID) || !picked && slices.Contains(snap.excluded, acc.ID) {
			continue
		}
		for _, b := range snap.boxes[acc.ID] {
			var match bool
			switch f.Scope {
			case smartScopeInbox:
				match = b.Kind == store.KindInbox
			case smartScopeAll:
				match = b.Kind != store.KindJunk && b.Kind != store.KindTrash
			default:
				match = !slices.Contains(unreadSkipKinds, b.Kind)
			}
			if match {
				out = append(out, store.MailboxRef{AccountID: acc.ID, MailboxID: b.ID})
			}
		}
	}
	return out, filter
}

// listSmartFolder 分页列出筛选文件夹 (only 不为空时其中一个账号) 里的邮件, 按收件时间倒序。
func (a *App) listSmartFolder(st *store.Store, id, only string, limit, offset int) ([]store.EmailSummary, error) {
	f, err := a.smartFolderByID(st, id)
	if err != nil {
		return nil, err
	}
	snap, err := a.loadSmartMailboxes(st)
	if err != nil {
		return nil, err
	}
	boxes, filter := smartFolderQuery(snap, f, only)
	return st.FilteredEmails(a.ctx, boxes, filter, limit, offset)
}

// SmartFolderCounts 数出每个筛选文件夹里的邮件数与未读数, 键是 id。
func (a *App) SmartFolderCounts() (map[string]SmartFolderCount, error) {
	st, err := a.currentStore()
	if err != nil {
		return nil, err
	}
	rows, err := st.TimedSettingsWithPrefix(a.ctx, smartFolderPrefix)
	if err != nil {
		return nil, err
	}
	out := map[string]SmartFolderCount{}
	folders := smartFolderList(rows, a.lookSignIns())
	if len(folders) == 0 {
		return out, nil
	}
	snap, err := a.loadSmartMailboxes(st)
	if err != nil {
		return nil, err
	}
	for _, f := range folders {
		boxes, filter := smartFolderQuery(snap, f, "")
		byAccount, err := st.CountFilteredEmails(a.ctx, boxes, filter)
		if err != nil {
			return nil, err
		}
		c := SmartFolderCount{Accounts: byAccount}
		for _, n := range byAccount {
			c.Total += n.Total
			c.Unread += n.Unread
		}
		out[f.ID] = c
	}
	return out, nil
}

type smartFoldersFile struct {
	Version    int                        `json:"version"`
	Type       string                     `json:"type"`
	ExportedAt string                     `json:"exportedAt"`
	Folders    map[string]smartFolderFile `json:"folders"`
}

// smartFolderFile 是文件里的一个筛选文件夹, 键是 id; 名字为空是墓碑。
type smartFolderFile struct {
	smartFolderValue
	UpdatedAt string `json:"updatedAt"`
}

// syncSmartFoldersSoon 在后台同步筛选文件夹, 失败只记日志。
func (a *App) syncSmartFoldersSoon() {
	go func() {
		if err := a.SyncSmartFolders(); errors.Is(err, errNoDriveSync) {
			a.log.Debug("sync smart folders skipped", "err", err)
		} else if err != nil {
			a.log.Warn("sync smart folders", "err", err)
		}
	}()
}

// SyncSmartFolders 立即与个人网盘里的 smart-folders.json 合并。
func (a *App) SyncSmartFolders() error { return retryDriveSync(a.syncSmartFoldersOnce) }

func (a *App) syncSmartFoldersOnce() error {
	driveSyncMu.Lock()
	defer driveSyncMu.Unlock()

	st, err := a.currentStore()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()

	file, data, err := a.openDriveSyncFile(ctx, smartFoldersFileName)
	if err != nil {
		return err
	}
	remote := map[string]store.TimedSetting{}
	if data != nil {
		var f smartFoldersFile
		if err := json.Unmarshal(data, &f); err != nil || f.Type != smartFoldersFileType {
			return fmt.Errorf("2254 %s/%s is not a smart folders file", driveSyncFolder, smartFoldersFileName)
		}
		for id, e := range f.Folders {
			remote[id] = store.TimedSetting{
				Value:     encodeSmartFolder(normalizeSmartFolder(e.smartFolderValue)),
				UpdatedAt: parseISO(e.UpdatedAt),
			}
		}
	}

	local, err := st.TimedSettingsWithPrefix(ctx, smartFolderPrefix)
	if err != nil {
		return err
	}
	merged, changes, remoteChanged := mergeTimed(local, remote, nil)
	if len(changes) > 0 {
		if err := st.WithTx(ctx, func(tx *sql.Tx) error {
			for k, v := range changes {
				if err := store.PutTimedSetting(ctx, tx, smartFolderPrefix+k, v); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		a.emit(EventSettingsChanged)
	}

	if !remoteChanged && file.exists() {
		return nil
	}
	out := smartFoldersFile{
		Version: 1, Type: smartFoldersFileType, ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Folders: make(map[string]smartFolderFile, len(merged)),
	}
	for k, v := range merged {
		out.Folders[k] = smartFolderFile{smartFolderValue: decodeSmartFolder(v.Value), UpdatedAt: isoTime(v.UpdatedAt)}
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return file.write(ctx, body)
}
