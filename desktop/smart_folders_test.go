package main

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/woodchen-ink/czlmail/desktop/internal/config"
	"github.com/woodchen-ink/czlmail/desktop/internal/store"
)

func TestNormalizeSmartFolder(t *testing.T) {
	got := normalizeSmartFolder(smartFolderValue{
		Name: "  Ke   zpracování ", Flag: "weird", Scope: "everything", From: "  boss ",
		Tags: []string{"Projekt", "projekt", "", "$seen", "faktury", "a b"}, Accounts: []string{"b", "a", "b", ""},
	})
	want := smartFolderValue{Name: "Ke zpracování", From: "boss", Tags: []string{"faktury", "projekt"}, Accounts: []string{"a", "b"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizeSmartFolder = %+v, want %+v", got, want)
	}
	if encodeSmartFolder(smartFolderValue{Unread: true}) != "" {
		t.Error("a folder without a name is a tombstone")
	}
}

func newSmartFolderTestApp(t *testing.T) (*App, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := &App{ctx: ctx, store: st, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	a.tap = func(string, ...any) {}
	a.primary.cfg = config.Config{SessionEndpoint: "https://mail.example/jmap", Username: "me@example"}
	a.extra = []*signIn{{id: "s2", cfg: config.Config{SessionEndpoint: "https://gw.example/jmap", Username: "gw"}}}

	at := func(i int) time.Time { return time.Unix(int64(1700000000+i), 0) }
	err = st.WithTx(ctx, func(tx *sql.Tx) error {
		if err := store.UpsertAccounts(ctx, tx, []store.Account{
			{ID: "c", Name: "Já", IsPersonal: true}, {ID: "s2~g", Name: "Gmail", IsPersonal: true},
		}, nil); err != nil {
			return err
		}
		boxes := []store.Mailbox{
			{ID: "in", Name: "Inbox", Role: "inbox"}, {ID: "arch", Name: "Archive", Role: "archive"},
			{ID: "sent", Name: "Sent", Role: "sent"}, {ID: "junk", Name: "Junk", Role: "junk"},
		}
		for _, acc := range []string{"c", "s2~g"} {
			if err := store.UpsertMailboxes(ctx, tx, acc, boxes); err != nil {
				return err
			}
		}
		if err := store.UpsertEmails(ctx, tx, "c", []store.Email{
			{ID: "1", MailboxIDs: []string{"in"}, ReceivedAt: at(9), Keywords: []string{"$flagged"}},
			{ID: "2", MailboxIDs: []string{"arch"}, ReceivedAt: at(8), Keywords: []string{"$seen", "$flagged"}},
			{ID: "3", MailboxIDs: []string{"sent"}, ReceivedAt: at(7), Keywords: []string{"$seen", "$flagged"}},
			{ID: "4", MailboxIDs: []string{"junk"}, ReceivedAt: at(6), Keywords: []string{"$flagged"}},
			{ID: "5", MailboxIDs: []string{"in"}, ReceivedAt: at(5)},
		}); err != nil {
			return err
		}
		return store.UpsertEmails(ctx, tx, "s2~g", []store.Email{
			{ID: "1", MailboxIDs: []string{"in"}, ReceivedAt: at(4), Keywords: []string{"$seen", "$flagged", "projekt"}},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, st
}

func keysOf(list []store.EmailSummary) string {
	var out []string
	for _, e := range list {
		out = append(out, e.AccountID+"/"+e.ID)
	}
	return strings.Join(out, " ")
}

// 新建、列出、按范围与账号筛选、计数、改名保留新建时间与别的设备上的账号、删除留墓碑。
func TestSmartFolders(t *testing.T) {
	a, st := newSmartFolderTestApp(t)
	ctx := context.Background()

	todo, err := a.SaveSmartFolder(SmartFolder{Name: "Ke zpracování", Flag: "flagged"})
	if err != nil {
		t.Fatal(err)
	}
	if todo.ID == "" || len(todo.Accounts) != 0 || todo.Tags == nil {
		t.Fatalf("saved folder = %+v", todo)
	}
	if _, err := a.SaveSmartFolder(SmartFolder{Name: "  "}); err == nil || !strings.HasPrefix(err.Error(), "2252") {
		t.Errorf("empty name: %v", err)
	}

	list := func(id string) string {
		t.Helper()
		got, err := a.ListUnified(smartViewPrefix+id, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		return keysOf(got)
	}
	// 默认范围同「未读」: 不含已发送与垃圾邮件。
	if got, want := list(todo.ID), "c/1 c/2 s2~g/1"; got != want {
		t.Errorf("flagged: got %q want %q", got, want)
	}

	todo.Scope = smartScopeAll
	todo.Accounts = []string{"c"}
	if todo, err = a.SaveSmartFolder(todo); err != nil {
		t.Fatal(err)
	}
	if got, want := list(todo.ID), "c/1 c/2 c/3"; got != want {
		t.Errorf("all folders of one account: got %q want %q", got, want)
	}

	// 按账号展开的行: 只看一个账号。
	if got, want := list(todo.ID+"@s2~g"), ""; got != want {
		t.Errorf("one account outside the folder: got %q want %q", got, want)
	}
	if got, want := list(todo.ID+"@c"), "c/1 c/2 c/3"; got != want {
		t.Errorf("one account: got %q want %q", got, want)
	}
	if got, err := a.ListUnified("unread@c", 50, 0); err != nil || keysOf(got) != "c/1 c/5" {
		t.Errorf("unread of one account: got %q %v", keysOf(got), err)
	}
	if got, err := a.ListUnified("unread", 50, 0); err != nil || keysOf(got) != "c/1 c/5" {
		t.Errorf("unread: got %q %v", keysOf(got), err)
	}

	inbox, err := a.SaveSmartFolder(SmartFolder{Name: "Projekt", Tags: []string{"Projekt"}, Scope: smartScopeInbox})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := list(inbox.ID), "s2~g/1"; got != want {
		t.Errorf("tag in inboxes: got %q want %q", got, want)
	}

	folders, err := a.GetSmartFolders()
	if err != nil || len(folders) != 2 || folders[0].ID != todo.ID || folders[1].ID != inbox.ID {
		t.Fatalf("GetSmartFolders = %+v, %v", folders, err)
	}
	counts, err := a.SmartFolderCounts()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]SmartFolderCount{
		todo.ID:  {3, 1, map[string]store.FilteredCount{"c": {Total: 3, Unread: 1}}},
		inbox.ID: {1, 0, map[string]store.FilteredCount{"s2~g": {Total: 1, Unread: 0}}},
	}
	if !reflect.DeepEqual(counts, want) {
		t.Errorf("counts = %v, want %v", counts, want)
	}

	// 别的设备上选了一个这台设备没有的登录里的账号: 本机看不到它, 但改名不丢。
	key := smartFolderPrefix + todo.ID
	raw, _ := st.StringSetting(ctx, key)
	v := decodeSmartFolder(raw)
	created := v.Created
	v.Accounts = append(v.Accounts, "https://elsewhere.example/jmap#z")
	if err := st.SetStringSetting(ctx, key, encodeSmartFolder(v)); err != nil {
		t.Fatal(err)
	}
	todo.Name = "K vyřízení"
	saved, err := a.SaveSmartFolder(todo)
	if err != nil {
		t.Fatal(err)
	}
	if saved.AccountsElsewhere != 1 || !reflect.DeepEqual(saved.Accounts, []string{"c"}) {
		t.Errorf("saved = %+v", saved)
	}
	raw, _ = st.StringSetting(ctx, key)
	if v := decodeSmartFolder(raw); v.Created != created || !reflect.DeepEqual(v.Accounts, []string{"https://elsewhere.example/jmap#z", "https://mail.example/jmap#c"}) {
		t.Errorf("stored after rename = %+v (created %d)", v, created)
	}

	// 只选了别的设备上的账号: 列表为空, 不退回全部账号。
	saved.Accounts = nil
	if _, err := a.SaveSmartFolder(saved); err != nil {
		t.Fatal(err)
	}
	if got := list(saved.ID); got != "" {
		t.Errorf("only accounts elsewhere: got %q", got)
	}

	if err := a.DeleteSmartFolder(inbox.ID); err != nil {
		t.Fatal(err)
	}
	if v, err := st.StringSetting(ctx, smartFolderPrefix+inbox.ID); err != nil || v != "" {
		t.Errorf("deleted folder should stay as a tombstone: %q %v", v, err)
	}
	if _, err := a.ListUnified(smartViewPrefix+inbox.ID, 50, 0); err == nil || !strings.HasPrefix(err.Error(), "2253") {
		t.Errorf("deleted folder: %v", err)
	}
	if folders, _ := a.GetSmartFolders(); len(folders) != 1 {
		t.Errorf("deleted folder still listed: %+v", folders)
	}
	if err := a.DeleteSmartFolder("a/b"); err == nil {
		t.Error("ids with a slash are rejected")
	}
}

func TestSetSmartFolderOrder(t *testing.T) {
	a, _ := newSmartFolderTestApp(t)
	var ids []string
	for _, name := range []string{"A", "B", "C"} {
		f, err := a.SaveSmartFolder(SmartFolder{Name: name, Unread: true})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, f.ID)
		time.Sleep(2 * time.Millisecond) // 新建时间按毫秒记
	}
	names := func() string {
		t.Helper()
		list, err := a.GetSmartFolders()
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, f := range list {
			out = append(out, f.Name)
		}
		return strings.Join(out, " ")
	}
	// 没排过: 按新建时间。
	if got := names(); got != "A B C" {
		t.Errorf("default order = %q", got)
	}
	// 只排了 C、A: B 没排过, 接在后面。
	if err := a.SetSmartFolderOrder([]string{ids[2], ids[0]}); err != nil {
		t.Fatal(err)
	}
	if got := names(); got != "C A B" {
		t.Errorf("custom order = %q", got)
	}
	// 改条件不改位置。
	list, _ := a.GetSmartFolders()
	edited := list[0]
	edited.Attachment = true
	if _, err := a.SaveSmartFolder(edited); err != nil {
		t.Fatal(err)
	}
	if got := names(); got != "C A B" {
		t.Errorf("order after edit = %q", got)
	}
}

// update_smart_folder 只改给了的条件。
func TestSmartFolderFieldsApply(t *testing.T) {
	f := SmartFolder{Name: "Follow up", Flag: "flagged", Tags: []string{"work"}, Accounts: []string{"c"}, Scope: smartScopeInbox}
	unread := true
	smartFolderFields{Unread: &unread, Tags: []string{}}.apply(&f)
	if !f.Unread || f.Name != "Follow up" || f.Flag != "flagged" || len(f.Tags) != 0 || len(f.Accounts) != 1 || f.Scope != smartScopeInbox {
		t.Errorf("apply = %+v", f)
	}
}
