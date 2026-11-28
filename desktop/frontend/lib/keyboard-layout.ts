/**
 * 纠正用错键盘布局打的字: 开着英文 (US) 布局打了捷克语 ("6lu+tou4k7 k;+n" → "žluťoučký kůň"), 或反过来 ("Zes" → "Yes")。
 *
 * 对照表按 Windows 的布局逐键对应, 含捷克语布局的死键 (「=」键是 ´, Shift+「=」是 ˇ, 「\」是 ¨), 整个词能原样换回来。
 * 捷克语有 QWERTZ (Windows 默认) 与 QWERTY 两种, 只差 Y 和 Z。
 *
 * 用哪种布局跟着这台机器的键盘走 (同快捷键), 存 localStorage, 不随账号同步。
 * 只对捷克语用户有用: 没设过时捷克语界面默认开启 (QWERTZ), 其它语言默认关闭——
 * 「b2b」「k8s」这类词按捷克语布局换算后也像捷克语单词, 不关掉会给不用捷克语的人误报。
 */

export type CzechLayout = "qwertz" | "qwerty";
/** 设置值: 某种捷克语布局, 或关闭 (编辑器里不出现按钮、快捷键与提示)。 */
export type CzechLayoutSetting = CzechLayout | "off";

const STORAGE_KEY = "czlmail.czechKeyboard";

/** 这台机器的设置; 没设过时按界面语言: 捷克语界面为 QWERTZ, 其它为关闭。 */
export function czechLayoutSetting(uiLocale: string): CzechLayoutSetting {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === "qwertz" || v === "qwerty" || v === "off") return v;
  } catch {
    // 读不到存储时按界面语言。
  }
  return uiLocale === "cs" ? "qwertz" : "off";
}

/** 换算用的布局 (关闭时也给一个, 供直接调用转换函数时用)。 */
export function czechLayout(): CzechLayout {
  try {
    return localStorage.getItem(STORAGE_KEY) === "qwerty" ? "qwerty" : "qwertz";
  } catch {
    return "qwertz";
  }
}

export function setCzechLayout(layout: CzechLayoutSetting) {
  try {
    localStorage.setItem(STORAGE_KEY, layout);
  } catch {
    // 存不了时下次仍按界面语言的默认值。
  }
}

const ACUTE = "´"; // ´
const CARON = "ˇ"; // ˇ
const DIAERESIS = "¨"; // ¨

/** US 布局的字符 → 同一个键在捷克语布局上打出的字符 (不含 Y/Z, 见 layoutMap)。 */
const US_TO_CS: Record<string, string> = {
  "`": ";", "~": "°",
  "1": "+", "!": "1",
  "2": "ě", "@": "2",
  "3": "š", "#": "3",
  "4": "č", "$": "4",
  "5": "ř", "%": "5",
  "6": "ž", "^": "6",
  "7": "ý", "&": "7",
  "8": "á", "*": "8",
  "9": "í", "(": "9",
  "0": "é", ")": "0",
  "-": "=", "_": "%",
  "=": ACUTE, "+": CARON,
  "[": "ú", "{": "/",
  "]": ")", "}": "(",
  "\\": DIAERESIS, "|": "'",
  ";": "ů", ":": '"',
  "'": "§", '"': "!",
  "<": "?", ">": ":",
  "/": "-", "?": "_",
};

const DEAD_KEYS: Record<string, Record<string, string>> = {
  [ACUTE]: { a: "á", e: "é", i: "í", o: "ó", u: "ú", y: "ý", A: "Á", E: "É", I: "Í", O: "Ó", U: "Ú", Y: "Ý" },
  [CARON]: {
    c: "č", d: "ď", e: "ě", n: "ň", r: "ř", s: "š", t: "ť", z: "ž", l: "ľ",
    C: "Č", D: "Ď", E: "Ě", N: "Ň", R: "Ř", S: "Š", T: "Ť", Z: "Ž", L: "Ľ",
  },
  [DIAERESIS]: { a: "ä", e: "ë", o: "ö", u: "ü", A: "Ä", E: "Ë", O: "Ö", U: "Ü" },
};

function layoutMap(layout: CzechLayout): Record<string, string> {
  if (layout === "qwerty") return US_TO_CS;
  return { ...US_TO_CS, y: "z", z: "y", Y: "Z", Z: "Y" };
}

