// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { decisionTimesOf } from "./play-state.ts";
import { servedBy, type Answer, type Heard } from "./test-dom.tsx";
import { testServices } from "./test-services.tsx";
import { Watching, warmUpKeptMs } from "./watching.ts";

const playableFormats = testServices().playableFormats();

/** A decision naming its media token, on its film's ladder unless isCut is false, as POST /playback answers. */
function decided(mediaToken: string, isCut = true): Answer {
  const onLadder = {
    url: `/api/v1/bytes/media/chunks/${mediaToken}/master.m3u8`,
    startAtBps: 4_000_000,
    qualityKbps: 2400,
    height: 1080,
    isBelowTheLowest: false,
    soundCarriedGainDb: 0,
    isReady: false,
    filmPictureKbps: 2400,
  };
  return [
    200,
    {
      delivery: "direct",
      url: "/u",
      notDirectReasons: [],
      mediaToken,
      ...(isCut ? { onLadder } : {}),
    },
  ];
}

/** The requests heard, by method and path, once every one sent so far has been answered. */
async function settled(heard: readonly Heard[]): Promise<string[]> {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await new Promise((resolve) => setTimeout(resolve, 0));
  return heard.map((request) => request.methodAndPath);
}

/** A Watching over a stand-in server answering decisions a, b, c in turn, and a clock it moves. */
function watchingServed(heard: Heard[]): { watching: Watching; clock: { ms: number } } {
  const { api } = servedBy(
    {
      "POST /playback": [decided("a"), decided("b"), decided("c")],
      "DELETE /playback/a": [204, undefined],
      "DELETE /playback/b": [204, undefined],
    },
    heard,
  );
  const clock = { ms: 0 };
  return { watching: new Watching(api, () => clock.ms), clock };
}

describe("the film being watched (D258)", () => {
  it("plays on the decision its film's page warmed up, asking once", async () => {
    const heard: Heard[] = [];
    const { watching } = watchingServed(heard);
    watching.warmUp(7, playableFormats);
    await settled(heard);
    const pressedMs = performance.now();
    const decision = await watching.decide(7, playableFormats);
    expect(decision.mediaToken).toBe("a");
    expect(await settled(heard)).toEqual(["POST /playback"]);
    // Its start is timed from Play, not from opening the page.
    expect(decisionTimesOf("a")?.askedMs).toBeGreaterThanOrEqual(pressedMs);
  });

  it("keeps its playbacks when the person goes back to its page, and lets go of them only for another film", async () => {
    const heard: Heard[] = [];
    const { watching } = watchingServed(heard);
    await watching.decide(7, playableFormats);
    // Back to film 7's page, from the player: nothing is let go of, nor asked again.
    watching.warmUp(7, playableFormats);
    expect(await settled(heard)).toEqual(["POST /playback", "POST /playback"]);
    // Film 9's page: film 7's playbacks are let go of.
    watching.warmUp(9, playableFormats);
    expect(await settled(heard)).toEqual([
      "POST /playback",
      "POST /playback",
      "DELETE /playback/a",
      "DELETE /playback/b",
      "POST /playback",
    ]);
  });

  it("asks again when its page's decision has waited too long, or was already played on", async () => {
    const heard: Heard[] = [];
    const { watching, clock } = watchingServed(heard);
    watching.warmUp(7, playableFormats);
    clock.ms = warmUpKeptMs;
    expect((await watching.decide(7, playableFormats)).mediaToken).toBe("b");
    // Deciding again, as a changed sound does, asks the server again.
    expect((await watching.decide(7, playableFormats)).mediaToken).toBe("c");
  });

  it("asks again at Play when its page's decision came before the film was cut (D260 §1)", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy(
      {
        "POST /playback": [decided("a", false), decided("b")],
        "DELETE /playback/a": [204, undefined],
      },
      heard,
    );
    const watching = new Watching(api, () => 0);
    watching.warmUp(7, playableFormats);
    const decision = await watching.decide(7, playableFormats);
    expect(decision.mediaToken).toBe("b");
    expect(decision.onLadder).toBeDefined();
  });
});
