package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestEmailsInMailboxesAcrossAccounts(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	at := func(i int) time.Time { return time.Unix(int64(1700000000+i), 0) }
	// 两个账号的收件箱 id 相同(Stalwart 就是这样), 邮件 id 也有重复。
	personal := []Email{
		{ID: "x", MailboxIDs: []string{"a"}, ReceivedAt: at(5), Keywords: []string{"$seen"}},
		{ID: "y", MailboxIDs: []string{"a", "h"}, ReceivedAt: at(3)},
		{ID: "z", MailboxIDs: []string{"b"}, ReceivedAt: at(9)},
	}
	shared := []Email{
		{ID: "x", MailboxIDs: []string{"a"}, ReceivedAt: at(4)},
		{ID: "w", MailboxIDs: []string{"a"}, ReceivedAt: at(6), Keywords: []string{"$seen"}},
	}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := UpsertEmails(ctx, tx, "c", personal); err != nil {
			return err
		}
		return UpsertEmails(ctx, tx, "f", shared)
	})
	if err != nil {
		t.Fatal(err)
	}

	keys := func(list []EmailSummary) string {
		var out []string
		for _, e := range list {
			out = append(out, e.AccountID+"/"+e.ID)
		}
		return strings.Join(out, " ")
	}

	inboxes := []MailboxRef{{"c", "a"}, {"f", "a"}}
	var all []EmailSummary
	for offset := 0; offset < 6; offset += 2 {
		page, err := s.EmailsInMailboxes(ctx, inboxes, false, 2, offset)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page...)
	}
	if got, want := keys(all), "f/w c/x f/x c/y"; got != want {
		t.Fatalf("inbox: got %q want %q", got, want)
	}

	// 同一封邮件在两个被选中的文件夹里只出现一次。
	both, err := s.EmailsInMailboxes(ctx, []MailboxRef{{"c", "a"}, {"c", "h"}}, false, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keys(both), "c/x c/y"; got != want {
		t.Fatalf("dedupe: got %q want %q", got, want)
	}

	unread, err := s.EmailsInMailboxes(ctx, inboxes, true, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keys(unread), "f/x c/y"; got != want {
		t.Fatalf("unread: got %q want %q", got, want)
	}
}

func TestFilteredEmails(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	at := func(i int) time.Time { return time.Unix(int64(1700000000+i), 0) }
	boss := []Address{{Name: "Šéf Novák", Email: "boss@example.cz"}}
	personal := []Email{
		{ID: "a", MailboxIDs: []string{"in"}, ReceivedAt: at(9), Keywords: []string{"$flagged"}},
		{ID: "b", MailboxIDs: []string{"in"}, ReceivedAt: at(8), Keywords: []string{"$seen", "$flagged", "faktury"}, From: boss},
		{ID: "c", MailboxIDs: []string{"in"}, ReceivedAt: at(7), Keywords: []string{"$seen", "$completed"}, HasAttachment: true},
		{ID: "d", MailboxIDs: []string{"in", "arch"}, ReceivedAt: at(6), Keywords: []string{"projekt"}, From: boss, HasAttachment: true},
		{ID: "e", MailboxIDs: []string{"arch"}, ReceivedAt: at(5), Keywords: []string{"$flagged", "$completed"}},
	}
	shared := []Email{
		{ID: "a", MailboxIDs: []string{"in"}, ReceivedAt: at(4), Keywords: []string{"$seen", "$flagged", "projekt"}},
		{ID: "x", MailboxIDs: []string{"in"}, ReceivedAt: at(3), From: []Address{{Name: "100% jistota_", Email: "x@example.cz"}}},
	}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := UpsertEmails(ctx, tx, "c", personal); err != nil {
			return err
		}
		return UpsertEmails(ctx, tx, "f", shared)
	})
	if err != nil {
		t.Fatal(err)
	}
	keys := func(list []EmailSummary) string {
		var out []string
		for _, e := range list {
			out = append(out, e.AccountID+"/"+e.ID)
		}
		return strings.Join(out, " ")
	}
	boxes := []MailboxRef{{"c", "in"}, {"c", "arch"}, {"f", "in"}}
	cases := []struct {
		name   string
		f      EmailFilter
		want   string
		unread int
	}{
		{"all", EmailFilter{}, "c/a c/b c/c c/d c/e f/a f/x", 4},
		{"unread", EmailFilter{Unread: true}, "c/a c/d c/e f/x", 4},
		{"flagged", EmailFilter{Flag: "flagged"}, "c/a c/b c/e f/a", 2},
		{"flagged unread", EmailFilter{Flag: "flagged", Unread: true}, "c/a c/e", 2},
		{"completed", EmailFilter{Flag: "completed"}, "c/c", 0},
		{"tags any", EmailFilter{Keywords: []string{"projekt", "faktury"}}, "c/b c/d f/a", 1},
		{"tag and flag", EmailFilter{Keywords: []string{"projekt"}, Flag: "flagged"}, "f/a", 0},
		{"attachment", EmailFilter{Attachment: true}, "c/c c/d", 1},
		{"from ascii any case", EmailFilter{From: "BOSS@"}, "c/b c/d", 1},
		{"from name", EmailFilter{From: "Novák"}, "c/b c/d", 1},
		{"from literal wildcards", EmailFilter{From: "100%"}, "f/x", 1},
		{"from underscore is literal", EmailFilter{From: "a_"}, "f/x", 1},
		{"no match", EmailFilter{From: "nikdo"}, "", 0},
	}
	for _, tc := range cases {
		list, err := s.FilteredEmails(ctx, boxes, tc.f, 50, 0)
		if err != nil {
			t.Fatal(tc.name, err)
		}
		if got := keys(list); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
		counts, err := s.CountFilteredEmails(ctx, boxes, tc.f)
		if err != nil {
			t.Fatal(tc.name, err)
		}
		total, unread := 0, 0
		for _, c := range counts {
			total += c.Total
			unread += c.Unread
		}
		if total != len(list) || unread != tc.unread {
			t.Errorf("%s: count %d/%d want %d/%d", tc.name, total, unread, len(list), tc.unread)
		}
	}
	// 只在给定的文件夹里找; 两个文件夹里都有的邮件只算一次。
	if list, _ := s.FilteredEmails(ctx, []MailboxRef{{"c", "arch"}}, EmailFilter{}, 50, 0); keys(list) != "c/d c/e" {
		t.Errorf("archive only: got %q", keys(list))
	}
	if counts, _ := s.CountFilteredEmails(ctx, boxes, EmailFilter{Flag: "flagged"}); counts["c"] != (FilteredCount{3, 2}) || counts["f"] != (FilteredCount{1, 0}) {
		t.Errorf("per account: got %v", counts)
	}
	if counts, err := s.CountFilteredEmails(ctx, nil, EmailFilter{}); err != nil || len(counts) != 0 {
		t.Errorf("no boxes: got %v %v", counts, err)
	}
}
