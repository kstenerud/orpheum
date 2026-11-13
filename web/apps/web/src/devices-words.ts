// SPDX-License-Identifier: MPL-2.0

/**
 * The This device page's and the person's list of devices' words (locales/en-devices.ts), added as
 * the first of their modules loads, never with the first page (i18n.ts's addWords; the owner,
 * 2026-11-13: reverting a part of the shared catalogue re-ran nearly every web test). Every module
 * that asks for a `thisDevice.` or `devices.` word imports this.
 */

import { addWords } from "./i18n.ts";
import { enDevices } from "./locales/en-devices.ts";

addWords(enDevices);
