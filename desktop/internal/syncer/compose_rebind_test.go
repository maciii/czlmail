package syncer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woodchen-ink/czlmail/desktop/internal/store"
)

func TestRebindAttachments(t *testing.T) {
	sent := []Attachment{
		{BlobID: "pOLD.2", Type: "application/pdf", Name: "faktura.pdf"},
		{BlobID: "pOLD.3", Type: "application/xml", Name: "faktura.isdoc"},
		{BlobID: "u123", Type: "text/plain", Name: "faktura.isdoc"},
		{BlobID: "pORIG.4", Type: "image/png", Name: "logo.png", CID: "<logo@x>"},
	}
	// 服务器的顺序: 内嵌部件在前; 新上传的文件服务器认成了别的类型。
	got := []Attachment{
		{BlobID: "pNEW.2", Type: "image/png", Name: "logo.png", CID: "logo@x"},
		{BlobID: "pNEW.3", Type: "application/pdf", Name: "faktura.pdf"},
		{BlobID: "pNEW.4", Type: "application/xml", Name: "faktura.isdoc"},
		{BlobID: "pNEW.5", Type: "application/octet-stream", Name: "faktura.isdoc"},
	}
	out := rebindAttachments(sent, got)
	want := []string{"pNEW.3", "pNEW.4", "pNEW.5", "pNEW.2"}
	for i, a := range out {
		if a.BlobID != want[i] {
			t.Errorf("attachment %d (%s): blob %s, want %s", i, sent[i].Name, a.BlobID, want[i])
		}
		if a.Name != sent[i].Name || a.Type != sent[i].Type || a.CID != sent[i].CID {
			t.Errorf("attachment %d: only the blob may change, got %+v", i, a)
		}
	}
	if sent[0].BlobID != "pOLD.2" {
		t.Error("the input must not be modified")
	}
}

func TestRebindAttachmentsKeepsUnmatched(t *testing.T) {
	sent := []Attachment{{BlobID: "a", Name: "a.pdf"}, {BlobID: "b", Name: "b.pdf"}}
	// 只回来一个改了名的部件: 不知道是哪一个, 两个都保持原样。
	out := rebindAttachments(sent, []Attachment{{BlobID: "x", Name: "renamed.pdf"}})
	if out[0].BlobID != "a" || out[1].BlobID != "b" {
		t.Fatalf("got %+v", out)
	}
	// 个数相同时按顺序配对 (服务器改写了文件名)。
	out = rebindAttachments(sent, []Attachment{{BlobID: "x", Name: "=?utf-8?a"}, {BlobID: "y", Name: "=?utf-8?b"}})
	if out[0].BlobID != "x" || out[1].BlobID != "y" {
		t.Fatalf("got %+v", out)
	}
	// 改了名、顺序也不同 (类型对不上): 不按顺序硬配, 保持原样。
	typed := []Attachment{{BlobID: "a", Name: "a.pdf", Type: "application/pdf"}, {BlobID: "b", Name: "b.png", Type: "image/png"}}
	out = rebindAttachments(typed, []Attachment{{BlobID: "x", Name: "=?utf-8?b", Type: "image/png"}, {BlobID: "y", Name: "=?utf-8?a", Type: "application/pdf"}})
	if out[0].BlobID != "a" || out[1].BlobID != "b" {
		t.Fatalf("swapped parts must not be paired by order, got %+v", out)
	}
}

// 保存续编的草稿: 新草稿建好后读回它的部件, 返回指向新草稿的引用, 再删旧草稿
// (czlmail-gateway 上旧草稿的部件 blob 随它一起消失)。
func TestSaveDraftReturnsNewAttachmentBlobs(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MethodCalls [][3]json.RawMessage `json:"methodCalls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		var responses []any
		for _, mc := range req.MethodCalls {
			var name, callID string
			var args map[string]any
			json.Unmarshal(mc[0], &name)
			json.Unmarshal(mc[1], &args)
			json.Unmarshal(mc[2], &callID)
			out := map[string]any{"accountId": "acc", "newState": "2", "state": "2"}
			switch {
			case name == "Email/set" && args["create"] != nil:
				calls = append(calls, "create")
				out["created"] = map[string]any{"draft": map[string]any{"id": "NEW", "blobId": "mNEW", "threadId": "T", "size": 10}}
			case name == "Email/set" && args["destroy"] != nil:
				calls = append(calls, "destroy")
				out["destroyed"] = args["destroy"]
			case name == "Email/get":
				calls = append(calls, "get")
				out["list"] = []any{map[string]any{"id": "NEW", "attachments": []any{
					map[string]any{"blobId": "pNEW.2", "type": "application/pdf", "name": "faktura.pdf"},
					map[string]any{"blobId": "pNEW.3", "type": "application/xml", "name": "faktura.isdoc"},
				}}}
			default:
				calls = append(calls, name)
				out = map[string]any{"accountId": "acc", "state": "1", "list": []any{}, "ids": []any{}, "queryState": "1", "newState": "1", "oldState": "1", "changed": []any{}, "created": []any{}, "updated": []any{}, "destroyed": []any{}}
			}
			responses = append(responses, []any{name, out, callID})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"methodResponses": responses, "sessionState": "s"})
	}))
	defer server.Close()

	d := Draft{AccountID: "acc", IdentityID: "id", FromEmail: "a@example.cz", Subject: "x", TextBody: "ahoj",
		ReplaceDraftID: "OLD", Attachments: []Attachment{
			{BlobID: "pOLD.2", Type: "application/pdf", Name: "faktura.pdf"},
			{BlobID: "pOLD.3", Type: "application/xml", Name: "faktura.isdoc"},
		}}
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := signSyncer(server)
	s.store = st
	saved, _ := s.SaveDraft(context.Background(), d, "drafts") // 随后的 SyncMail 对假服务器可以失败
	if saved.ID != "NEW" {
		t.Fatalf("id %q", saved.ID)
	}
	if len(saved.Attachments) != 2 || saved.Attachments[0].BlobID != "pNEW.2" || saved.Attachments[1].BlobID != "pNEW.3" {
		t.Fatalf("attachments %+v", saved.Attachments)
	}
	if got := strings.Join(calls, ","); !strings.HasPrefix(got, "create,get,destroy") {
		t.Fatalf("calls %s: the new parts must be read before the old draft is destroyed", got)
	}
}
