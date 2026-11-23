// SPDX-License-Identifier: MPL-2.0

/** Every browser test file's frame has a sessionStorage of its own (frame-storage.ts). */
import { giveTheFrameItsOwnStorage } from "./frame-storage.ts";

giveTheFrameItsOwnStorage(window);
