// SPDX-License-Identifier: MPL-2.0

import i18next, { type i18n as I18n } from "i18next";
import { initReactI18next } from "react-i18next";

import { en } from "./locales/en.ts";

/**
 * The translation layer (D51), set up before the first render, synchronously, from the bundled
 * English. An untranslated key shows English rather than nothing: returnEmptyString is off,
 * because translation platforms export untranslated keys as "".
 */
export const i18n: I18n = i18next.createInstance();

void i18n.use(initReactI18next).init({
  resources: { en: { translation: en } },
  lng: "en",
  fallbackLng: "en",
  returnEmptyString: false,
  interpolation: { escapeValue: false }, // React escapes
  initAsync: false,
});

/**
 * Adds words a screen loaded later brings with its code (later-screens.tsx), as the lab's and the
 * gallery's do (lab-words.ts, gallery-words.ts), so the first page's script carries none of them
 * (the efficiency audit's finding 10). Added as the screen's module loads, before it first draws.
 */
export function addWords(words: Record<string, unknown>): void {
  i18n.addResourceBundle("en", "translation", words, true, false);
}
