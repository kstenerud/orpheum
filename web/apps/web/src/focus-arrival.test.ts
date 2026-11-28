// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import { arriveWhenDrawn } from "./focus-navigation.ts";

/** Lets the arrival's first frame and its watch's look run. */
async function drawn(): Promise<void> {
  for (let frame = 0; frame < 3; frame += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
    await new Promise((resolve) => requestAnimationFrame(resolve));
  }
}

describe("a page's arrival, as its first frame hears a focusin", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  /** A page opened from opener, focused, its arrival drawn a moment later; heard is fired first. */
  async function arrivedAfter(heardOn: (opener: HTMLElement) => HTMLElement): Promise<Element[]> {
    const opener = document.createElement("button");
    const elsewhere = document.createElement("button");
    elsewhere.id = "elsewhere";
    const page = document.createElement("main");
    document.body.append(opener, elsewhere, page);
    opener.focus();
    const given: Element[] = [];
    const arrival = document.createElement("button");
    const stop = arriveWhenDrawn(
      () => page,
      (element) => {
        given.push(element);
      },
      (root) => root.querySelector("button") ?? undefined,
    );
    heardOn(opener).dispatchEvent(new FocusEvent("focusin", { bubbles: true }));
    page.append(arrival);
    await drawn();
    stop();
    return given;
  }

  // A window given the keyboard back fires focusin on what had focus all along: Firefox in CI, the
  // test's frame behind another as the film's page opened, left focus on nothing (2026-11-28).
  it("still arrives when the focusin is on what opened the page, nothing having moved", async () => {
    expect(await arrivedAfter((opener) => opener)).toHaveLength(1);
  });

  it("does not arrive once focus has gone anywhere else, the person having moved on", async () => {
    const elsewhere = (): HTMLElement =>
      document.querySelector<HTMLElement>("#elsewhere") ?? document.body;
    expect(await arrivedAfter(elsewhere)).toEqual([]);
  });
});