/** 用 US 布局打出的文字 → 同样的按键在捷克语布局上打出的文字。 */
export function usToCzech(text: string, layout: CzechLayout = czechLayout()): string {
  const map = layoutMap(layout);
  const chars = Array.from(text);
  let out = "";
  for (let i = 0; i < chars.length; i++) {
    const c = map[chars[i]] ?? chars[i];
    const dead = DEAD_KEYS[c];
    if (!dead || !(chars[i] in map)) {
      out += c;
      continue;
    }
    // 死键与下一个键合成一个字母; 后面跟空格时打出符号本身, 与 Windows 相同。
    const next = i + 1 < chars.length ? (map[chars[i + 1]] ?? chars[i + 1]) : undefined;
    if (next === " ") {
      out += c;
      i++;
    } else if (next !== undefined && dead[next]) {
      out += dead[next];
      i++;
    } else {
      out += c;
    }
  }
  return out;
}

/** 用捷克语布局打出的文字 → 同样的按键在 US 布局上打出的文字。 */
export function czechToUs(text: string, layout: CzechLayout = czechLayout()): string {
  const map = layoutMap(layout);
  const reverse: Record<string, string> = {};
  for (const [us, cs] of Object.entries(map)) reverse[cs] = us;
  // 靠死键打出的字母: 先死键, 再基本字母。
  for (const [mark, letters] of Object.entries(DEAD_KEYS)) {
    for (const [base, letter] of Object.entries(letters)) {
      if (!(letter in reverse)) reverse[letter] = reverse[mark] + (reverse[base] ?? base);
    }
  }
  return Array.from(text, (c) => reverse[c] ?? c).join("");
}

/** 只有捷克语布局才打得出 (或靠死键合成) 的字符。 */
const CZECH_ONLY = /[ěščřžýáíéůúóďťňäëöüľĚŠČŘŽÝÁÍÉŮÚÓĎŤŇÄËÖÜĽ§°´ˇ¨]/;

/** 在捷克语布局上打出捷克字母的 US 键: 用错布局打的词里会有它们。 */
const LETTER_KEYS = /[2-90;[+=]/;
const LETTER = /\p{L}/u;
const LETTERS_ONLY = /^\p{L}+$/u;
/** 两种布局里都可能出现在词尾的标点。 */
const TRAILING = /[.,!?:"')(\-]+$/;

/**
 * 像是开着 US 布局打的捷克语词: 字母里夹着能打出捷克字母的键, 换算后是一个全由字母组成的词
 * ("6lu+tou4k7" → "žluťoučký")。年份 ("2026")、"mp3" 之类没有字母或字母太少, 不算 (见 mistypedRun)。
 */
export function looksMistyped(word: string, layout: CzechLayout = czechLayout()): boolean {
  if (CZECH_ONLY.test(word) || !LETTER_KEYS.test(word) || !/[a-zA-Z]/.test(word)) return false;
  const converted = usToCzech(word, layout).replace(TRAILING, "");
  // 剩下单独的死键符号 (ˇ 在 Unicode 里算「修饰字母」) 说明这几个键拼不成词。
  return converted.length > 1 && LETTERS_ONLY.test(converted) && !/[´ˇ¨]/.test(converted) && CZECH_ONLY.test(converted);
}

export interface TextRun {
  from: number;
  to: number;
}

/**
 * 光标前用错布局打的那一串词: 从最后一个带捷克字母的词 (它是用捷克语布局打的) 之后的第一个打错的词起, 到光标为止。
 * 第一个打错的词之前的词保持原样: "za" 这类普通词在两种布局里除 Y、Z 外打法相同, 换算只会把它们改坏。
 *
 * 看起来打错的词少于 `minWords` 个时返回 null。
 */
export function mistypedRun(text: string, cursor: number, minWords = 1, layout: CzechLayout = czechLayout()): TextRun | null {
  const words = Array.from(text.slice(0, cursor).matchAll(/\S+/g), (m) => ({ from: m.index, to: m.index + m[0].length, word: m[0] }));
  let start = 0;
  for (let i = words.length - 1; i >= 0; i--) {
    if (CZECH_ONLY.test(words[i].word) && LETTER.test(words[i].word)) {
      start = i + 1;
      break;
    }
  }
  const tail = words.slice(start);
  const first = tail.findIndex((w) => looksMistyped(w.word, layout));
  if (first < 0) return null;
  if (tail.filter((w) => looksMistyped(w.word, layout)).length < minWords) return null;
  return { from: tail[first].from, to: tail[tail.length - 1].to };
}

/** 选中文字往哪个方向换: 里面有捷克字母说明是用捷克语布局打的。 */
export function convertDirection(text: string, layout: CzechLayout = czechLayout()): "toCzech" | "toUs" {
  const words = text.split(/\s+/).filter(Boolean);
  if (words.some((w) => looksMistyped(w, layout))) return "toCzech";
  return CZECH_ONLY.test(text) ? "toUs" : "toCzech";
}

export function convertLayout(text: string, direction: "toCzech" | "toUs", layout: CzechLayout = czechLayout()): string {
  return direction === "toCzech" ? usToCzech(text, layout) : czechToUs(text, layout);
}
