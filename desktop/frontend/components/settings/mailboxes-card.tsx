"use client";

import { t } from "@/lib/i18n";
import { useI18n } from "@/components/locale-provider";

import { useRef, useState, type DragEvent, type KeyboardEvent } from "react";
import { toast } from "sonner";
import { Server } from "lucide-react";

import { Card, SectionTitle } from "@/components/settings/ui";
import { DragHandle, DropLine } from "@/components/settings/reorder";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ACCOUNT_COLORS, accountColorValue, signInLookKey, useAccountLooksState, type AccountLooks } from "@/lib/account-looks";
import { api, errorMessage, type Account } from "@/lib/api";
import { cn } from "@/lib/utils";
import { main } from "@/wailsjs/go/models";

/**
 * 设置 → 账户 → 可访问的邮箱: 每个邮箱一行, 左边的圆点挑颜色, 右边起名字、决定共享邮箱是否并入「全部账号」。
 * 多个登录时按登录分组 (不同登录里可能有同名邮箱), 附加登录的组头可以给它在侧栏里的那一节起标题。
 * 名字与颜色经个人网盘同步到别的设备, 服务器上的账号名不变。
 * 左边的手柄拖动 (或聚焦后按 ↑↓) 调整先后: 登录之间整组移动, 邮箱在本登录里、个人与共享各自移动;
 * 侧栏、「全部账号」、写信的发件身份按这个顺序 (后端 ListAccounts 排好), 同样同步到别的设备。
 */
export function MailboxesCard({
  accounts,
  excluded,
  onToggleUnified,
  disabled,
}: {
  accounts: Account[];
  /** 不并入「全部账号」的共享邮箱。 */
  excluded: Set<string>;
  onToggleUnified: (accountId: string, on: boolean) => void;
  disabled: boolean;
}) {
  useI18n();
  const { looks, reload } = useAccountLooksState();
  // 保存并重读完之前先按刚改的显示, 免得颜色点了要等一拍才变。
  const [pending, setPending] = useState<AccountLooks>({});
  const shown = { ...looks, ...pending };
  const { list, dragging, dropAt, handle, target } = useAccountOrder(accounts);

  async function update(key: string, patch: { name?: string; color?: string }) {
    const next = main.AccountLook.createFrom({ ...shown[key], ...patch });
    setPending((p) => ({ ...p, [key]: next }));
    try {
      await api.setAccountLook(key, next);
      await reload();
    } catch (err) {
      toast.error(errorMessage(err));
    }
    setPending((p) => {
      const rest = { ...p };
      delete rest[key];
      return rest;
    });
  }

  const groups = groupBySignIn(list);
  const grouped = groups.length > 1;

  return (
    <>
      <SectionTitle
        title={t("可访问的邮箱")}
        desc={
          <>
            {t("由服务器按你的权限共享，在邮件侧栏的「共享」下显示，新共享的邮箱会自动出现。开关决定是否并入「全部账号」；关闭后仍可在「共享」下单独查看，统一搜索照常包含它。")}
            {t("圆点与名称是你给邮箱挑的颜色和起的名字，用在侧栏与「全部账号」里，经个人网盘同步到你的其它设备。")}
            {t("拖动左边的手柄调整顺序，侧栏与「全部账号」按这个顺序显示。")}
          </>
        }
        small
      />
      <Card className="gap-0 p-0">
        {groups.map((g, gi) => (
          <div
            key={g.signIn}
            className={cn("relative", gi > 0 && "border-border border-t", dragging?.signIn === g.signIn && !dragging.id && "opacity-50")}
            {...target({ signIn: g.signIn })}
          >
            <DropLine at={dropAt({ signIn: g.signIn })} />
            {grouped && (
              <div className="text-muted-foreground flex items-center gap-2 pt-2.5 pr-4 pb-1 pl-1 text-xs">
                <DragHandle {...handle({ signIn: g.signIn }, groups.length > 1)} />
                <Server className="size-3.5 shrink-0" />
                <span className="min-w-0 flex-1 truncate">{g.host}</span>
                {/* 附加登录各占侧栏一节, 标题默认是服务器地址; 主登录那节叫「文件夹」, 不用起名。 */}
                {g.signIn && (
                  <NameInput
                    value={shown[signInLookKey(g.signIn)]?.name ?? ""}
                    placeholder={t("侧栏标题")}
                    onCommit={(name) => update(signInLookKey(g.signIn), { name })}
                  />
                )}
                <span aria-hidden className="w-[4.75rem] shrink-0" />
              </div>
            )}
            {g.list.map((a, i) => (
              <div
                key={a.id}
                className={cn(
                  "relative flex items-center gap-3 py-2 pr-4 pl-1 text-sm",
                  (i > 0 || grouped) && "border-border border-t",
                  dragging?.id === a.id && "opacity-50",
                )}
                {...target({ signIn: g.signIn, id: a.id, personal: a.isPersonal })}
              >
                <DropLine at={dropAt({ signIn: g.signIn, id: a.id })} />
                <DragHandle
                  {...handle(
                    { signIn: g.signIn, id: a.id, personal: a.isPersonal },
                    g.list.filter((b) => b.isPersonal === a.isPersonal).length > 1,
                  )}
                  className="-mr-2"
                />
                <ColorPicker value={shown[a.id]?.color ?? ""} onChange={(color) => update(a.id, { color })} />
                <span className="min-w-0 flex-1 truncate" title={a.name}>
                  {a.name}
                </span>
                <NameInput
                  value={shown[a.id]?.name ?? ""}
                  placeholder={t("显示名称")}
                  onCommit={(name) => update(a.id, { name })}
                />
                <span className="text-muted-foreground w-7 shrink-0 text-xs">{a.isPersonal ? t("个人") : t("共享")}</span>
                {a.isPersonal ? (
                  <span aria-hidden className="w-8 shrink-0" />
                ) : (
                  <Switch
                    aria-label={t("并入全部账号")}
                    checked={!excluded.has(a.id)}
                    disabled={disabled}
                    onCheckedChange={(on) => onToggleUnified(a.id, on)}
                  />
                )}
              </div>
            ))}
          </div>
        ))}
      </Card>
    </>
  );
}

