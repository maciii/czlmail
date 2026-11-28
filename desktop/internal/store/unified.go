package store

import (
	"context"
	"strings"
)

// MailboxRef 指定某个账号的一个文件夹。
type MailboxRef struct {
	AccountID string
	MailboxID string
}

// FlaggedInMailboxes 取这些文件夹里带 $flagged 的邮件 (待办里的旗标邮件), 按收件时间倒序, 最多 limit 封。
func (s *Store) FlaggedInMailboxes(ctx context.Context, boxes []MailboxRef, limit int) ([]EmailSummary, error) {
	if len(boxes) == 0 {
		return nil, nil
	}
	conds := make([]string, len(boxes))
	args := make([]any, 0, len(boxes)*2+1)
	for i, b := range boxes {
		conds[i] = "(m.account_id = ? AND m.mailbox_id = ?)"
		args = append(args, b.AccountID, b.MailboxID)
	}
	args = append(args, limit)
	rows, err := s.rd.QueryContext(ctx, `
		SELECT `+summaryColumns+threadCountColumn+`
		FROM emails e
		WHERE EXISTS (SELECT 1 FROM email_keywords k
		              WHERE k.account_id = e.account_id AND k.email_id = e.id AND k.keyword = '$flagged')
		  AND EXISTS (SELECT 1 FROM email_mailboxes m
		              WHERE m.account_id = e.account_id AND m.email_id = e.id
		                AND (`+strings.Join(conds, " OR ")+`))
		ORDER BY e.received_at DESC, e.account_id, e.id
		LIMIT ?`, args...)
	if err != nil {
		return nil, wrap(CodeQuery, "query flagged emails", err)
	}
	defer rows.Close()
	return scanSummaries(rows)
}

// EmailsInMailboxes 跨账号列出若干文件夹里的邮件, 按收件时间倒序, 供统一视图使用。
// 同一封邮件在多个文件夹里只出现一次; unreadOnly 只列未读。
//
// 不像单个文件夹那样把置顶的排在前面: 统一视图是各账号的邮件按时间合流,
// 某个共享邮箱的置顶邮件压在所有账号前面反而打乱阅读顺序。
func (s *Store) EmailsInMailboxes(
	ctx context.Context, boxes []MailboxRef, unreadOnly bool, limit, offset int,
) ([]EmailSummary, error) {
	return s.FilteredEmails(ctx, boxes, EmailFilter{Unread: unreadOnly}, limit, offset)
}

// EmailFilter 是筛选文件夹的条件, 各条件同时满足; 零值不筛选。
type EmailFilter struct {
	// Unread 只列未读。
	Unread bool
	// Flag 是待办旗标: "flagged" 待办 ($flagged), "completed" 已完成 ($completed 且没有 $flagged), 空串不限。
	Flag string
	// Keywords 是标签 (关键字), 有其中任意一个即可。
	Keywords []string
	// Attachment 只列有附件的。
	Attachment bool
	// From 是发件人 (名字或地址) 里要包含的文字, 不分大小写 (仅 ASCII 字母)。
	From string
}

// hasKeywordSQL 是"邮件 e 有关键字 kw"的条件; kw 只来自本文件的常量。
func hasKeywordSQL(kw string) string {
	return `EXISTS (SELECT 1 FROM email_keywords k
	        WHERE k.account_id = e.account_id AND k.email_id = e.id AND k.keyword = '` + kw + `')`
}

// filterWhere 拼出在 boxes 里、满足 f 的邮件 (表别名 e) 的 WHERE 条件与参数。
func filterWhere(boxes []MailboxRef, f EmailFilter) (string, []any) {
	conds := make([]string, len(boxes))
	args := make([]any, 0, len(boxes)*2+len(f.Keywords)+1)
	for i, b := range boxes {
		conds[i] = "(m.account_id = ? AND m.mailbox_id = ?)"
		args = append(args, b.AccountID, b.MailboxID)
	}
	where := []string{`EXISTS (SELECT 1 FROM email_mailboxes m
	              WHERE m.account_id = e.account_id AND m.email_id = e.id
	                AND (` + strings.Join(conds, " OR ") + `))`}
	if f.Unread {
		where = append(where, "NOT "+hasKeywordSQL("$seen"))
	}
	switch f.Flag {
	case "flagged":
		where = append(where, hasKeywordSQL("$flagged"))
	case "completed":
		where = append(where, hasKeywordSQL("$completed"), "NOT "+hasKeywordSQL("$flagged"))
	}
	if len(f.Keywords) > 0 {
		where = append(where, `EXISTS (SELECT 1 FROM email_keywords k
		        WHERE k.account_id = e.account_id AND k.email_id = e.id
		          AND k.keyword IN (?`+strings.Repeat(", ?", len(f.Keywords)-1)+`))`)
		for _, kw := range f.Keywords {
			args = append(args, kw)
		}
	}
	if f.Attachment {
		where = append(where, "e.has_attachment = 1")
	}
	if from := strings.TrimSpace(f.From); from != "" {
		where = append(where, `e.from_json LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(from)+"%")
	}
	return strings.Join(where, "\n\t\t  AND "), args
}

// FilteredEmails 跨账号列出若干文件夹里满足 f 的邮件, 按收件时间倒序; 同一封邮件只出现一次。
// 只看本地缓存, 与统一视图的「未读」一样。
func (s *Store) FilteredEmails(
	ctx context.Context, boxes []MailboxRef, f EmailFilter, limit, offset int,
) ([]EmailSummary, error) {
	if len(boxes) == 0 {
		return nil, nil
	}
	where, args := filterWhere(boxes, f)
	args = append(args, limit, offset)

	rows, err := s.rd.QueryContext(ctx, `
		SELECT `+summaryColumns+threadCountColumn+`
		FROM emails e
		WHERE `+where+`
		ORDER BY e.received_at DESC, e.account_id, e.id
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, wrap(CodeQuery, "query unified emails", err)
	}
	defer rows.Close()
	return scanSummaries(rows)
}

// FilteredCount 是一个账号里满足条件的邮件数与其中的未读数。
type FilteredCount struct {
	Total  int `json:"total"`
	Unread int `json:"unread"`
}

// CountFilteredEmails 按账号数出若干文件夹里满足 f 的邮件 (同一封只算一次) 与其中的未读数; 没有的账号不出现。
func (s *Store) CountFilteredEmails(ctx context.Context, boxes []MailboxRef, f EmailFilter) (map[string]FilteredCount, error) {
	out := map[string]FilteredCount{}
	if len(boxes) == 0 {
		return out, nil
	}
	where, args := filterWhere(boxes, f)
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.account_id, COUNT(*), COALESCE(SUM(NOT `+hasKeywordSQL("$seen")+`), 0)
		FROM emails e
		WHERE `+where+`
		GROUP BY e.account_id`, args...)
	if err != nil {
		return nil, wrap(CodeQuery, "count filtered emails", err)
	}
	defer rows.Close()
	for rows.Next() {
		var acc string
		var c FilteredCount
		if err := rows.Scan(&acc, &c.Total, &c.Unread); err != nil {
			return nil, wrap(CodeQuery, "scan filtered count", err)
		}
		out[acc] = c
	}
	if err := rows.Err(); err != nil {
		return nil, wrap(CodeQuery, "count filtered emails", err)
	}
	return out, nil
}
