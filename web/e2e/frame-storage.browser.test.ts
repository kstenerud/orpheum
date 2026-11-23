// SPDX-License-Identifier: MPL-2.0

import { afterEach, expect, it } from "vitest";

// The frames of the one test page shared its sessionStorage, so a mark one test file left held
// another file's page (frame-storage.ts). This file's frame runs with the setup the config gives
// every file.
afterEach(() => {
  window.sessionStorage.removeItem("frame-storage-test");
});

it("keeps what a test file's frame stores out of the tab's storage the other frames share", () => {
  window.sessionStorage.setItem("frame-storage-test", "kept");
  expect(window.sessionStorage.getItem("frame-storage-test")).toBe("kept");
  expect(window.parent).not.toBe(window);
  expect(window.parent.sessionStorage.getItem("frame-storage-test")).toBeNull();
});
