// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { createApi, upNextLeadMs } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { UpNextCard, upNextLines } from "./up-next.ts";

const bag = {
  itemId: 8,
  fileId: 8,
  name: "Bag",
  groupNumber: 1,
  numberInGroup: 3,
  isPartway: false,
};
const chosenThere = "Choose it on your phone or computer";

describe("a Chromecast's Up next words (D471)", () => {
  it("say the episode as the caster's offer does, and where it is chosen", () => {
    const disc = { ...bag, groupNumber: 2, groupName: "Disc 2" };
    expect(upNextLines({ episode: disc, ordering: "dvd" })).toEqual([
      "Up next: Disc 2 · 3 · Bag",
      chosenThere,
    ]);
    expect(upNextLines({ episode: { ...bag, groupNumber: 2 }, ordering: "dvd" })?.[0]).toBe(
      "Up next: Part 2 · 3 · Bag",
    );
    expect(upNextLines({ episode: bag, ordering: "absolute", isOnlyGroup: true })?.[0]).toBe(
      "Up next: Episode 3 · Bag",
    );
  });

  it("are none where nothing plays next, or it cannot be read now, as the caster offers none", () => {
    expect(upNextLines(undefined)).toBeUndefined();
    const away = { reason: "nodeAway", nodeName: "devnas" } as const;
    expect(
      upNextLines({ episode: { ...bag, unavailable: away }, ordering: "aired" }),
    ).toBeUndefined();
  });
});

describe("a Chromecast's Up next card (D471)", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  /** A video in the page, at atS of a film of durationS, which the test moves along. */
  function videoAt(durationS: number): { video: HTMLVideoElement; at: (atS: number) => void } {
    const video = document.createElement("video");
    let atS = 0;
    Object.defineProperty(video, "duration", { get: () => durationS });
    Object.defineProperty(video, "currentTime", { get: () => atS });
    Object.defineProperty(video, "ended", { get: () => atS >= durationS });
    document.body.append(video);
    return {
      video,
      at: (to) => {
        atS = to;
        video.dispatchEvent(new Event("timeupdate"));
      },
    };
  }

  it("shows what plays next near the film's end, by the caster's rule, and goes before it and after", async () => {
    const asked: string[] = [];
    const api = createApi({
      origin: "https://core.example",
      getToken: () => "token",
      fetch: (request) => {
        asked.push(`${request.method} ${new URL(request.url).pathname}`);
        const body = { upNext: { episode: { ...bag, groupName: "Disc 1" }, ordering: "dvd" } };
        return Promise.resolve(Response.json(body));
      },
    });
    const { video, at } = videoAt(1200);
    const card = new UpNextCard(video, api, () => undefined);
    const shown = (): string | undefined => {
      const drawn = document.querySelector<HTMLElement>(".receiver__up-next");
      return drawn?.hidden === false ? drawn.textContent : undefined;
    };
    card.plays(7);
    await vi.waitFor(() => {
      expect(asked).toEqual(["GET /api/v1/files/7/up-next"]);
    });
    at(100);
    expect(shown()).toBeUndefined();
    at(1200 - upNextLeadMs / 1000 + 1);
    expect(shown()).toBe(`Up next: Disc 1 · 3 · Bag${chosenThere}`);
    // Seeked back, as the caster's remote hides its own.
    at(600);
    expect(shown()).toBeUndefined();
    at(1200);
    expect(shown()).toBe(`Up next: Disc 1 · 3 · Bag${chosenThere}`);
    // The next film plays: nothing until its own is read.
    card.plays(0);
    expect(shown()).toBeUndefined();
    card.remove();
    expect(document.querySelector(".receiver__up-next")).toBeNull();
  });
});
