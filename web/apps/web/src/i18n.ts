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
