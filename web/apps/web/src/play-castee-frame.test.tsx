// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CasteePlay } from "./play-castee.tsx";
import { scrubPicturesReadAgainEveryMs } from "./scrub-pictures.tsx";
import { castingRemote } from "./test-castee.tsx";
import { buttonNamed, click, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";

/** Each image the page loads, by its address. */
const asked: string[] = [];

/** Every sheet loads at once, 10 × 10 pictures of 320 × 180. */
class FakeImage {
  onload: (() => void) | null = null;
  naturalWidth = 0;
  set src(url: string) {
    asked.push(url);
    queueMicrotask(() => {
      this.naturalWidth = 3200;
      this.onload?.();
    });
  }
}

const sheetUrl = (index: number): string => `/api/v1/bytes/trickplay/4/${String(index)}.webp`;

/** Film 4's pictures, every 10 s, in sheets of 10 × 10: the first two of its six made. */
const madeTwo = [
  200,
  {
    intervalMs: 10_000,
    pictureWidth: 320,
    pictureHeight: 180,
    columns: 10,
    rows: 10,
    pictureCount: 600,
    sheets: [0, 1].map((index) => ({ index, url: sheetUrl(index) })),
  },
] as const;

const notMade = [404, { error: { code: "not_found", message: "no pictures made" } }] as const;

/** The remote's picture: its sheet and how far across and down the sheet is moved, in boxes. */
function picture(page: Element): string | null {
  const image = page.querySelector<HTMLImageElement>(".cast-remote__frame .cast-frame-image");
  return image === null
    ? null
    : `${image.getAttribute("src") ?? ""} ${image.style.left} ${image.style.top}`;
}

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  asked.length = 0;
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

type Remote = Awaited<ReturnType<typeof castingRemote>>;

/** The remote on film 4's Play page, its pictures' reads answered with answers in turn. */
async function casting(
  answers: readonly (typeof notMade | typeof madeTwo)[],
  heard: Heard[],
): Promise<{ readonly server: Remote["sockets"]["session"]; readonly page: HTMLElement }> {
  const drawn = await castingRemote({ "GET /files/4/trickplay": [...answers] }, heard, {
    remote: CasteePlay,
    onPlayHere: () => undefined,
  });
  mounted = drawn.mounted;
  return { server: drawn.sockets.session, page: drawn.mounted.container };
}

describe("the cast remote's picture (D571, the rework plan, phase 3)", () => {
  it("shows the film's still until its pictures exist, then the picture of where the castee stands, the pictures read again as they are made", async () => {
    vi.stubGlobal("Image", FakeImage);
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const heard: Heard[] = [];
    const { page } = await casting([notMade, madeTwo], heard);
    await settleUntil(() => heard.some((each) => each.methodAndPath === "GET /files/4/trickplay"));
    expect(picture(page)).toBeNull(); // none made: the still
    await act(async () => {
      vi.advanceTimersByTime(scrubPicturesReadAgainEveryMs);
      await Promise.resolve();
    });
    // 1:01 is picture 6: sheet 0, the 7th across in its first row.
    await settleUntil(() => picture(page) === `${sheetUrl(0)} -600% 0%`);
    expect(asked.some((url) => url.includes("/frame"))).toBe(false);
  });

  it("shows the picture of where the castee says it stands, not where a seek chosen on the remote aims, until the castee says it got there", async () => {
    vi.stubGlobal("Image", FakeImage);
    const { server, page } = await casting([madeTwo], []);
    await settleUntil(() => picture(page) === `${sheetUrl(0)} -600% 0%`);
    await click(buttonNamed(page, "Play"));
    await click(buttonNamed(page, "Forward 10 s"));
    expect(page.textContent).toContain("1:11 / 10:00"); // the aim shown at once
    expect(picture(page)).toBe(`${sheetUrl(0)} -600% 0%`); // no picture where nothing yet stands
    await settleUntil(() => server.heard.at(-1)?.["command"] === "seek");
    expect(picture(page)).toBe(`${sheetUrl(0)} -600% 0%`);
    await server.say({
      type: "group",
      seq: 5,
      group: {
        state: "paused",
        positionMs: 71_000,
        rate: 1,
        fileId: 4,
        event: "seeked",
        seekedFromMs: 61_000,
      },
    });
    await settleUntil(() => picture(page) === `${sheetUrl(0)} -700% 0%`);
    expect(asked.some((url) => url.includes("/frame"))).toBe(false);
  });
});
