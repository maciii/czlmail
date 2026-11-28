"use client";

import { t } from "@/lib/i18n";
import { useI18n } from "@/components/locale-provider";

import { useEffect, useState } from "react";
import { useEditorState, type Editor } from "@tiptap/react";
import { ChevronDown, Languages, Loader2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { api, errorMessage } from "@/lib/api";
import { languageName, languageShort, NO_CHECK_LANG, onSpellSettingsChange, spellSettings, type SpellSettings } from "@/lib/spellcheck";
import { installSpellcheck, recheckSpelling, replaceWord, setRangeLanguage, type SpellMenuRequest } from "@/components/compose/spellcheck-plugin";

/** 选了语言时给编辑器启用自己的拼写检查; 跟随设置里的改动。 */
export function useSpellcheck(editor: Editor | null) {
  const [settings, setSettings] = useState<SpellSettings | null>(null);
  const [menu, setMenu] = useState<SpellMenuRequest | null>(null);

  useEffect(() => {
    let alive = true;
    spellSettings().then((s) => alive && setSettings(s));
    const off = onSpellSettingsChange((s) => {
      setSettings(s);
      recheckSpelling();
    });
    return () => {
      alive = false;
      off();
    };
  }, []);

  const active = !!settings?.supported && settings.languages.length > 0;
  useEffect(() => {
    if (!editor || !active) return;
    return installSpellcheck(editor, setMenu);
  }, [editor, active]);

  return { settings: active ? settings : null, menu, closeMenu: () => setMenu(null) };
}

/** 标记文字时可选的语言: 所选的语言, 再加「不检查」。 */
function markLanguages(settings: SpellSettings): string[] {
  return [...settings.languages, NO_CHECK_LANG];
}

function markLabel(lang: string) {
  return lang === NO_CHECK_LANG ? t("不检查拼写") : languageName(lang);
}

/**
 * AI 旁边的工具栏按钮: 选中文字 (或接下来输入的文字) 的语言。「我的所有语言」下一个词在任一语言里正确就认可;
 * 选单一语言则只按它检查。
 */
export function SpellLanguageButton({ editor, settings }: { editor: Editor; settings: SpellSettings }) {
  useI18n();
  const current = useEditorState({
    editor,
    selector: ({ editor: e }) => (e.getAttributes("spellLang").lang as string | undefined) ?? "",
  });
  const label = current ? (current === NO_CHECK_LANG ? "—" : languageShort(current)) : settings.languages.map(languageShort).join("+");
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="h-7 gap-1" title={t("拼写检查语言")} onMouseDown={(e) => e.preventDefault()}>
          <Languages className="size-4" />
          <span className="text-xs">{label}</span>
          <ChevronDown className="size-3" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60" onCloseAutoFocus={(e) => e.preventDefault()}>
        <DropdownMenuLabel className="text-muted-foreground text-xs">{t("选中文字的语言")}</DropdownMenuLabel>
        <DropdownMenuRadioGroup
          value={current}
          onValueChange={(lang) => {
            const chain = editor.chain().focus();
            (lang ? chain.setMark("spellLang", { lang }) : chain.unsetMark("spellLang")).run();
          }}
        >
          <DropdownMenuRadioItem value="">{t("我的所有语言")}</DropdownMenuRadioItem>
          {markLanguages(settings).map((lang) => (
            <DropdownMenuRadioItem key={lang} value={lang}>
              {markLabel(lang)}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** 下划线词上的右键菜单: 改法、词典、这个词的语言。 */
export function SpellMenu({ editor, request, settings, onClose }: { editor: Editor; request: SpellMenuRequest; settings: SpellSettings; onClose: () => void }) {
  useI18n();
  const [fetched, setFetched] = useState<{ request: SpellMenuRequest; list: string[] } | null>(null);
  // 已知的常见拼错自带唯一改法; 否则向词典要。
  const fixed = request.replacement !== undefined ? [request.replacement] : null;
  const suggestions = fixed ?? (fetched?.request === request ? fetched.list : null);

  useEffect(() => {
    if (request.replacement !== undefined) return;
    let alive = true;
    api.spellSuggestions(request.word, request.langs).then(
      (list) => alive && setFetched({ request, list }),
      () => alive && setFetched({ request, list: [] }),
    );
    return () => {
      alive = false;
    };
  }, [request]);

  const dictionary = (add: boolean) => {
    (add ? api.spellAddWord : api.spellIgnoreWord)(request.word, request.langs).then(recheckSpelling, (err) => toast.error(errorMessage(err)));
  };

  return (
    <DropdownMenu open onOpenChange={(open) => !open && onClose()}>
      <DropdownMenuTrigger asChild>
        <span aria-hidden className="pointer-events-none fixed size-0" style={{ left: request.x, top: request.y }} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-60" onCloseAutoFocus={(e) => e.preventDefault()}>
        {suggestions === null ? (
          <DropdownMenuLabel className="text-muted-foreground flex items-center gap-2 text-xs">
            <Loader2 className="size-3.5 animate-spin" />
            {t("载入中…")}
          </DropdownMenuLabel>
        ) : suggestions.length === 0 ? (
          <DropdownMenuLabel className="text-muted-foreground text-xs">{t("没有拼写建议")}</DropdownMenuLabel>
        ) : (
          suggestions.map((s) => (
            <DropdownMenuItem key={s} className="font-medium" onSelect={() => replaceWord(editor, request.from, request.to, s)}>
              {s || t("删除")}
            </DropdownMenuItem>
          ))
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => dictionary(true)}>{t("添加到词典")}</DropdownMenuItem>
        <DropdownMenuItem onSelect={() => dictionary(false)}>{t("全部忽略")}</DropdownMenuItem>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>{t("这个词的语言")}</DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <DropdownMenuItem onSelect={() => setRangeLanguage(editor, request.from, request.to, null)}>{t("我的所有语言")}</DropdownMenuItem>
            {markLanguages(settings).map((lang) => (
              <DropdownMenuItem key={lang} onSelect={() => setRangeLanguage(editor, request.from, request.to, lang)}>
                {markLabel(lang)}
              </DropdownMenuItem>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
