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
 * Fix text typed with the wrong keyboard layout in the message body (see lib/keyboard-layout.ts):
 * Ctrl+Shift+K or the toolbar button converts the selection, or without a selection the mistyped
 * words before the cursor; a hint under the editor offers it when a paragraph looks mistyped.
 */

export const KEYBOARD_FIX_SHORTCUT = "Ctrl+Shift+K";

/** Ctrl+Shift+K by the physical key, so it works whichever layout is active. */
export function isKeyboardFixShortcut(e: KeyboardEvent): boolean {
  return (e.ctrlKey || e.metaKey) && e.shiftKey && !e.altKey && e.code === "KeyK";
}

/** Characters of the current paragraph before the cursor; inline leaves (images, line breaks) count as one. */
function textBeforeCursor(editor: Editor): { start: number; text: string } {
  const { $from } = editor.state.selection;
  const start = $from.start();
  return { start, text: editor.state.doc.textBetween(start, $from.pos, "\n", "\n") };
}

/** The mistyped words before the cursor, as document positions. */
function mistypedRange(editor: Editor, minWords: number): TextRun | null {
  if (!editor.state.selection.empty) return null;
  const { start, text } = textBeforeCursor(editor);
  const run = mistypedRun(text, text.length, minWords);
  return run && { from: start + run.from, to: start + run.to };
}

/** Converts the text between from and to, keeping its formatting; returns whether anything changed. */
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
  // From the end, so the positions of the earlier parts stay valid.
  for (const part of parts.reverse()) {
    const converted = convertLayout(part.text, direction);
    if (converted === part.text) continue;
    tr.replaceWith(part.from, part.to, state.schema.text(converted, part.marks));
  }
  if (!tr.docChanged) return false;
  // A selection stays selected, so pressing the shortcut again converts it back.
  if (keepSelection) tr.setSelection(TextSelection.create(tr.doc, tr.mapping.map(from, -1), tr.mapping.map(to, 1)));
  editor.view.dispatch(tr.scrollIntoView());
  return true;
}

/** Converts the selection, or without one the mistyped words before the cursor. */
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
          // Keep the selection in the editor: the button must not take focus on mouse down.
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

/** Two mistyped words before the cursor are enough to offer the fix; one could be a name like "mp3". */
const HINT_MIN_WORDS = 2;

/** A hint under the editor when the paragraph being typed looks like it was typed on the wrong layout. */
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
