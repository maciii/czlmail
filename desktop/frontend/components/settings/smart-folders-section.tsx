"use client";

import { t } from "@/lib/i18n";
import { useI18n } from "@/components/locale-provider";

import { useEffect, useRef, useState, type DragEvent, type KeyboardEvent } from "react";
import { FolderSearch, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, SectionTitle } from "@/components/settings/ui";
import { DragHandle, DropLine } from "@/components/settings/reorder";
import { cn } from "@/lib/utils";
import { SmartFolderDialog } from "@/components/smart-folder-dialog";
import { accountName, useAccountLooks } from "@/lib/account-looks";
import { Events, api, errorMessage, onEvent, type Account } from "@/lib/api";
import { tagName, useMessageMarksState } from "@/lib/message-tags";
import { emptySmartFolder, smartFolderSummary, useSmartFolders, type SmartFolder } from "@/lib/smart-folders";

/**
 * 设置 → 筛选文件夹: 列出、新建、编辑、删除。文件夹本身显示在邮件侧栏「全部账号」的「未读」下面,
 * 侧栏里右键也能编辑; 定义经个人网盘同步到别的设备。左边的手柄拖动 (或聚焦后按 ↑↓) 调整侧栏里的先后。
 */
export function SmartFoldersSection({ active }: { active: boolean }) {
  useI18n();
  const { folders: loaded } = useSmartFolders();
  const { folders, dragging, dropAt, handle, target } = useSmartFolderOrder(loaded);
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
          <div key={f.id} className={cn("relative flex items-center gap-2", dragging === f.id && "opacity-50")} {...target(f.id)}>
            <DropLine at={dropAt(f.id)} />
            <DragHandle {...handle(f.id, folders.length > 1)} className="-mr-1" />
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

const SMART_DRAG_TYPE = "application/x-czlmail-smart-folder";

/**
 * 列表的拖动与方向键。保存排队 (同 设置 → 账户 的邮箱先后): 同时只发一次, 连按方向键时只补发最后的顺序;
 * 保存完、重读到新定义之前先按刚排的显示。
 */
function useSmartFolderOrder(loaded: SmartFolder[]) {
  const [local, setLocal] = useState<{ base: SmartFolder[]; ids: string[] } | null>(null);
  const [saving, setSaving] = useState(false);
  const [dragging, setDragging] = useState<string | null>(null);
  const [over, setOver] = useState<{ id: string; after: boolean } | null>(null);
  const queue = useRef<{ running: boolean; next: string[] | null }>({ running: false, next: null });

  const sameSet = local && local.ids.length === loaded.length && loaded.every((f) => local.ids.includes(f.id));
  const folders =
    local && sameSet && (local.base === loaded || saving)
      ? local.ids.map((id) => loaded.find((f) => f.id === id)!).filter(Boolean)
      : loaded;

  async function flush() {
    const q = queue.current;
    if (q.running) return;
    q.running = true;
    setSaving(true);
    try {
      while (q.next) {
        const ids = q.next;
        q.next = null;
        await api.setSmartFolderOrder(ids);
      }
    } catch (err) {
      q.next = null;
      setLocal(null);
      toast.error(errorMessage(err));
    } finally {
      q.running = false;
      setSaving(false);
    }
  }

  function move(id: string, to: string, after: boolean) {
    if (id === to) return;
    const rest = folders.map((f) => f.id).filter((x) => x !== id);
    rest.splice(rest.indexOf(to) + (after ? 1 : 0), 0, id);
    if (rest.every((x, i) => x === folders[i]?.id)) return;
    setLocal({ base: loaded, ids: rest });
    queue.current.next = rest;
    void flush();
  }

  return {
    folders,
    dragging,
    dropAt: (id: string) => (over?.id === id ? (over.after ? "bottom" : "top") : null),
    handle: (id: string, enabled: boolean) => ({
      enabled,
      onDragStart: (e: DragEvent) => {
        e.dataTransfer.effectAllowed = "move";
        e.dataTransfer.setData(SMART_DRAG_TYPE, id);
        const row = (e.currentTarget as HTMLElement).closest<HTMLElement>("[data-order-row]");
        if (row) e.dataTransfer.setDragImage(row, 12, 12);
        setDragging(id);
      },
      onDragEnd: () => {
        setDragging(null);
        setOver(null);
      },
      onKeyDown: (e: KeyboardEvent) => {
        if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
        e.preventDefault();
        const i = folders.findIndex((f) => f.id === id);
        const to = folders[i + (e.key === "ArrowUp" ? -1 : 1)];
        if (to) move(id, to.id, e.key === "ArrowDown");
      },
    }),
    target: (id: string) => ({
      "data-order-row": "",
      onDragOver: (e: DragEvent) => {
        if (!dragging || dragging === id || !e.dataTransfer.types.includes(SMART_DRAG_TYPE)) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
        const after = e.clientY > rect.top + rect.height / 2;
        if (over?.id !== id || over.after !== after) setOver({ id, after });
      },
      onDragLeave: (e: DragEvent) => {
        if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node | null) && over?.id === id) setOver(null);
      },
      onDrop: (e: DragEvent) => {
        if (!dragging || dragging === id) return;
        e.preventDefault();
        const after = over?.id === id ? over.after : false;
        setDragging(null);
        setOver(null);
        move(dragging, id, after);
      },
    }),
  };
}
