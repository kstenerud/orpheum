// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DataCaps } from "./data-cap.ts";
import {
  isWaitingForTheOriginal,
  linkCarriesTheOriginal,
  linkLookEveryMs,
  linkOverTheFileAtLeast,
  originalAfterLinkCarriesMs,
  OriginalTries,
  triedFailsWithinMs,
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

/**
 * A film of a 2400 kb/s file on its ladder, the link read from link, its buffer full as ahead says
 * (full when not given), waiting for its original or not, its playback's tries given or not.
 */
function Waiting(props: {
  readonly link: { kbps: number | undefined };
  readonly ahead?: { isFull: boolean };
  readonly isWaiting: boolean;
  readonly caps: DataCaps;
  readonly carried: string[];
  readonly tries?: OriginalTries;
}): JSX.Element | null {
  const { ahead } = props;
  useOriginalOnceTheLinkCarries(
    props.isWaiting
      ? {
          fileKbps: 2400,
          readLinkKbps: () => props.link.kbps,
          readIsAheadFull: () => ahead?.isFull ?? true,
        }
      : undefined,
    () => props.carried.push("linkCarriesOriginal"),
    { caps: props.caps, tries: props.tries },
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
    const link = { kbps: 3500 as number | undefined }; // over 1.3 × 2400, 3120
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
    link.kbps = 3000; // short of 1.3 × 2400, 3120
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

describe("whether a link carries a film's own file (D501 §1)", () => {
  it("is the link at least 1.3 times the file, and never a link not measured", () => {
    expect(linkCarriesTheOriginal(3121, 2400)).toBe(true);
    expect(linkCarriesTheOriginal(3119, 2400)).toBe(false);
    expect(linkCarriesTheOriginal(undefined, 2400)).toBe(false);
    expect(linkOverTheFileAtLeast).toBe(1.3);
  });
});

// D501 §1: about 60 s of the link carrying the file, with the film's buffer full.
describe("the hold before a film moves up to the original (D501 §1)", () => {
  it("is about a minute", () => {
    expect(originalAfterLinkCarriesMs).toBe(60_000);
  });

  it("waits for the film's buffer to be full once the link has carried the file for the hold", async () => {
    const link = { kbps: 3500 as number | undefined };
    const ahead = { isFull: false };
    const carried: string[] = [];
    const caps = new DataCaps(heldStorage().storage, undefined);
    mounted = await mount(
      <Waiting link={link} ahead={ahead} isWaiting caps={caps} carried={carried} />,
    );
    await waitFor(originalAfterLinkCarriesMs * 2);
    expect(carried).toEqual([]);
    ahead.isFull = true;
    await waitFor(linkLookEveryMs);
    expect(carried).toEqual(["linkCarriesOriginal"]);
  });
});

// D501 §2: one try per film; a move down from the original within two minutes ends the moves up.
describe("a film's one try at the original (D501 §2)", () => {
  /** The time the tries read, moved on with the looks (waitAlong). */
  const clock = { ms: 0 };

  /** Lets ms pass for the looks and the tries' time alike. */
  async function waitAlong(ms: number): Promise<void> {
    clock.ms += ms;
    await waitFor(ms);
  }
  /** Mounts a waiting film whose link carries its file, its playback's tries given, and lets the hold pass. */
  async function movedUpWith(tries: OriginalTries, carried: string[]): Promise<void> {
    const caps = new DataCaps(heldStorage().storage, undefined);
    mounted?.unmount();
    mounted = await mount(
      <Waiting link={{ kbps: 3500 }} isWaiting caps={caps} carried={carried} tries={tries} />,
    );
    await waitAlong(originalAfterLinkCarriesMs);
  }

  it("ends the moves up when the player moves down from the original within two minutes", async () => {
    const tries = new OriginalTries(() => clock.ms);
    const carried: string[] = [];
    await movedUpWith(tries, carried);
    expect(carried).toEqual(["linkCarriesOriginal"]);
    expect(tries.isOnTheOriginal).toBe(true);
    await waitAlong(triedFailsWithinMs - linkLookEveryMs);
    tries.movedDown();
    expect(tries.isEnded).toBe(true);
    expect(tries.isOnTheOriginal).toBe(false);
    expect(isWaitingForTheOriginal({ isOriginalWaitingForTheLink: true }, tries)).toBe(false);
    // A decision that waits for the original again, as the person choosing it again would have.
    await movedUpWith(tries, carried);
    await waitAlong(originalAfterLinkCarriesMs * 2);
    expect(carried).toEqual(["linkCarriesOriginal"]);
  });

  it("leaves them where the original held two minutes, and where no move up was made", async () => {
    const tries = new OriginalTries(() => clock.ms);
    tries.movedDown(); // the original as its person chose it, on a link not measured
    expect(tries.isEnded).toBe(false);
    const carried: string[] = [];
    await movedUpWith(tries, carried);
    await waitAlong(triedFailsWithinMs);
    tries.movedDown();
    expect(tries.isEnded).toBe(false);
    expect(tries.isOnTheOriginal).toBe(false);
    expect(isWaitingForTheOriginal({ isOriginalWaitingForTheLink: true }, tries)).toBe(true);
    expect(isWaitingForTheOriginal({}, tries)).toBe(false);
    expect(triedFailsWithinMs).toBe(120_000);
  });
});
