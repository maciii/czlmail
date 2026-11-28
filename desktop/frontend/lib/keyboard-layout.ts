/**
 * Fix text typed with the wrong keyboard layout: Czech typed while the English (US) layout was active
 * ("6lu+tou4k7 k;+n" → "žluťoučký kůň") and the other way round ("Zes" → "Yes").
 *
 * The tables follow the Windows layouts key by key, including the dead keys of the Czech layout
 * (´ on the "=" key, ˇ on Shift+"=", ¨ on "\"), so a whole word converts back exactly.
 * Czech exists as QWERTZ (the Windows default) and QWERTY; they differ only in Y and Z.
 *
 * The layout is a per-machine preference like the shortcuts: it follows the keyboard, not the account.
 */

export type CzechLayout = "qwertz" | "qwerty";

const STORAGE_KEY = "czlmail.czechKeyboard";

export function czechLayout(): CzechLayout {
  try {
    return localStorage.getItem(STORAGE_KEY) === "qwerty" ? "qwerty" : "qwertz";
  } catch {
    return "qwertz";
  }
}

export function setCzechLayout(layout: CzechLayout) {
  try {
    localStorage.setItem(STORAGE_KEY, layout);
  } catch {
    // Without storage the default (QWERTZ) applies.
  }
}

const ACUTE = "´"; // ´
const CARON = "ˇ"; // ˇ
const DIAERESIS = "¨"; // ¨

/** US character → the character the same key gives on the Czech layout (without Y/Z, see layoutMap). */
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

/** Text typed on the US layout → what the same keys give on the Czech layout. */
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
    // A dead key combines with the next key; with a space it gives the accent itself, as in Windows.
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

/** Text typed on the Czech layout → what the same keys give on the US layout. */
export function czechToUs(text: string, layout: CzechLayout = czechLayout()): string {
  const map = layoutMap(layout);
  const reverse: Record<string, string> = {};
  for (const [us, cs] of Object.entries(map)) reverse[cs] = us;
  // Letters written with a dead key: the dead key, then the base letter.
  for (const [mark, letters] of Object.entries(DEAD_KEYS)) {
    for (const [base, letter] of Object.entries(letters)) {
      if (!(letter in reverse)) reverse[letter] = reverse[mark] + (reverse[base] ?? base);
    }
  }
  return Array.from(text, (c) => reverse[c] ?? c).join("");
}

/** Characters only the Czech layout types (or composes with its dead keys). */
const CZECH_ONLY = /[ěščřžýáíéůúóďťňäëöüľĚŠČŘŽÝÁÍÉŮÚÓĎŤŇÄËÖÜĽ§°´ˇ¨]/;

/** US keys that give Czech letters, so a word typed on the wrong layout contains them. */
const LETTER_KEYS = /[2-90;[+=]/;
const LETTER = /\p{L}/u;
const LETTERS_ONLY = /^\p{L}+$/u;
/** Punctuation that may end a word in both layouts. */
const TRAILING = /[.,!?:"')(\-]+$/;

/**
 * A word that looks like Czech typed on the US layout: it mixes letters with keys that give Czech
 * letters, and converted it is one word of letters ("6lu+tou4k7" → "žluťoučký"). Years ("2026"),
 * "mp3" and the like are left alone because they have no letters, or have too few to count (see mistypedRun).
 */
export function looksMistyped(word: string, layout: CzechLayout = czechLayout()): boolean {
  if (CZECH_ONLY.test(word) || !LETTER_KEYS.test(word) || !/[a-zA-Z]/.test(word)) return false;
  const converted = usToCzech(word, layout).replace(TRAILING, "");
  // A dead key left on its own (ˇ is a "modifier letter" in Unicode) means the keys did not form a word.
  return converted.length > 1 && LETTERS_ONLY.test(converted) && !/[´ˇ¨]/.test(converted) && CZECH_ONLY.test(converted);
}

export interface TextRun {
  from: number;
  to: number;
}

/**
 * The run of words before the cursor that was typed on the wrong layout: from the first mistyped word
 * after the last word that has Czech letters (that one was typed on the Czech layout) up to the cursor.
 * Words before the first mistyped one stay as they are: plain words such as "za" type the same on
 * both layouts except for Y and Z, so converting them could only break them.
 *
 * Returns null when fewer than `minWords` words look mistyped.
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

/** Which way to convert a selection: Czech letters in it mean it was typed on the Czech layout. */
export function convertDirection(text: string, layout: CzechLayout = czechLayout()): "toCzech" | "toUs" {
  const words = text.split(/\s+/).filter(Boolean);
  if (words.some((w) => looksMistyped(w, layout))) return "toCzech";
  return CZECH_ONLY.test(text) ? "toUs" : "toCzech";
}

export function convertLayout(text: string, direction: "toCzech" | "toUs", layout: CzechLayout = czechLayout()): string {
  return direction === "toCzech" ? usToCzech(text, layout) : czechToUs(text, layout);
}
