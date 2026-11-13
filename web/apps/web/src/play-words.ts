// SPDX-License-Identifier: MPL-2.0

/**
 * The player's words (locales/en-play.ts), added as the first of the modules that ask for a
 * `play.`, `skipped.`, `upNext.` or `playerControls.` word loads, never with the first page
 * (i18n.ts's addWords; the owner, 2026-11-13: reverting a part of the shared catalogue re-ran
 * nearly every web test). Every module that asks for one imports this.
 */

import { addWords } from "./i18n.ts";
import { enPlay } from "./locales/en-play.ts";

addWords(enPlay);
