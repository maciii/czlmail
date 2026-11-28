import { Mark, type Editor } from "@tiptap/react";
import type { Node as PMNode } from "@tiptap/pm/model";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet, type EditorView } from "@tiptap/pm/view";

import { api } from "@/lib/api";


/**
 * 邮件正文里自己做的拼写检查 (lib/spellcheck.ts): 给所选语言都不认的词画下划线, 右键弹出改法菜单。
 */

/**
 * 一段文字的语言, 写进邮件时是 <span lang="en-US">。拼写检查对它只用这一种语言, 不用所选的全部语言;
 * 收件人的读屏软件和拼写检查也可以用。
 */
export const SpellLanguage = Mark.create({
  name: "spellLang",
  // 在已标记文字的末尾继续输入, 沿用它的语言 (同 Word 与 Outlook)。
  inclusive: true,
  addAttributes() {
    return { lang: { default: null, parseHTML: (el) => el.getAttribute("lang"), renderHTML: (attrs) => (attrs.lang ? { lang: attrs.lang } : {}) } };
  },
  parseHTML() {
    // 只认按 BCP 47 规范大小写写的语言 (cs、cs-CZ、sr-Latn-RS, 不检查拼写的 zxx), 也就是这里自己写出去的。
    // Word / Outlook 粘贴来的 HTML 满是 lang="EN-US"、"X-NONE" 之类, 当成语言标记的话那段文字只按那一种语言检查。
    return [{ tag: "span[lang]", getAttrs: (el) => (/^[a-z]{2,3}(-[A-Z][a-z]{3})?(-[A-Z]{2})?$/.test(el.getAttribute("lang") ?? "") ? null : false) }];
  },
  renderHTML({ HTMLAttributes }) {
    return ["span", HTMLAttributes, 0];
  },
});

export interface SpellMenuRequest {
  x: number;
  y: number;
  from: number;
  to: number;
  word: string;
  /** 这个词检查时用的语言; 为空 = 所选的语言。 */
  langs: string[];
  replacement?: string;
}

interface Segment {
  from: number;
  text: string;
  lang: string;
}

interface ErrorSpec {
  word: string;
  langs: string[];
  replacement?: string;
}

const key = new PluginKey<DecorationSet>("czlSpellcheck");
const SKIP_MARKS = new Set(["code", "link"]);
const HAS_LETTER = /\p{L}/u;
const CACHE_LIMIT = 2000;

/** 按文字与语言缓存的检查结果, 所有打开的编辑器共用; 词典变化时清空。 */
const cache = new Map<string, { start: number; length: number; replacement?: string }[]>();
/** 各打开编辑器的检查函数, 词典变化后重新跑。 */
const rechecks = new Set<() => void>();

/** 重新检查所有打开的编辑器, 如词加进词典或语言改了之后。 */
export function recheckSpelling() {
  cache.clear();
  for (const run of rechecks) run();
}

/**
 * 每个段落里按语言切开的文字段, 带文档位置。代码、链接与行内节点换成空格, 位置不错位, 也不会被画下划线。
 */
function segments(doc: PMNode): Segment[] {
  const out: Segment[] = [];
  doc.descendants((node, pos) => {
    if (!node.isTextblock) return true;
    let current: Segment | null = null;
    node.forEach((child, offset) => {
      const start = pos + 1 + offset;
      const lang = (child.marks.find((m) => m.type.name === "spellLang")?.attrs.lang as string | undefined) ?? "";
      const skip = !child.isText || child.marks.some((m) => SKIP_MARKS.has(m.type.name));
      const text = skip ? " ".repeat(child.nodeSize) : child.text!;
      if (!current || current.lang !== lang) {
        if (current) out.push(current);
        current = { from: start, text: "", lang };
      }
      current.text += text;
    });
    if (current) out.push(current);
    return false;
  });
  return out.filter((s) => HAS_LETTER.test(s.text));
}

/** 碰到光标的错误属于正在输入的词: 等光标离开它再标。 */
function atCursor(view: EditorView, from: number, to: number) {
  const sel = view.state.selection;
  return sel.empty && view.hasFocus() && sel.from >= from && sel.from <= to;
}

