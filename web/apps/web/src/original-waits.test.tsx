// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DataCaps } from "./data-cap.ts";
import {
  linkCarriesTheOriginal,
  linkLookEveryMs,
  originalAfterLinkCarriesMs,
  useOriginalOnceTheLinkCarries,
} from "./original-waits.ts";
import { heldStorage } from "./test-data-cap.ts";
import { mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
});

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.useRealTimers();
});

/** A film of a 2400 kb/s file on its ladder, the link read from link, waiting for its original or not. */
function Waiting(props: {
  readonly link: { kbps: number | undefined };
  readonly isWaiting: boolean;
  readonly caps: DataCaps;
  readonly carried: string[];
}): JSX.Element | null {
  useOriginalOnceTheLinkCarries(
    props.isWaiting
      ? { fileKbps: 2400, sharePercent: 70, readLinkKbps: () => props.link.kbps }
      : undefined,
    () => props.carried.push("linkCarriesOriginal"),
    props.caps,
  );
  return null;
}

async function waitFor(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

// The owner, 2026-11-15: "Better to start on a lower rung and then go back up later".
describe("a film on its ladder waiting for the original its person chose", () => {
  it("moves to it once its link has carried the file for the hold, and only once", async () => {
    const link = { kbps: 3500 as number | undefined }; // 70% of 3500 is 2450, over 2400
    const carried: string[] = [];
    const caps = new DataCaps(heldStorage().storage, undefined);
    mounted = await mount(<Waiting link={link} isWaiting caps={caps} carried={carried} />);
    await waitFor(originalAfterLinkCarriesMs - linkLookEveryMs);
    expect(carried).toEqual([]);
    await waitFor(linkLookEveryMs);
    expect(carried).toEqual(["linkCarriesOriginal"]);
    await waitFor(originalAfterLinkCarriesMs * 2);
    expect(carried).toEqual(["linkCarriesOriginal"]);
  });

  it("starts its hold again at a look its link falls short at, and waits while it cannot say", async () => {
    const link = { kbps: undefined as number | undefined };
    const carried: string[] = [];
    const caps = new DataCaps(heldStorage().storage, undefined);
    mounted = await mount(<Waiting link={link} isWaiting caps={caps} carried={carried} />);
    await waitFor(originalAfterLinkCarriesMs * 2);
    expect(carried).toEqual([]);
    link.kbps = 3500;
    await waitFor(originalAfterLinkCarriesMs - linkLookEveryMs);
    link.kbps = 3000; // 70% of 3000 is 2100, short of 2400
    await waitFor(linkLookEveryMs);
    link.kbps = 3500;
    await waitFor(originalAfterLinkCarriesMs - linkLookEveryMs);
    expect(carried).toEqual([]);
    await waitFor(linkLookEveryMs);
    expect(carried).toEqual(["linkCarriesOriginal"]);
  });

  it("does not move up where the device's data cap forbids the original (D489), until it is lifted", async () => {
    const link = { kbps: 9600 as number | undefined };
    const carried: string[] = [];
    const caps = new DataCaps(heldStorage().storage, undefined);
    caps.choose(1200);
    mounted = await mount(<Waiting link={link} isWaiting caps={caps} carried={carried} />);
    await waitFor(originalAfterLinkCarriesMs * 2);
    expect(carried).toEqual([]);
    caps.choose(undefined);
    await waitFor(linkLookEveryMs);
    expect(carried).toEqual(["linkCarriesOriginal"]);
  });

  it("is nothing for a film not waiting: one that started on its ladder of itself", async () => {
    const carried: string[] = [];
    const caps = new DataCaps(heldStorage().storage, undefined);
    mounted = await mount(
      <Waiting link={{ kbps: 9600 }} isWaiting={false} caps={caps} carried={carried} />,
    );
    await waitFor(originalAfterLinkCarriesMs * 2);
    expect(carried).toEqual([]);
  });
});

describe("whether a link carries a film's own file (its ladder's rule, D247 §6)", () => {
  it("is the file within the share of the link, and never a link not measured", () => {
    expect(linkCarriesTheOriginal(3429, 2400, 70)).toBe(true); // 2400.3
    expect(linkCarriesTheOriginal(3428, 2400, 70)).toBe(false); // 2399.6
    expect(linkCarriesTheOriginal(undefined, 2400, 70)).toBe(false);
  });
});
