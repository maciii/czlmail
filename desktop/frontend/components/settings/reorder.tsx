"use client";

import { t } from "@/lib/i18n";
import { useI18n } from "@/components/locale-provider";

import { type DragEvent, type KeyboardEvent } from "react";
import { GripVertical } from "lucide-react";

import { cn } from "@/lib/utils";

// 设置页里拖动调整先后 (可访问的邮箱、筛选文件夹) 共用的手柄与放下位置线。

/** 拖动的手柄; 不能动 (同类只有它一个) 时只占位, 名字照样对齐。 */
export function DragHandle({
  enabled,
  className,
  ...events
}: {
  enabled: boolean;
  className?: string;
  onDragStart: (e: DragEvent) => void;
  onDragEnd: () => void;
  onKeyDown: (e: KeyboardEvent) => void;
}) {
  useI18n();
  if (!enabled) return <span aria-hidden className={cn("w-5 shrink-0", className)} />;
  return (
    <button
      type="button"
      draggable
      aria-label={t("拖动或按 ↑↓ 调整顺序")}
      title={t("拖动或按 ↑↓ 调整顺序")}
      className={cn(
        "text-muted-foreground/60 hover:text-foreground hover:bg-secondary flex h-6 w-5 shrink-0 cursor-grab items-center justify-center rounded-sm active:cursor-grabbing",
        className,
      )}
      {...events}
    >
      <GripVertical className="size-3.5" />
    </button>
  );
}

/** 放下位置的细线。 */
export function DropLine({ at }: { at: "top" | "bottom" | null }) {
  if (!at) return null;
  return <span aria-hidden className={cn("bg-primary pointer-events-none absolute inset-x-2 z-10 h-0.5 rounded-full", at === "top" ? "-top-px" : "-bottom-px")} />;
}