export function spellcheckPlugin(onMenu: (req: SpellMenuRequest) => void): Plugin {
  let lastHead = -1;

  return new Plugin<DecorationSet>({
    key,
    state: {
      init: () => DecorationSet.empty,
      apply: (tr, set) => tr.getMeta(key) ?? set.map(tr.mapping, tr.doc),
    },
    props: {
      decorations: (state) => key.getState(state),
      // WebView2 自带的检查会按单一语言给同样的词画下划线。
      attributes: { spellcheck: "false" },
      handleDOMEvents: {
        contextmenu: (view, event) => {
          const at = view.posAtCoords({ left: event.clientX, top: event.clientY });
          if (!at) return false;
          const found = key.getState(view.state)?.find(at.pos, at.pos);
          const deco = found?.[0];
          if (!deco) return false;
          event.preventDefault();
          const spec = deco.spec as ErrorSpec;
          onMenu({ x: event.clientX, y: event.clientY, from: deco.from, to: deco.to, word: spec.word, langs: spec.langs, replacement: spec.replacement });
          return true;
        },
      },
    },
    view(view) {
      let timer: ReturnType<typeof setTimeout> | undefined;
      let destroyed = false;

      const run = async () => {
        const doc = view.state.doc;
        const segs = segments(doc);
        const keyOf = (s: Segment) => `${s.lang}\u0001${s.text}`;
        const pending = segs.filter((s) => !cache.has(keyOf(s)));
        if (pending.length) {
          try {
            const results = await api.checkSpelling(pending.map((s) => ({ text: s.text, langs: s.lang ? [s.lang] : [] })));
            if (cache.size > CACHE_LIMIT) cache.clear();
            pending.forEach((s, i) => cache.set(keyOf(s), results[i] ?? []));
          } catch {
            return; // 没有检查器 (或调用失败): 文字不标记。
          }
        }
        if (destroyed) return;
        if (!view.state.doc.eq(doc)) return schedule();
        const decos: Decoration[] = [];
        for (const s of segs) {
          for (const e of cache.get(keyOf(s)) ?? []) {
            const from = s.from + e.start;
            const to = from + e.length;
            if (e.length <= 0 || atCursor(view, from, to)) continue;
            const spec: ErrorSpec = { word: s.text.slice(e.start, e.start + e.length), langs: s.lang ? [s.lang] : [], replacement: e.replacement || undefined };
            decos.push(Decoration.inline(from, to, { class: "czl-misspelled" }, spec));
          }
        }
        view.dispatch(view.state.tr.setMeta(key, DecorationSet.create(view.state.doc, decos)).setMeta("addToHistory", false));
      };

      const schedule = (delay = 400) => {
        clearTimeout(timer);
        timer = setTimeout(run, delay);
      };

      const recheck = () => schedule(0);
      rechecks.add(recheck);
      schedule(0);
      return {
        update(v, prev) {
          if (!prev.doc.eq(v.state.doc)) return schedule();
          // 离开了正在输入的词: 立即检查它。
          const head = v.state.selection.head;
          if (head !== lastHead) {
            lastHead = head;
            schedule(150);
          }
        },
        destroy() {
          destroyed = true;
          rechecks.delete(recheck);
          clearTimeout(timer);
        },
      };
    },
  });
}

/** 给编辑器启用自己的检查; 返回关闭它的函数。 */
export function installSpellcheck(editor: Editor, onMenu: (req: SpellMenuRequest) => void): () => void {
  const plugin = spellcheckPlugin(onMenu);
  editor.registerPlugin(plugin);
  return () => {
    if (!editor.isDestroyed) editor.unregisterPlugin(key);
  };
}

/** 用改法替换拼错的范围。 */
export function replaceWord(editor: Editor, from: number, to: number, text: string) {
  const { state } = editor;
  const marks = state.doc.nodeAt(from)?.marks ?? [];
  editor.view.dispatch(text ? state.tr.replaceWith(from, to, state.schema.text(text, marks)) : state.tr.delete(from, to));
  editor.commands.focus();
}

/** 设置一段范围的语言 (null = 所选的全部语言)。 */
export function setRangeLanguage(editor: Editor, from: number, to: number, lang: string | null) {
  const chain = editor.chain().focus().setTextSelection({ from, to });
  (lang ? chain.setMark("spellLang", { lang }) : chain.unsetMark("spellLang")).run();
}
