// SPDX-License-Identifier: MPL-2.0

/**
 * The Users, person, roles and first set-up pages' words, added as the first of their modules loads, never with the first page (i18n.ts's
 * addWords; the web client's architecture audit of 2026-11-10, Startup 1). Every module that asks
 * for a `setUp.`, `people.`, `person.` or `roles.` word imports this.
 */

import { addWords } from "./i18n.ts";
import { enPeople } from "./locales/en-people.ts";

addWords(enPeople);
