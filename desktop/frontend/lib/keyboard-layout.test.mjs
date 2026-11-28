import assert from "node:assert/strict";
import { test } from "node:test";

import { convertDirection, czechToUs, looksMistyped, mistypedRun, usToCzech } from "./keyboard-layout.ts";

const pangram = "Příliš žluťoučký kůň úpěl ďábelské ódy, 2026!";

test("Czech typed on the US layout converts back, dead keys included", () => {
  assert.equal(usToCzech("6lu+tou4k7 k;+n", "qwertz"), "žluťoučký kůň");
  assert.equal(usToCzech("P59li3 6lu+tou4k7 k;+n [p2l +d8belsk0 =odz", "qwertz"), "Příliš žluťoučký kůň úpěl ďábelské ódy");
  assert.equal(usToCzech("Dobr2 den", "qwerty"), "Dobrě den");
  // A dead key with a space gives the accent (the space is used up); with a letter it cannot take, both stay.
  assert.equal(usToCzech("+ a+b", "qwertz"), "ˇaˇb");
});

test("the conversion is reversible on both Czech layouts", () => {
  for (const layout of ["qwertz", "qwerty"]) {
    assert.equal(usToCzech(czechToUs(pangram, layout), layout), pangram);
  }
  assert.equal(czechToUs("Zes, I šill", "qwertz"), "Yes, I 3ill");
  assert.equal(czechToUs("Zes", "qwerty"), "Zes");
});

test("only words that become Czech words look mistyped", () => {
  for (const word of ["6lu+tou4k7", "k;+n", "d2kuji,", "+rek"]) assert.ok(looksMistyped(word, "qwertz"), word);
  for (const word of ["2026", "a+b", "žluťoučký", "hello", "1+1"]) assert.ok(!looksMistyped(word, "qwertz"), word);
});

test("the mistyped run starts at the first mistyped word after the last Czech one", () => {
  const text = "Díky za opravu, te+d nar869m na dal39 v2c";
  assert.deepEqual(mistypedRun(text, text.length, 1, "qwertz"), { from: text.indexOf("te+d"), to: text.length });
  assert.equal(mistypedRun("Ahoj mp3", 8, 2, "qwertz"), null);
  assert.equal(usToCzech("te+d nar869m na dal39 v2c", "qwertz"), "teď narážím na další věc");
});

test("the direction follows the text", () => {
  assert.equal(convertDirection("6lu+tou4k7", "qwertz"), "toCzech");
  assert.equal(convertDirection("Zes, I šill", "qwertz"), "toUs");
  assert.equal(convertDirection("plain words", "qwertz"), "toCzech");
});
