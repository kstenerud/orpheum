// SPDX-License-Identifier: MPL-2.0

/**
 * The UI gallery's words, added as its module loads, never with the first page (i18n.ts's
 * addWords). Every module that asks for a `gallery.` word imports this.
 */

import { addWords } from "./i18n.ts";
import { enGallery } from "./locales/en-gallery.ts";

addWords(enGallery);
