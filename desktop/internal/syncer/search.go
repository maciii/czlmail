package syncer

import (
	"context"
	"time"

	"git.sr.ht/~rockorager/go-jmap"
	"git.sr.ht/~rockorager/go-jmap/mail/email"
)

// SearchServer 交给服务端做全文检索(Email/query 的 text 条件), 覆盖整个账号的全部文件夹。
//
// 本地缓存只有首次引导的最近一批与翻过的页, 归档里的旧邮件多半不在本地, 只搜本地就会搜不到;
// 服务端还能命中没拉过正文的邮件。命中而本地没有的邮件把元数据补进缓存(不拉正文),
// 返回 id 按收件时间倒序。与翻页补取一样不写 state、不拿同步锁。
func (s *Syncer) SearchServer(ctx context.Context, accountID, text string, limit int) ([]string, error) {
	return s.QueryServer(ctx, accountID, EmailFilter{Text: text}, limit)
}

// EmailFilter 是服务端 Email/query 的条件子集, 各项之间为"且"。零值字段不参与过滤。
type EmailFilter struct {
	Text      string
	From      string
	To        string
	Subject   string
	MailboxID string
	After     time.Time
	Before    time.Time
	// Unread / Flagged / HasAttachment 为真时才过滤, 不能表达"必须没有"。
	Unread        bool
	Flagged       bool
	HasAttachment bool
	// Keyword 是必须有的关键字 (标签、$completed 等); 与 Flagged 同时给时以 Flagged 为准。
	Keyword string
}

// Empty 表示没有任何条件, 这时查询等于列出整个账号, 调用方应拒绝。
func (f EmailFilter) Empty() bool {
	return f == EmailFilter{}
}

// QueryServer 按条件查询服务端, 语义同 SearchServer: 命中而本地没有的补元数据进缓存, 返回 id 按收件时间倒序。
func (s *Syncer) QueryServer(ctx context.Context, accountID string, f EmailFilter, limit int) ([]string, error) {
	ids, _, err := s.queryServer(ctx, accountID, f, limit)
	return ids, err
}

// FillFromServer 同 QueryServer, 只为把本地缺的邮件补进缓存, 返回补进了几封。
func (s *Syncer) FillFromServer(ctx context.Context, accountID string, f EmailFilter, limit int) (int, error) {
	_, added, err := s.queryServer(ctx, accountID, f, limit)
	return added, err
}

func (s *Syncer) queryServer(ctx context.Context, accountID string, f EmailFilter, limit int) ([]string, int, error) {
	cond := &email.FilterCondition{
		Text: f.Text, From: f.From, To: f.To, Subject: f.Subject,
		InMailbox: jmap.ID(f.MailboxID), HasAttachment: f.HasAttachment,
	}
	if !f.After.IsZero() {
		cond.After = &f.After
	}
	if !f.Before.IsZero() {
		cond.Before = &f.Before
	}
	if f.Unread {
		cond.NotKeyword = "$seen"
	}
	if f.Keyword != "" {
		cond.HasKeyword = f.Keyword
	}
	if f.Flagged {
		cond.HasKeyword = "$flagged"
	}

	req := &jmap.Request{Context: ctx}
	req.Invoke(&email.Query{
		Account: s.server(accountID),
		Filter:  cond,
		Sort:    []*email.SortComparator{{Property: "receivedAt", IsAscending: false}},
		Limit:   uint64(limit),
	})

	resp, err := s.do(req)
	if err != nil {
		return nil, 0, err
	}
	q, ok := resp.Responses[0].Args.(*email.QueryResponse)
	if !ok {
		return nil, 0, &Error{Code: CodeUnhandled, Msg: "unexpected response to Email/query"}
	}
	added, err := s.cacheMissing(ctx, accountID, q.IDs)
	if err != nil {
		return nil, 0, err
	}
	return idsToStrings(q.IDs), added, nil
}
