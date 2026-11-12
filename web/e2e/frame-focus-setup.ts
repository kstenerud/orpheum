// SPDX-License-Identifier: MPL-2.0

/** Every browser test file's frame keeps its focus however many pages open beside it (frame-focus.ts). */
import { keepTheFrameFocused } from "./frame-focus.ts";

keepTheFrameFocused(window);
