// SPDX-License-Identifier: MPL-2.0

/**
 * A credited person's page's words (locales/en-credits.ts), added as its module loads, never with
 * the first page (i18n.ts's addWords; the owner, 2026-11-13: reverting a part of the shared
 * catalogue re-ran nearly every web test). Every module that asks for a `credits.` word imports this.
 */

import { addWords } from "./i18n.ts";
import { enCredits } from "./locales/en-credits.ts";

addWords(enCredits);
