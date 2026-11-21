// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { buildOfPage } from "./page-build.ts";

/** A page whose head holds markup, as the build writes it. */
function pageWith(head: string): Document {
  return new DOMParser().parseFromString(
    `<html><head>${head}</head><body></body></html>`,
    "text/html",
  );
}

describe("buildOfPage", () => {
  it("reads the build the page's meta element names, which its worker's address carries (VQ374)", () => {
    expect(
      buildOfPage(pageWith('<meta name="orpheum-build" content="v0.8.9-61-g44ebca125">')),
    ).toBe("v0.8.9-61-g44ebca125");
  });

  it("names none for a page with no marker, or an empty one, as the dev loop's", () => {
    expect(buildOfPage(pageWith(""))).toBeUndefined();
    expect(buildOfPage(pageWith('<meta name="orpheum-build" content="">'))).toBeUndefined();
  });
});
