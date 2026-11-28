package main

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 筛选文件夹 (smart_folders.go) 的工具。筛选文件夹只是一组条件, 增删改都不动邮件, 所以都直接可用。

// smartFolderFields 是新建与修改共用的条件; 修改时没给的条件保持原样。
type smartFolderFields struct {
	Name       *string  `json:"name,omitempty" jsonschema:"名称, 显示在侧栏"`
	Unread     *bool    `json:"unread,omitempty" jsonschema:"只列未读"`
	Flag       *string  `json:"flag,omitempty" jsonschema:"待办旗标: flagged(待办/星标)、completed(已完成), 空串不限"`
	Tags       []string `json:"tags,omitempty" jsonschema:"标签关键字 (见 list_mailboxes 的 labels), 有其中任意一个即可; 修改时给空数组清掉"`
	Attachment *bool    `json:"attachment,omitempty" jsonschema:"只列有附件的"`
	From       *string  `json:"from,omitempty" jsonschema:"发件人名字或地址里包含的文字"`
	Accounts   []string `json:"accounts,omitempty" jsonschema:"只看这些账号 (list_accounts 的 id); 省略或空数组为并入「全部账号」的所有账号"`
	Scope      *string  `json:"scope,omitempty" jsonschema:"在哪些文件夹里找: 空串同「未读」(不含已发送、草稿、垃圾邮件、已删除), inbox 只在收件箱, all 除垃圾邮件与已删除外的全部"`
}

func (in smartFolderFields) apply(f *SmartFolder) {
	if in.Name != nil {
		f.Name = *in.Name
	}
	if in.Unread != nil {
		f.Unread = *in.Unread
	}
	if in.Flag != nil {
		f.Flag = *in.Flag
	}
	if in.Tags != nil {
		f.Tags = in.Tags
	}
	if in.Attachment != nil {
		f.Attachment = *in.Attachment
	}
	if in.From != nil {
		f.From = *in.From
	}
	if in.Accounts != nil {
		f.Accounts = in.Accounts
	}
	if in.Scope != nil {
		f.Scope = *in.Scope
	}
}

func (a *App) addSmartFolderTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_smart_folders",
		Description: "列出筛选文件夹 (用户按条件组合的跨账号视图, 侧栏「全部账号」里) 及其条件、本机缓存里的邮件数与未读数。",
		Annotations: readOnlyTool(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		folders, err := a.GetSmartFolders()
		if err != nil {
			return nil, nil, err
		}
		counts, err := a.SmartFolderCounts()
		if err != nil {
			return nil, nil, err
		}
		out := make([]map[string]any, len(folders))
		for i, f := range folders {
			c := counts[f.ID]
			out[i] = map[string]any{"folder": f, "total": c.Total, "unread": c.Unread}
		}
		return nil, map[string]any{"smartFolders": out}, nil
	})

	type listIn struct {
		ID        string `json:"id" jsonschema:"筛选文件夹 id (list_smart_folders)"`
		AccountID string `json:"accountId,omitempty" jsonschema:"只看其中一个账号; 省略时合并筛选文件夹里的所有账号"`
		Limit     int    `json:"limit,omitempty" jsonschema:"条数, 默认 20, 最多 100"`
		Offset    int    `json:"offset,omitempty"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_smart_folder_emails",
		Description: "按时间倒序列出筛选文件夹里的邮件摘要, 每封带 accountId。条件里有旗标或标签时先让服务器补齐本机没有的旧邮件, " +
			"其它条件只看本机缓存。",
		Annotations: readOnlyTool(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, any, error) {
		id := strings.TrimSpace(in.ID)
		if id == "" {
			return nil, nil, errors.New("id is empty")
		}
		if in.Offset <= 0 {
			if err := a.RefreshSmartFolder(id); err != nil {
				return nil, nil, err
			}
		}
		view := smartViewPrefix + id
		if in.AccountID != "" {
			view += viewAccountSeparator + in.AccountID
		}
		list, err := a.ListUnified(view, clamp(in.Limit, 20, 100), in.Offset)
		return nil, map[string]any{"id": id, "emails": summarize(list, nil)}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_smart_folder",
		Description: "新建筛选文件夹: 各条件同时满足, 没给的条件不筛选。只是一个视图, 不移动邮件。",
		Annotations: writeTool(false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in smartFolderFields) (*mcp.CallToolResult, any, error) {
		var f SmartFolder
		in.apply(&f)
		saved, err := a.SaveSmartFolder(f)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"folder": saved}, nil
	})

	type updateIn struct {
		ID string `json:"id" jsonschema:"筛选文件夹 id (list_smart_folders)"`
		smartFolderFields
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_smart_folder",
		Description: "修改筛选文件夹的名称或条件, 没给的保持原样。不影响邮件。",
		Annotations: writeTool(false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in updateIn) (*mcp.CallToolResult, any, error) {
		f, err := a.mcpSmartFolder(in.ID)
		if err != nil {
			return nil, nil, err
		}
		in.apply(&f)
		saved, err := a.SaveSmartFolder(f)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"folder": saved}, nil
	})

	type deleteIn struct {
		ID string `json:"id" jsonschema:"筛选文件夹 id (list_smart_folders)"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_smart_folder",
		Description: "删除筛选文件夹 (只删条件, 里面列出的邮件不受影响)。",
		Annotations: writeTool(true, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deleteIn) (*mcp.CallToolResult, any, error) {
		if _, err := a.mcpSmartFolder(in.ID); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"ok": true}, a.DeleteSmartFolder(in.ID)
	})
}

// mcpSmartFolder 按 id 取一个筛选文件夹 (本机账号 id), 不存在时报 2253。
func (a *App) mcpSmartFolder(id string) (SmartFolder, error) {
	st, err := a.currentStore()
	if err != nil {
		return SmartFolder{}, err
	}
	return a.smartFolderByID(st, strings.TrimSpace(id))
}
