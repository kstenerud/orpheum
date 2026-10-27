// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { CastToReceive } from "@orpheum/sdk";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastAsked, pressOf, type CastAnswer } from "./cast-asked.tsx";
import { castAnswerWithinMs } from "./cast.ts";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";

/** A cast handed to this device: Metropolis, from the Pixel. */
const cast: CastToReceive = {
  castId: 5,
  membershipCredential: "receiver",
  fileId: 7,
  titleName: "Metropolis",
  positionMs: 61_000,
  fromDeviceId: 2,
  fromDeviceName: "Pixel",
};

/** The page's own full screen, as a test's browser answers asking for it: answer, and how often. */
function pageFullScreen(answer: "grant" | "refuse"): { readonly asked: () => number } {
  let asked = 0;
  document.documentElement.requestFullscreen = (): Promise<void> => {
    asked += 1;
    return answer === "grant" ? Promise.resolve() : Promise.reject(new TypeError("no press"));
  };
  return { asked: () => asked };
}

describe("a cast asked about before it plays (D358)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
    Reflect.deleteProperty(document.documentElement, "requestFullscreen");
  });

  /** CastAsked for cast, served by answers, and what it was answered. */
  async function asked(heard: Heard[] = []): Promise<{
    readonly answers: (CastAnswer | undefined)[];
    readonly server: FakeTogetherServer;
  }> {
    const answers: (CastAnswer | undefined)[] = [];
    const server = fakeTogetherServer();
    const services = servedBy(
      { "POST /me/casts/5/end": [200, { positionMs: 0, isPlaying: false }] },
      heard,
      "token",
    );
    mounted = await mount(
      withServices(
        <CastAsked
          cast={cast}
          onAnswered={(answer) => {
            answers.push(answer);
          }}
        />,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    return { answers, server };
  }

  it("asks whether to play the film, Play focused for a remote's OK (§1)", async () => {
    await asked();
    expect(document.body.textContent).toContain("Pixel wants to play Metropolis here");
    const play = buttonWithText(document.body, "Play");
    expect(document.activeElement).toBe(play);
    expect(buttonWithText(document.body, "Not now")).toBeDefined();
  });

  it("asks for the page full screen in Play's own press, and says by what press it was answered (§2)", async () => {
    const fullScreen = pageFullScreen("grant");
    const { answers } = await asked();
    await click(buttonWithText(document.body, "Play")); // a remote's OK: a click with no count
    expect(fullScreen.asked()).toBe(1);
    expect(answers).toHaveLength(1);
    expect(answers[0]).toMatchObject({ castId: 5, by: "key" });
    await expect(answers[0]?.fullScreen).resolves.toBe("entered");
  });

  it("says a refused full screen as refused", async () => {
    pageFullScreen("refuse");
    const { answers } = await asked();
    await click(buttonWithText(document.body, "Play"));
    await expect(answers[0]?.fullScreen).resolves.toBe("refused");
  });

  it("declines at Not now, its sender told (§3)", async () => {
    const heard: Heard[] = [];
    const { answers } = await asked(heard);
    await click(buttonWithText(document.body, "Not now"));
    expect(answers).toEqual([undefined]);
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/5/end")?.body).toEqual({
      reason: "declined",
    });
  });

  it("takes no answer within a minute as unanswered, its sender told (§4)", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    const heard: Heard[] = [];
    const { answers } = await asked(heard);
    await act(async () => {
      vi.advanceTimersByTime(castAnswerWithinMs - 1);
      await Promise.resolve();
    });
    expect(answers).toEqual([]);
    await act(async () => {
      vi.advanceTimersByTime(1);
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(answers).toEqual([undefined]);
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/5/end")?.body).toEqual({
      reason: "unanswered",
    });
  });

  it("closes when the cast ends before it is answered, its sender gone", async () => {
    const { answers, server } = await asked();
    await server.accept();
    expect(server.heard[0]).toMatchObject({ type: "hello", credential: "receiver" });
    await server.say({ type: "ended", reason: "stopped" });
    expect(answers).toEqual([undefined]);
  });
});

describe("how a press was made", () => {
  it("tells a key's click, a tap and a mouse's click apart", () => {
    expect(pressOf(new MouseEvent("click", { detail: 0 }))).toBe("key");
    expect(pressOf(new PointerEvent("click", { detail: 1, pointerType: "touch" }))).toBe("tap");
    expect(pressOf(new PointerEvent("click", { detail: 1, pointerType: "mouse" }))).toBe("click");
    expect(pressOf(new MouseEvent("click", { detail: 1 }))).toBe("click");
  });
});
