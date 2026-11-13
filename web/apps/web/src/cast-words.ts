// SPDX-License-Identifier: MPL-2.0

/**
 * The casting pages' words (locales/en-cast.ts), added as the first of the modules that ask for a
 * `cast.` or `remote.` word loads, never with the first page (i18n.ts's addWords; the owner,
 * 2026-11-13: reverting a part of the shared catalogue re-ran nearly every web test). Every module
 * that asks for one imports this. What a start asks for, a cast taken on a page waiting for a press,
 * stays in the first page's `castStart.` words (en.ts).
 */

import { addWords } from "./i18n.ts";
import { enCast } from "./locales/en-cast.ts";

addWords(enCast);
