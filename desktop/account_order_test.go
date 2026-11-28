package main

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/woodchen-ink/czlmail/desktop/internal/config"
	"github.com/woodchen-ink/czlmail/desktop/internal/store"
)

func accountIDs(list []store.Account) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.ID
	}
	return out
}

func TestSortAccounts(t *testing.T) {
	signIns := map[string]lookSignIn{
		"":   {id: "", endpoint: "https://mail", user: "me"},
		"s2": {id: "s2", endpoint: "http://gw", user: "gmail"},
		"s3": {id: "s3", endpoint: "http://gw", user: "work"},
	}
	def := map[string]int{"": 0, "s2": 1, "s3": 2}
	// 库里的顺序: 个人在前、按名称。
	base := []store.Account{
		{ID: "c", IsPersonal: true},
		{ID: "s2~a", IsPersonal: true},
		{ID: "s2~b", IsPersonal: true},
		{ID: "s2~g", IsPersonal: true},
		{ID: "s3~w", IsPersonal: true},
		{ID: "shared", IsPersonal: false},
	}
	run := func(order []string) []string {
		list := slices.Clone(base)
		sortAccounts(list, order, signIns, def)
		return accountIDs(list)
	}

	if got, want := run(nil), []string{"c", "shared", "s2~a", "s2~b", "s2~g", "s3~w"}; !reflect.DeepEqual(got, want) {
		t.Errorf("default order = %v, want %v", got, want)
	}

	// Exchange nahoru, v Gmailu g před a; b a hlavní přihlášení v seznamu nejsou.
	order := []string{"http://gw#@work", "http://gw#g", "http://gw#a", "elsewhere#@x"}
	if got, want := run(order), []string{"s3~w", "c", "shared", "s2~g", "s2~a", "s2~b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("custom order = %v, want %v", got, want)
	}

	// Sdílený účet zůstává za osobními, i když je v seznamu první.
	order = []string{"https://mail#shared", "https://mail#c"}
	if got, want := run(order), []string{"c", "shared", "s2~a", "s2~b", "s2~g", "s3~w"}; !reflect.DeepEqual(got, want) {
		t.Errorf("shared first = %v, want %v", got, want)
	}

	// Hlavní přihlášení se dá posunout dolů.
	order = []string{"http://gw#@gmail", "https://mail#@me"}
	if got, want := run(order), []string{"s2~a", "s2~b", "s2~g", "c", "shared", "s3~w"}; !reflect.DeepEqual(got, want) {
		t.Errorf("primary moved = %v, want %v", got, want)
	}
}

func TestSetAccountOrder(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.WithTx(ctx, func(tx *sql.Tx) error {
		return store.UpsertAccounts(ctx, tx, []store.Account{
			{ID: "c", Name: "martin", IsPersonal: true},
			{ID: "s2~a", Name: "a@gmail", IsPersonal: true},
			{ID: "s2~b", Name: "b@gmail", IsPersonal: true},
			{ID: "s3~w", Name: "Work", IsPersonal: true},
		}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	var events []string
	a := &App{ctx: ctx, store: st, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	a.tap = func(name string, _ ...any) { events = append(events, name) }
	a.primary.cfg = config.Config{SessionEndpoint: "https://Mail.example/jmap/", Username: "me"}
	a.extra = []*signIn{
		{id: "s2", cfg: config.Config{SessionEndpoint: "http://localhost:18090/jmap", Username: "gmail"}},
		{id: "s3", cfg: config.Config{SessionEndpoint: "http://localhost:18090/jmap", Username: "work"}},
	}

	// Na jiném zařízení je navíc přihlášení, které tady není: jeho klíč musí zůstat.
	if err := st.SetStringSetting(ctx, accountOrderKey, encodeAccountOrder([]string{"https://other#@x", "https://mail.example/jmap#@me"})); err != nil {
		t.Fatal(err)
	}

	keys := []string{"signin:s3", "s3~w", "signin:", "c", "signin:s2", "s2~b", "s2~a"}
	if err := a.SetAccountOrder(keys); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(events, EventAccountsChanged) {
		t.Errorf("SetAccountOrder should announce the new order, events %v", events)
	}
	v, _ := st.StringSetting(ctx, accountOrderKey)
	wantStored := []string{
		"http://localhost:18090/jmap#@work", "http://localhost:18090/jmap#w",
		"https://mail.example/jmap#@me", "https://mail.example/jmap#c",
		"http://localhost:18090/jmap#@gmail", "http://localhost:18090/jmap#b", "http://localhost:18090/jmap#a",
		"https://other#@x",
	}
	if got := decodeAccountOrder(v); !reflect.DeepEqual(got, wantStored) {
		t.Errorf("stored order = %v, want %v", got, wantStored)
	}

	list, err := a.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := accountIDs(list), []string{"s3~w", "c", "s2~b", "s2~a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListAccounts = %v, want %v", got, want)
	}

	// Stejné pořadí znovu: nic se nezapíše ani neohlásí.
	events = nil
	if err := a.SetAccountOrder(keys); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("unchanged order emitted %v", events)
	}

	// Prázdný seznam = výchozí pořadí (náhrobek).
	if err := a.SetAccountOrder(nil); err != nil {
		t.Fatal(err)
	}
	if v, err := st.StringSetting(ctx, accountOrderKey); err != nil || v != "" {
		t.Errorf("reset should leave a tombstone, got %q %v", v, err)
	}
	list, _ = a.ListAccounts()
	if got, want := accountIDs(list), []string{"c", "s2~a", "s2~b", "s3~w"}; !reflect.DeepEqual(got, want) {
		t.Errorf("default ListAccounts = %v, want %v", got, want)
	}

	if err := a.SetAccountOrder([]string{"s9~x"}); err == nil {
		t.Error("unknown sign-in should be refused")
	}
}

func TestMergeTimedAccountOrder(t *testing.T) {
	local := map[string]store.TimedSetting{accountOrderName: {Value: encodeAccountOrder([]string{"a", "b"}), UpdatedAt: 10}}
	remote := map[string]store.TimedSetting{accountOrderName: {Value: encodeAccountOrder([]string{"b", "a"}), UpdatedAt: 20}}
	merged, changes, remoteChanged := mergeTimed(local, remote, nil)
	if got := decodeAccountOrder(merged[accountOrderName].Value); !reflect.DeepEqual(got, []string{"b", "a"}) || len(changes) != 1 || remoteChanged {
		t.Errorf("newer remote order should win: %v %v %v", got, changes, remoteChanged)
	}
}
