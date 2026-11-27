// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  isEverySheetMade,
  scrubPicturesReadAgainEveryMs,
  useScrubPictures,
} from "./scrub-pictures.tsx";
import { mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { forgetEveryAnswer } from "./use-resource.ts";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  forgetEveryAnswer();
  vi.useRealTimers();
});

/** A film of 250 pictures in sheets of 10 × 10: three sheets, the given ones made. */
function made(...indexes: number[]): readonly [200, object] {
  return [
    200,
    {
      intervalMs: 10_000,
      pictureWidth: 320,
      pictureHeight: 180,
      columns: 10,
      rows: 10,
      pictureCount: 250,
      sheets: indexes.map((index) => ({
        index,
        url: `/api/v1/bytes/trickplay/3/${String(index)}.webp`,
      })),
    },
  ];
}

const notMade = [404, { error: { code: "not_found", message: "no pictures made" } }] as const;

function Pictures(): JSX.Element {
  const pictures = useScrubPictures(3, null);
  return (
    <p>{pictures === undefined ? "none" : pictures.sheets.map((sheet) => sheet.index).join(",")}</p>
  );
}

async function mountPictures(
  answers: Parameters<typeof servedBy>[0],
  heard: Heard[],
): Promise<void> {
  mounted = await mount(withServices(<Pictures />, servedBy(answers, heard)));
}

async function waitOn(ms: number): Promise<void> {
  await act(async () => {
    vi.advanceTimersByTime(ms);
    await Promise.resolve();
  });
}

const reads = (heard: readonly Heard[]): number =>
  heard.filter((each) => each.methodAndPath === "GET /files/3/trickplay").length;

const shown = (): string | null | undefined => mounted?.container.textContent;

describe("a film's scrub-bar pictures, asked again as it plays while not all are made (trickplay.md §5)", () => {
  it("shows the sheets made since the player opened, without opening it again", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const heard: Heard[] = [];
    await mountPictures({ "GET /files/3/trickplay": [notMade, made(0), made(0, 1)] }, heard);
    await settleUntil(() => reads(heard) === 1);
    expect(shown()).toBe("none");
    await waitOn(scrubPicturesReadAgainEveryMs);
    await settleUntil(() => shown() === "0");
    await waitOn(scrubPicturesReadAgainEveryMs);
    await settleUntil(() => shown() === "0,1");
    expect(reads(heard)).toBe(3);
  });

  it("asks no more once every sheet is made", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const heard: Heard[] = [];
    await mountPictures({ "GET /files/3/trickplay": [made(0, 1), made(0, 1, 2)] }, heard);
    await settleUntil(() => shown() === "0,1");
    await waitOn(scrubPicturesReadAgainEveryMs);
    await settleUntil(() => shown() === "0,1,2");
    await waitOn(scrubPicturesReadAgainEveryMs);
    await waitOn(scrubPicturesReadAgainEveryMs);
    await waitOn(scrubPicturesReadAgainEveryMs);
    expect(reads(heard)).toBe(2);
  });

  it("asks no more once the player closes", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const heard: Heard[] = [];
    await mountPictures({ "GET /files/3/trickplay": notMade }, heard);
    await settleUntil(() => reads(heard) === 1);
    await waitOn(scrubPicturesReadAgainEveryMs);
    await settleUntil(() => reads(heard) === 2);
    mounted?.unmount();
    mounted = undefined;
    await waitOn(scrubPicturesReadAgainEveryMs);
    await waitOn(scrubPicturesReadAgainEveryMs);
    expect(reads(heard)).toBe(2);
  });

  it("keeps the sheets it shows through a read that fails, and asks again only when next due", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const heard: Heard[] = [];
    const failed = [500, { error: { code: "internal", message: "the server failed" } }] as const;
    await mountPictures({ "GET /files/3/trickplay": [made(0), failed, made(0, 1)] }, heard);
    await settleUntil(() => shown() === "0");
    await waitOn(scrubPicturesReadAgainEveryMs);
    await settleUntil(() => reads(heard) === 2);
    await act(async () => {
      await Promise.resolve();
    });
    expect(shown()).toBe("0");
    expect(reads(heard)).toBe(2);
    await waitOn(scrubPicturesReadAgainEveryMs);
    await settleUntil(() => shown() === "0,1");
  });
});

describe("whether every sheet of a film's pictures is made", () => {
  const pictures = (count: number, indexes: number[]) => ({
    intervalMs: 10_000,
    pictureWidth: 320,
    pictureHeight: 180,
    columns: 10,
    rows: 10,
    pictureCount: count,
    sheets: indexes.map((index) => ({ index, url: `/${String(index)}.webp` })),
  });

  it("counts the sheets the pictures call for, the last one part full", () => {
    expect(isEverySheetMade(pictures(250, [0, 1]))).toBe(false);
    expect(isEverySheetMade(pictures(250, [0, 2]))).toBe(false);
    expect(isEverySheetMade(pictures(250, [0, 1, 2]))).toBe(true);
    expect(isEverySheetMade(pictures(200, [0, 1]))).toBe(true);
  });
});
