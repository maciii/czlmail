"use client";

import { t } from "@/lib/i18n";
import { useI18n } from "@/components/locale-provider";

import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";

import { OptionSelect } from "@/components/option-select";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { accountColor, accountName, useAccountLooks } from "@/lib/account-looks";
import { api, errorMessage, type Account } from "@/lib/api";
import { tagOptions, useMessageMarksState } from "@/lib/message-tags";
import { smartScopeOptions, type SmartFolder } from "@/lib/smart-folders";
import { cn } from "@/lib/utils";

interface Props {
  /** 要编辑的筛选文件夹; id 为空是新建, null 时不显示。 */
  folder: SmartFolder | null;
  accounts: Account[];
  onClose: () => void;
  /** 保存成功, 带上保存后的定义 (新建的有了 id)。 */
  onSaved?: (folder: SmartFolder) => void;
}

/** 新建与编辑筛选文件夹: 名字与条件, 各条件同时满足。 */
export function SmartFolderDialog(props: Props) {
  // 每次打开从传进来的定义重新开始编辑。
  return props.folder ? <SmartFolderForm key={props.folder.id || "new"} {...props} folder={props.folder} /> : null;
}

function SmartFolderForm({ folder, accounts, onClose, onSaved }: Props & { folder: SmartFolder }) {
  useI18n();
  const marks = useMessageMarksState();
  const looks = useAccountLooks();
  const [draft, setDraft] = useState<SmartFolder>(
    () => ({ ...folder, tags: [...(folder.tags ?? [])], accounts: [...(folder.accounts ?? [])] }) as SmartFolder,
  );
  const [busy, setBusy] = useState(false);
  // 邮件上出现过、但没有名字的关键字也能选 (同 设置 → 标签)。
  const [used, setUsed] = useState<string[]>([]);

  useEffect(() => {
    let cancelled = false;
    Promise.all(accounts.map((a) => api.listLabels(a.id).catch(() => [] as string[])))
      .then((lists) => {
        if (!cancelled) setUsed([...new Set(lists.flatMap((l) => l ?? []))].sort());
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [accounts]);


  const set = (patch: Partial<SmartFolder>) => setDraft((cur) => ({ ...cur, ...patch }) as SmartFolder);
  const toggleIn = (list: string[], v: string, on: boolean) => (on ? [...list.filter((x) => x !== v), v] : list.filter((x) => x !== v));
  const tags = tagOptions(marks, draft.tags ?? [], used);

  async function submit() {
    if (busy || !draft.name.trim()) return;
    setBusy(true);
    try {
      const saved = await api.saveSmartFolder(draft);
      onSaved?.(saved);
      onClose();
    } catch (err) {
      toast.error(errorMessage(err));
      setBusy(false);
    }
  }

  const flagOptions: [string, string][] = [
    ["", t("不限")],
    ["flagged", t("待办（带旗标）")],
    ["completed", t("待办已完成")],
  ];

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{folder.id ? t("编辑筛选文件夹") : t("新建筛选文件夹")}</DialogTitle>
          <DialogDescription>
            {t("显示在「全部账号」的「未读」下面，列出同时满足下列条件的邮件。只查找本机已缓存的邮件。")}
          </DialogDescription>
        </DialogHeader>

        <form
          className="flex min-w-0 flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="smart-name">{t("名称")}</Label>
            <Input
              id="smart-name"
              autoFocus
              value={draft.name}
              onChange={(e) => set({ name: e.target.value })}
              placeholder={t("例如：待处理")}
            />
          </div>

          <div className="flex flex-col gap-3 text-sm">
            <label className="flex items-center gap-2">
              <Checkbox checked={draft.unread} onCheckedChange={(v) => set({ unread: v === true })} />
              {t("只看未读")}
            </label>
            <label className="flex items-center gap-2">
              <Checkbox checked={draft.attachment} onCheckedChange={(v) => set({ attachment: v === true })} />
              {t("只看有附件的")}
            </label>
            <div className="flex items-center gap-3">
              <Label htmlFor="smart-flag" className="w-36 shrink-0 font-normal">
                {t("待办旗标")}
              </Label>
              <OptionSelect id="smart-flag" value={draft.flag} onChange={(flag) => set({ flag })} options={flagOptions} className="flex-1" />
            </div>
            <div className="flex items-center gap-3">
              <Label htmlFor="smart-from" className="w-36 shrink-0 font-normal">
                {t("发件人包含")}
              </Label>
              <Input
                id="smart-from"
                value={draft.from}
                onChange={(e) => set({ from: e.target.value })}
                placeholder={t("名字或地址的一部分")}
                className="h-8 flex-1"
              />
            </div>
            <div className="flex items-center gap-3">
              <Label htmlFor="smart-scope" className="w-36 shrink-0 font-normal">
                {t("查找范围")}
              </Label>
              <OptionSelect
                id="smart-scope"
                value={draft.scope}
                onChange={(scope) => set({ scope })}
                options={smartScopeOptions()}
                className="flex-1"
              />
            </div>
          </div>

          <fieldset className="flex flex-col gap-2">
            <legend className="text-sm font-medium">{t("标签")}</legend>
            <p className="text-muted-foreground text-xs">{t("有所选标签中的任意一个即可；都不选表示不限标签。")}</p>
            {tags.length === 0 ? (
              <p className="text-muted-foreground text-xs">{t("还没有标签")}</p>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {tags.map((g) => (
                  <button
                    key={g.keyword}
                    type="button"
                    aria-pressed={g.on}
                    onClick={() => set({ tags: toggleIn(draft.tags ?? [], g.keyword, !g.on) })}
                    className={cn(
                      "flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs transition-colors",
                      g.on ? "border-foreground/40 bg-secondary font-medium" : "text-muted-foreground hover:bg-secondary",
                    )}
                  >
                    <span
                      aria-hidden
                      className={cn("size-2 rounded-full", !g.color && "border-muted-foreground/50 border")}
                      style={g.color ? { backgroundColor: g.color } : undefined}
                    />
                    {g.name}
                  </button>
                ))}
              </div>
            )}
          </fieldset>

          {accounts.length > 1 && (
            <fieldset className="flex flex-col gap-2">
              <legend className="text-sm font-medium">{t("账号")}</legend>
              <p className="text-muted-foreground text-xs">{t("都不选表示并入「全部账号」的所有账号。")}</p>
              <div className="flex flex-col gap-1.5 text-sm">
                {accounts.map((acc) => {
                  const color = accountColor(acc.id, looks);
                  return (
                    <label key={acc.id} className="flex min-w-0 items-center gap-2">
                      <Checkbox
                        checked={(draft.accounts ?? []).includes(acc.id)}
                        onCheckedChange={(v) => set({ accounts: toggleIn(draft.accounts ?? [], acc.id, v === true) })}
                      />
                      <span
                        aria-hidden
                        className={cn("size-2 shrink-0 rounded-full", !color && "border-muted-foreground/50 border")}
                        style={color ? { backgroundColor: color } : undefined}
                      />
                      <span className="min-w-0 truncate" title={acc.name}>
                        {accountName(acc, looks)}
                      </span>
                    </label>
                  );
                })}
                {draft.accountsElsewhere > 0 && (
                  <p className="text-muted-foreground text-xs">
                    {t("另选了其它设备上的 {0} 个账号，保存时保留。", draft.accountsElsewhere)}
                  </p>
                )}
              </div>
            </fieldset>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("取消")}
            </Button>
            <Button type="submit" disabled={busy || !draft.name.trim()}>
              {busy && <Loader2 className="size-4 animate-spin" />}
              {t("保存")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
