// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { NextWarmUp, playOnWarmUp, warmUpKeptMs, type WarmedDecision } from "./warm-up.ts";

/** A decision of its token, on its film's ladder unless isOnLadder is false. */
function decision(mediaToken: string, isOnLadder = true): WarmedDecision {
  return isOnLadder ? { mediaToken, onLadder: {} } : { mediaToken };
}

/** A NextWarmUp on a clock at clock.ms, each playback it lets go of kept in left. */
function keeper(): {
  readonly next: NextWarmUp<WarmedDecision>;
  readonly left: string[];
  readonly clock: { ms: number };
} {
  const left: string[] = [];
  const clock = { ms: 5 };
  const next = new NextWarmUp<WarmedDecision>({
    leave: (token) => left.push(token),
    nowMs: () => clock.ms,
  });
  return { next, left, clock };
}

/** Lets every promise settled so far run its callbacks. */
async function settled(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe("the next episode's warm-up (the owner's request, 2026-11-11; 2026-11-27)", () => {
  it("asks once for its next, and the next played takes it, its playback left alone", async () => {
    const { next, left } = keeper();
    let asks = 0;
    const ask = (): Promise<WarmedDecision> => {
      asks += 1;
      return Promise.resolve(decision("next-1"));
    };
    next.warm(8, 80, ask);
    next.warm(8, 80, ask);
    expect(asks).toBe(1);
    await settled();
    const taken = next.take(8);
    expect(taken?.itemId).toBe(80);
    expect(taken?.askedMs).toBe(5);
    await expect(taken?.decision).resolves.toEqual(decision("next-1"));
    next.letGo();
    await settled();
    expect(left).toEqual([]);
  });

  it("lets go of one next for another, and of the next where another film plays", async () => {
    const { next, left } = keeper();
    next.warm(8, undefined, () => Promise.resolve(decision("next-8")));
    await settled();
    next.warm(9, undefined, () => Promise.resolve(decision("next-9")));
    expect(left).toEqual(["next-8"]);
    await settled();
    expect(next.take(10)).toBeUndefined();
    expect(left).toEqual(["next-8", "next-9"]);
    expect(next.isWarming(9)).toBe(false);
  });

  it("lets go of a next once answered where it was let go of while asked, but not one taken", async () => {
    const { next, left } = keeper();
    next.warm(8, undefined, () => Promise.resolve(decision("next-8")));
    next.letGo();
    next.warm(9, undefined, () => Promise.resolve(decision("next-9")));
    expect(next.take(9)).toBeDefined();
    await settled();
    expect(left).toEqual(["next-8"]);
  });
});

describe("a film played on its warm-up (D258 §1, D260 §1)", () => {
  /** What playOnWarmUp did with warm asked at askedMs and played at nowMs: each step it took. */
  async function played(
    warm: Promise<WarmedDecision>,
    nowMs: number,
  ): Promise<{ readonly steps: string[]; readonly plays: WarmedDecision }> {
    const steps: string[] = [];
    const plays = await playOnWarmUp(
      { decision: warm, askedMs: 0, itemId: undefined },
      {
        nowMs,
        askAgain: (cause) => {
          steps.push(`asked again for ${cause}`);
          return Promise.resolve(decision("again"));
        },
        stale: () => steps.push("stale"),
        offLadder: (off) => steps.push(`let go of ${off.mediaToken}`),
        played: (on) => {
          steps.push(`played ${on.mediaToken}`);
          return on;
        },
      },
    );
    return { steps, plays };
  }

  it("plays on a fresh warm-up on the film's ladder, asking nothing", async () => {
    const { steps, plays } = await played(Promise.resolve(decision("warm")), warmUpKeptMs - 1);
    expect(steps).toEqual(["played warm"]);
    expect(plays).toEqual(decision("warm"));
  });

  it("asks again for one kept too long, one off the ladder, and one that failed, saying why", async () => {
    expect((await played(Promise.resolve(decision("warm")), warmUpKeptMs)).steps).toEqual([
      "stale",
      "asked again for warmUpStale",
    ]);
    expect((await played(Promise.resolve(decision("warm", false)), 0)).steps).toEqual([
      "let go of warm",
      "asked again for warmUpOffLadder",
    ]);
    const { steps, plays } = await played(Promise.reject(new Error("refused")), 0);
    expect(steps).toEqual(["asked again for warmUpFailed"]);
    expect(plays).toEqual(decision("again"));
  });
});
