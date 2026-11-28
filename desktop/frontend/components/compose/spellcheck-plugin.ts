import { Mark, type Editor } from "@tiptap/react";
import type { Node as PMNode } from "@tiptap/pm/model";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet, type EditorView } from "@tiptap/pm/view";

import { api } from "@/lib/api";


/**
 * Our own spell checking in the message body (lib/spellcheck.ts): underlines what none of the chosen
 * languages accepts, and opens a menu with corrections on right click.
 */

/**
 * The language of a piece of text, written to the mail as <span lang="en-US">. Spell checking uses it
 * instead of all the chosen languages, and recipients' screen readers and spell checkers can too.
 */
export const SpellLanguage = Mark.create({
  name: "spellLang",
  // Typing at the end of marked text continues in its language, like Word and Outlook.
  inclusive: true,
  addAttributes() {
    return { lang: { default: null, parseHTML: (el) => el.getAttribute("lang"), renderHTML: (attrs) => (attrs.lang ? { lang: attrs.lang } : {}) } };
  },
  parseHTML() {
    return [{ tag: "span[lang]" }];
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
  /** The languages the word was checked in; empty = the chosen ones. */
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

/** Results per text and language, shared by all open editors; cleared when the dictionary changes. */
const cache = new Map<string, { start: number; length: number; replacement?: string }[]>();
/** The checks of the open editors, to run again after the dictionary changed. */
const rechecks = new Set<() => void>();

/** Checks all open editors again, e.g. after a word was added to the dictionary or languages changed. */
export function recheckSpelling() {
  cache.clear();
  for (const run of rechecks) run();
}

/**
 * Text runs of one language per paragraph, with their document positions. Code, links and inline
 * nodes become spaces, so positions stay aligned and they are never underlined.
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

/** Errors that touch the cursor belong to the word being typed: wait until the cursor leaves it. */
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
      // WebView2's own checking would underline the same words in one language only.
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
            return; // No checker (or the call failed): leave the text unmarked.
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
          // Leaving a word that was being typed: check it now.
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

/** Turns our checking on for an editor; returns the function that turns it off again. */
export function installSpellcheck(editor: Editor, onMenu: (req: SpellMenuRequest) => void): () => void {
  const plugin = spellcheckPlugin(onMenu);
  editor.registerPlugin(plugin);
  return () => {
    if (!editor.isDestroyed) editor.unregisterPlugin(key);
  };
}

/** Replaces a misspelled range with a correction. */
export function replaceWord(editor: Editor, from: number, to: number, text: string) {
  const { state } = editor;
  const marks = state.doc.nodeAt(from)?.marks ?? [];
  editor.view.dispatch(text ? state.tr.replaceWith(from, to, state.schema.text(text, marks)) : state.tr.delete(from, to));
  editor.commands.focus();
}

/** Sets the language of a range (null = all chosen languages). */
export function setRangeLanguage(editor: Editor, from: number, to: number, lang: string | null) {
  const chain = editor.chain().focus().setTextSelection({ from, to });
  (lang ? chain.setMark("spellLang", { lang }) : chain.unsetMark("spellLang")).run();
}