/** 失焦或回车保存, Esc 放弃; 外面的值变了 (保存完、别的设备同步来) 时草稿跟着换。设置 → 标签也用。 */
export function NameInput({
  value,
  placeholder,
  onCommit,
}: {
  value: string;
  placeholder: string;
  onCommit: (name: string) => void;
}) {
  const [draft, setDraft] = useState(value);
  const [seen, setSeen] = useState(value);
  if (value !== seen) {
    setSeen(value);
    setDraft(value);
  }
  return (
    <Input
      value={draft}
      placeholder={placeholder}
      aria-label={placeholder}
      className="h-7 w-44 shrink-0 text-sm"
      onChange={(e) => setDraft(e.target.value)}
      onBlur={() => {
        if (draft.trim() !== value) onCommit(draft.trim());
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter") e.currentTarget.blur();
        if (e.key === "Escape") setDraft(value);
      }}
    />
  );
}

export function ColorPicker({ value, onChange }: { value: string; onChange: (key: string) => void }) {
  useI18n();
  const [open, setOpen] = useState(false);
  const current = accountColorValue(value);
  const pick = (key: string) => {
    setOpen(false);
    if (key !== value) onChange(key);
  };
  const swatches = [{ key: "", value: undefined, label: () => t("无颜色") }, ...ACCOUNT_COLORS];
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={t("颜色")}
          title={t("颜色")}
          className="hover:bg-secondary flex size-6 shrink-0 items-center justify-center rounded-full"
        >
          <Swatch color={current} />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-auto flex-row gap-1 p-1.5">
        {swatches.map((c) => (
          <button
            key={c.key}
            type="button"
            onClick={() => pick(c.key)}
            aria-label={c.label()}
            title={c.label()}
            aria-pressed={value === c.key}
            className={cn(
              "hover:bg-secondary flex size-7 items-center justify-center rounded-full",
              value === c.key && "bg-secondary",
            )}
          >
            <Swatch color={c.value} selected={value === c.key} />
          </button>
        ))}
      </PopoverContent>
    </Popover>
  );
}

