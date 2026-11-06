// SPDX-License-Identifier: MPL-2.0

/**
 * The device lab's words, added as the first of its modules loads, never with the first page
 * (i18n.ts's addWords). Every module that asks for a `lab.` word imports this.
 */

import { addWords } from "./i18n.ts";
import { enLab } from "./locales/en-lab.ts";

addWords(enLab);
