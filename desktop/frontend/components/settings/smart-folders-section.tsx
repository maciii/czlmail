"use client";

import { t } from "@/lib/i18n";
import { useI18n } from "@/components/locale-provider";

import { useEffect, useState } from "react";
import { FolderSearch, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, SectionTitle } from "@/components/settings/ui";
import { SmartFolderDialog } from "@/components/smart-folder-dialog";
import { accountName, useAccountLooks } from "@/lib/account-looks";
import { Events, api, errorMessage, onEvent, type Account } from "@/lib/api";
import { tagName, useMessageMarksState } from "@/lib/message-tags";
import { emptySmartFolder, smartFolderSummary, useSmartFolders, type SmartFolder } from "@/lib/smart-folders";

/**
 * 设置 → 筛选文件夹: 列出、新建、编辑、删除。文件夹本身显示在邮件侧栏「全部账号」的「未读」下面,
 * 侧栏里右键也能编辑; 定义经个人网盘同步到别的设备。
 */
export function SmartFoldersSection({ active }: { active: boolean }) {
  useI18n();
  const { folders } = useSmartFolders();
  const marks = useMessageMarksState();
  const looks = useAccountLooks();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [editing, setEditing] = useState<SmartFolder | null>(null);

  useEffect(() => {
    if (!active) return;
    const load = () =>
      api
        .listAccounts()
        .then((l) => setAccounts(l ?? []))
        .catch(() => {});
    load();
    return onEvent(Events.accountsChanged, load);
  }, [active]);

  const nameOf = (id: string) => {
    const acc = accounts.find((a) => a.id === id);
    return acc ? accountName(acc, looks) : id;
  };

  async function remove(f: SmartFolder) {
    if (!window.confirm(t("删除筛选文件夹「{0}」？其中的邮件不受影响。", f.name))) return;
    try {
      await api.deleteSmartFolder(f.id);
    } catch (err) {
      toast.error(errorMessage(err));
    }
  }

  return (
    <>
      <SectionTitle
        title={t("筛选文件夹")}
        desc={t("按条件（未读、待办旗标、标签、附件、发件人、账号）汇总各账号的邮件，显示在邮件侧栏「全部账号」的「未读」下面，并通过个人网盘同步到你的其它设备。")}
      />
      <Card className="gap-2">
        {folders.length === 0 && <p className="text-muted-foreground text-sm">{t("还没有筛选文件夹")}</p>}
        {folders.map((f) => (
          <div key={f.id} className="flex items-center gap-2">
            <FolderSearch className="text-muted-foreground size-4 shrink-0" />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">{f.name}</p>
              <p className="text-muted-foreground truncate text-xs">
                {smartFolderSummary(f, (kw) => tagName(kw, marks.byKeyword), nameOf)}
              </p>
            </div>
            <Button variant="ghost" size="icon" className="size-7" aria-label={t("编辑")} title={t("编辑")} onClick={() => setEditing(f)}>
              <Pencil className="size-4" />
            </Button>
            <Button variant="ghost" size="icon" className="size-7" aria-label={t("删除")} title={t("删除")} onClick={() => remove(f)}>
              <Trash2 className="size-4" />
            </Button>
          </div>
        ))}
        <div>
          <Button size="sm" variant="outline" onClick={() => setEditing(emptySmartFolder())}>
            <Plus className="size-4" />
            {t("新建筛选文件夹")}
          </Button>
        </div>
      </Card>
      <SmartFolderDialog folder={editing} accounts={accounts} onClose={() => setEditing(null)} />
    </>
  );
}