/** 色块: 没颜色时是一个空心圆。选中时外圈一道细环。 */
function Swatch({ color, selected }: { color?: string; selected?: boolean }) {
  return (
    <span
      aria-hidden
      className={cn(
        "size-3.5 rounded-full",
        !color && "border-muted-foreground/50 border border-dashed",
        selected && "ring-foreground/30 ring-offset-background ring-1 ring-offset-2",
      )}
      style={color ? { backgroundColor: color } : undefined}
    />
  );
}

/* ---------------- 先后 ---------------- */

/** 拖动的东西: 整个登录 (只有 signIn), 或登录里的一个邮箱。 */
type OrderItem = { signIn: string; id?: string; personal?: boolean };

/** 按登录分组, 组的先后与组内的先后都照 list。 */
function groupBySignIn(list: Account[]) {
  return [...new Set(list.map((a) => a.signIn))].map((signIn) => {
    const own = list.filter((a) => a.signIn === signIn);
    return { signIn, host: own[0].signInHost || signIn, list: own };
  });
}

/** 两项能不能互换位置: 登录只与登录, 邮箱只在本登录、个人与个人、共享与共享之间。 */
function compatible(a: OrderItem, b: OrderItem) {
  if (!a.id || !b.id) return !a.id && !b.id && a.signIn !== b.signIn;
  return a.id !== b.id && a.signIn === b.signIn && a.personal === b.personal;
}

/** 把 item 挪到 to 的前面 (after 时后面), 返回新的账号顺序。 */
function reorder(list: Account[], item: OrderItem, to: OrderItem, after: boolean): Account[] {
  if (!item.id) {
    const groups = groupBySignIn(list).filter((g) => g.signIn !== item.signIn);
    const moved = list.filter((a) => a.signIn === item.signIn);
    const at = groups.findIndex((g) => g.signIn === to.signIn) + (after ? 1 : 0);
    const out = groups.map((g) => g.list);
    out.splice(at, 0, moved);
    return out.flat();
  }
  const moved = list.find((a) => a.id === item.id);
  if (!moved) return list;
  const rest = list.filter((a) => a.id !== item.id);
  const at = rest.findIndex((a) => a.id === to.id) + (after ? 1 : 0);
  rest.splice(at, 0, moved);
  return rest;
}

/** 两份列表的账号一样 (不论先后): 账号增减后本地排的顺序作废。 */
function sameAccounts(a: Account[], b: Account[]) {
  if (a.length !== b.length) return false;
  const ids = new Set(a.map((x) => x.id));
  return b.every((x) => ids.has(x.id));
}

/** 后端要的键: 每个登录 "signin:" + 前缀, 后面跟它的邮箱 id。 */
function orderKeys(list: Account[]): string[] {
  return groupBySignIn(list).flatMap((g) => [signInLookKey(g.signIn), ...g.list.map((a) => a.id)]);
}

/**
 * 卡片里的拖动与方向键。保存后后端发 accounts-changed, 设置页重读账号 (accounts 变了) 前先按刚排的显示。
 * 拖动用 HTML5 拖放, 类型是自有的, 别处拖进来的东西 (邮件、文件) 不当成邮箱。
 */
