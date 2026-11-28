"use client";

import { api, hasRuntime, onEvent } from "@/lib/api";
import { getLocale } from "@/lib/i18n";

/**
 * Spell checking in several languages through the Windows Spell Checking API (see spellcheck.go and
 * internal/spell). Off until languages are chosen in Settings → Compose; then the editor turns off
 * WebView2's own checking, which knows one language only.
 */

export interface SpellSettings {
  supported: boolean;
  /** The single language of WebView2's own checking (applies after a restart). */
  language: string;
  /** Languages of our own checking; empty means WebView2 checks. */
  languages: string[];
  /** Languages Windows can check. */
  available: string[];
}

export const SPELLCHECK_EVENT = "spellcheck:changed";

let cached: Promise<SpellSettings | null> | null = null;

export function spellSettings(refresh = false): Promise<SpellSettings | null> {
  if (!hasRuntime()) return Promise.resolve(null);
  if (!cached || refresh) cached = api.getSpellcheckSettings().then((s) => s as SpellSettings, () => null);
  return cached;
}

/** Calls back with fresh settings whenever they change in Settings. */
export function onSpellSettingsChange(cb: (s: SpellSettings | null) => void): () => void {
  return onEvent(SPELLCHECK_EVENT, () => {
    spellSettings(true).then(cb);
  });
}

/** "cs-CZ" → "čeština" in the UI language; falls back to the tag. */
export function languageName(tag: string): string {
  if (tag === NO_CHECK_LANG) return tag;
  try {
    const names = new Intl.DisplayNames([getLocale(), "en"], { type: "language" });
    return names.of(tag) ?? tag;
  } catch {
    return tag;
  }
}

/** Short label for a toolbar button: "cs", "en". */
export function languageShort(tag: string): string {
  return tag.split("-")[0].toUpperCase();
}

/** BCP 47 "no linguistic content": text marked with it is not checked (names, code, addresses). */
export const NO_CHECK_LANG = "zxx";
