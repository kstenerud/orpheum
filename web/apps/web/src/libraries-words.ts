// SPDX-License-Identifier: MPL-2.0

/**
 * The Libraries pages' words, added as the first of their modules loads, never with the first page (i18n.ts's
 * addWords; the web client's architecture audit of 2026-11-10, Startup 1). Every module that asks
 * for a `libraries.` word imports this.
 */

import { addWords } from "./i18n.ts";
import { enLibraries } from "./locales/en-libraries.ts";

addWords(enLibraries);