function useAccountOrder(accounts: Account[]) {
  const [local, setLocal] = useState<{ base: Account[]; list: Account[] } | null>(null);
  const [dragging, setDragging] = useState<OrderItem | null>(null);
  const [over, setOver] = useState<{ item: OrderItem; after: boolean } | null>(null);
  // 保存排队: 同时只发一次, 连按方向键时只把最后的顺序补发出去。绑定调用是并发执行的,
  // 一起发出去时旧顺序可能最后落库; 中途收到 accounts-changed 重读的列表也不盖掉还没存完的顺序。
  const [saving, setSaving] = useState(false);
  const queue = useRef<{ running: boolean; next: string[] | null }>({ running: false, next: null });
  const list = local && (local.base === accounts || saving) && sameAccounts(local.list, accounts) ? local.list : accounts;

  async function flush() {
    const q = queue.current;
    if (q.running) return;
    q.running = true;
    setSaving(true);
    try {
      while (q.next) {
        const keys = q.next;
        q.next = null;
        await api.setAccountOrder(keys);
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

  function move(item: OrderItem, to: OrderItem, after: boolean) {
    const next = reorder(list, item, to, after);
    if (next.every((a, i) => a.id === list[i]?.id)) return;
    setLocal({ base: accounts, list: next });
    queue.current.next = orderKeys(next);
    void flush();
  }

  /** 方向键: 与同类的上一项 / 下一项换位。 */
  function step(item: OrderItem, delta: -1 | 1) {
    const peers: OrderItem[] = item.id
      ? list.filter((a) => a.signIn === item.signIn && a.isPersonal === item.personal).map((a) => ({ signIn: a.signIn, id: a.id, personal: a.isPersonal }))
      : groupBySignIn(list).map((g) => ({ signIn: g.signIn }));
    const i = peers.findIndex((p) => (item.id ? p.id === item.id : p.signIn === item.signIn));
    const to = peers[i + delta];
    if (to) move(item, to, delta > 0);
  }

  const same = (a: OrderItem | null | undefined, b: OrderItem) => !!a && a.signIn === b.signIn && a.id === b.id;

  return {
    list,
    move,
    dragging,
    /** 放下的位置线画在这一项的上沿还是下沿, 不在这一项上时为 null。 */
    dropAt: (item: OrderItem) => (over && same(over.item, item) ? (over.after ? "bottom" : "top") : null),
    handle: (item: OrderItem, enabled: boolean) => ({
      enabled,
      onDragStart: (e: DragEvent) => {
        e.dataTransfer.effectAllowed = "move";
        e.dataTransfer.setData(ACCOUNT_DRAG_TYPE, item.id ?? item.signIn);
        // 拖的是整行 (或整组), 不只是手柄。
        const row = (e.currentTarget as HTMLElement).closest<HTMLElement>("[data-order-row]");
        if (row) e.dataTransfer.setDragImage(row, 12, 12);
        setDragging(item);
      },
      onDragEnd: () => {
        setDragging(null);
        setOver(null);
      },
      onKeyDown: (e: KeyboardEvent) => {
        if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
        e.preventDefault();
        step(item, e.key === "ArrowUp" ? -1 : 1);
      },
    }),
    target: (item: OrderItem) => ({
      "data-order-row": "",
      onDragOver: (e: DragEvent) => {
        if (!dragging || !e.dataTransfer.types.includes(ACCOUNT_DRAG_TYPE) || !compatible(dragging, item)) return;
        // 组里的邮箱行也在组的范围里: 拖登录时由组接住, 拖邮箱时由行接住, 不往外冒。
        e.preventDefault();
        e.stopPropagation();
        e.dataTransfer.dropEffect = "move";
        const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
        const after = e.clientY > rect.top + rect.height / 2;
        if (!same(over?.item, item) || over?.after !== after) setOver({ item, after });
      },
      onDragLeave: (e: DragEvent) => {
        if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node | null) && same(over?.item, item)) setOver(null);
      },
      onDrop: (e: DragEvent) => {
        if (!dragging || !compatible(dragging, item)) return;
        e.preventDefault();
        e.stopPropagation();
        const after = over && same(over.item, item) ? over.after : false;
        setDragging(null);
        setOver(null);
        move(dragging, item, after);
      },
    }),
  };
}

const ACCOUNT_DRAG_TYPE = "application/x-czlmail-account";
