"use client";

import { api, hasRuntime, onEvent } from "@/lib/api";
import { getLocale } from "@/lib/i18n";

/**
 * 经 Windows 拼写检查 API 按多种语言检查拼写 (见 spellcheck.go 与 internal/spell)。
 * 在设置 → 写信里选了语言才启用; 启用后编辑器关掉 WebView2 自带的检查 (它只认一种语言)。
 */

export interface SpellSettings {
  supported: boolean;
  /** 自己检查用的语言; 为空表示由 WebView2 检查。 */
  languages: string[];
  /** Windows 能检查的语言。 */
  available: string[];
}

export const SPELLCHECK_EVENT = "spellcheck:changed";

let cached: Promise<SpellSettings | null> | null = null;

export function spellSettings(refresh = false): Promise<SpellSettings | null> {
  if (!hasRuntime()) return Promise.resolve(null);
  if (!cached || refresh) cached = api.getSpellcheckSettings().then((s) => s as SpellSettings, () => null);
  return cached;
}

/** 设置里改了之后, 用最新设置回调。 */
export function onSpellSettingsChange(cb: (s: SpellSettings | null) => void): () => void {
  return onEvent(SPELLCHECK_EVENT, () => {
    spellSettings(true).then(cb);
  });
}

/** "cs-CZ" → 界面语言下的语言名 (如 "čeština"); 取不到时退回标记本身。 */
export function languageName(tag: string): string {
  if (tag === NO_CHECK_LANG) return tag;
  try {
    const names = new Intl.DisplayNames([getLocale(), "en"], { type: "language" });
    return names.of(tag) ?? tag;
  } catch {
    return tag;
  }
}

/** 工具栏按钮的简称: "cs"、"en"。 */
export function languageShort(tag: string): string {
  return tag.split("-")[0].toUpperCase();
}

/** BCP 47 的 "无语言内容": 标成它的文字不检查 (人名、代码、地址)。 */
export const NO_CHECK_LANG = "zxx";
