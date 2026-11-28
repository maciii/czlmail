"use client";

import { t } from "@/lib/i18n";
import { useI18n } from "@/components/locale-provider";

import { useEffect, useState } from "react";
import type { Editor } from "@tiptap/react";
import type { Mark } from "@tiptap/pm/model";
import { TextSelection } from "@tiptap/pm/state";
import { Keyboard, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { convertDirection, convertLayout, mistypedRun, type TextRun } from "@/lib/keyboard-layout";

/**
 * 纠正正文里用错键盘布局打的字 (见 lib/keyboard-layout.ts): Ctrl+Shift+K 或工具条按钮换算选中的文字,
 * 没有选中时换算光标前打错的那几个词; 正在打的段落像是打错了时, 编辑器下方给出提示。
 * 设置 → 写信 →「捷克语键盘」为关闭时 (非捷克语界面的默认) 这些都不出现。
 */

export const KEYBOARD_FIX_SHORTCUT = "Ctrl+Shift+K";

/** 按物理键位认 Ctrl+Shift+K, 当前是哪种布局都能用。 */
export function isKeyboardFixShortcut(e: KeyboardEvent): boolean {
  return (e.ctrlKey || e.metaKey) && e.shiftKey && !e.altKey && e.code === "KeyK";
}

/** 当前段落里光标前的文字; 行内叶节点 (图片、换行) 各算一个字符。 */
function textBeforeCursor(editor: Editor): { start: number; text: string } {
  const { $from } = editor.state.selection;
  const start = $from.start();
  return { start, text: editor.state.doc.textBetween(start, $from.pos, "\n", "\n") };
}

/** 光标前打错的词在文档里的位置。 */
function mistypedRange(editor: Editor, minWords: number): TextRun | null {
  if (!editor.state.selection.empty) return null;
  const { start, text } = textBeforeCursor(editor);
  const run = mistypedRun(text, text.length, minWords);
  return run && { from: start + run.from, to: start + run.to };
}

/** 换算 from 到 to 之间的文字, 格式不变; 返回是否有改动。 */
function convertRange(editor: Editor, from: number, to: number, keepSelection: boolean): boolean {
  const { state } = editor;
  const direction = convertDirection(state.doc.textBetween(from, to, " ", " "));
  const parts: { from: number; to: number; text: string; marks: readonly Mark[] }[] = [];
  state.doc.nodesBetween(from, to, (node, pos) => {
    if (!node.isText || !node.text) return;
    const start = Math.max(from, pos);
    const end = Math.min(to, pos + node.nodeSize);
    if (start < end) parts.push({ from: start, to: end, text: node.text.slice(start - pos, end - pos), marks: node.marks });
  });
  const tr = state.tr;
  // 从后往前换, 前面各段的位置不受影响。
  for (const part of parts.reverse()) {
    const converted = convertLayout(part.text, direction);
    if (converted === part.text) continue;
    tr.replaceWith(part.from, part.to, state.schema.text(converted, part.marks));
  }
  if (!tr.docChanged) return false;
  // 选中的文字换完仍选中, 再按一次快捷键就换回去。
  if (keepSelection) tr.setSelection(TextSelection.create(tr.doc, tr.mapping.map(from, -1), tr.mapping.map(to, 1)));
  editor.view.dispatch(tr.scrollIntoView());
  return true;
}

/** 换算选中的文字; 没有选中时换算光标前打错的词。 */
export function fixKeyboardLayout(editor: Editor): boolean {
  const { from, to, empty } = editor.state.selection;
  if (!empty) return convertRange(editor, from, to, true);
  const run = mistypedRange(editor, 1);
  return run ? convertRange(editor, run.from, run.to, false) : false;
}

export function KeyboardFixButton({ editor }: { editor: Editor | null }) {
  useI18n();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="size-7"
          disabled={!editor}
          aria-label={t("修正键盘布局")}
          aria-keyshortcuts="Control+Shift+K"
          // 按下鼠标时不抢焦点, 编辑器里的选区才留得住。
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => {
            if (editor && fixKeyboardLayout(editor)) editor.commands.focus();
          }}
        >
          <Keyboard className="size-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {t("修正用错键盘布局输入的文字（选中的文字；未选中时为光标前打错的词）")} · {KEYBOARD_FIX_SHORTCUT}
      </TooltipContent>
    </Tooltip>
  );
}

/** 光标前有两个打错的词才提示; 只有一个可能是 "mp3" 这类名字。 */
const HINT_MIN_WORDS = 2;

/** 正在打的段落像是用错了布局时, 编辑器下方的提示。 */
export function KeyboardFixHint({ editor }: { editor: Editor | null }) {
  useI18n();
  const [preview, setPreview] = useState<{ before: string; after: string } | null>(null);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    if (!editor || dismissed) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const check = () => {
      clearTimeout(timer);
      timer = setTimeout(() => {
        const run = mistypedRange(editor, HINT_MIN_WORDS);
        if (!run) return setPreview(null);
        const before = editor.state.doc.textBetween(run.from, run.to, " ", " ");
        setPreview({ before, after: convertLayout(before, "toCzech") });
      }, 400);
    };
    editor.on("update", check);
    editor.on("selectionUpdate", check);
    return () => {
      clearTimeout(timer);
      editor.off("update", check);
      editor.off("selectionUpdate", check);
    };
  }, [editor, dismissed]);

  if (!editor || !preview || dismissed) return null;
  const shorten = (s: string) => (s.length > 40 ? `…${s.slice(-40)}` : s);
  return (
    <div className="border-border bg-secondary mx-4 mb-3 flex items-center gap-2 rounded-md border px-3 py-2 text-sm" role="status">
      <Keyboard className="text-muted-foreground size-4 shrink-0" />
      <span className="min-w-0 flex-1 truncate">
        {t("这段文字像是用错了键盘布局：")} <span className="text-muted-foreground">{shorten(preview.before)}</span> → <span className="font-medium">{shorten(preview.after)}</span>
      </span>
      <Button
        variant="outline"
        size="sm"
        className="h-7"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          fixKeyboardLayout(editor);
          editor.commands.focus();
          setPreview(null);
        }}
      >
        {t("修正")} <span className="text-muted-foreground text-xs">{KEYBOARD_FIX_SHORTCUT}</span>
      </Button>
      <Button variant="ghost" size="icon" className="size-7" aria-label={t("本封邮件不再提示")} title={t("本封邮件不再提示")} onClick={() => setDismissed(true)}>
        <X className="size-4" />
      </Button>
    </div>
  );
}
