// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { CastToReceive } from "@orpheum/sdk";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastAsked, pressOf, type CastAnswer } from "./cast-asked.tsx";
import { castAnswerWithinMs } from "./cast.ts";
import {
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";

/** A cast handed to this device: Metropolis, from the Pixel. */
const cast: CastToReceive = {
  castId: 5,
  membershipCredential: "receiver",
  fileId: 7,
  itemName: "Metropolis",
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

describe("a cast asked about once, before it plays (D364 §2, D358)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
    Reflect.deleteProperty(document.documentElement, "requestFullscreen");
  });

  /** CastAsked for cast, served by answers, and what it was answered. */
  async function asked(
    heard: Heard[] = [],
    accept: Answers[string] = [200, { castId: 5 }],
  ): Promise<{
    readonly answers: (CastAnswer | undefined)[];
    readonly server: FakeTogetherServer;
  }> {
    const answers: (CastAnswer | undefined)[] = [];
    const server = fakeTogetherServer();
    const services = servedBy(
      {
        "POST /me/casts/5/end": [200, { positionMs: 0, isPlaying: false }],
        "POST /me/casts/5/accept": accept,
      },
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

  it("asks whether to use this device for playback, Use focused for a remote's OK (D364 §2)", async () => {
    await asked();
    expect(document.body.textContent).toContain("Pixel wants to use this device for playback");
    const use = buttonWithText(document.body, "Use");
    expect(document.activeElement).toBe(use);
    expect(buttonWithText(document.body, "Not now")).toBeDefined();
  });

  it("asks for the page full screen in Use's own press, tells the server, and says by what press it was answered (§2)", async () => {
    const fullScreen = pageFullScreen("grant");
    const heard: Heard[] = [];
    const { answers } = await asked(heard);
    await click(buttonWithText(document.body, "Use")); // a remote's OK: a click with no count
    expect(heard.map((each) => each.methodAndPath)).toContain("POST /me/casts/5/accept");
    expect(fullScreen.asked()).toBe(1);
    expect(answers).toHaveLength(1);
    expect(answers[0]).toMatchObject({ castId: 5, by: "key" });
    await expect(answers[0]?.fullScreen).resolves.toBe("entered");
  });

  it("says a refused full screen as refused", async () => {
    pageFullScreen("refuse");
    const { answers } = await asked();
    await click(buttonWithText(document.body, "Use"));
    await expect(answers[0]?.fullScreen).resolves.toBe("refused");
  });

  // The owner, 2026-11-03: a cast whose sender stopped, or which the server lost as it restarted,
  // before Use was pressed; Use took the device into receiving a cast no one sent, full screen.
  it("receives nothing where the server refuses Use, the cast gone: leaves full screen and says its sender stopped", async () => {
    const fullScreen = pageFullScreen("grant");
    let left = 0;
    document.exitFullscreen = (): Promise<void> => {
      left += 1;
      return Promise.resolve();
    };
    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      get: () => (fullScreen.asked() > left ? document.documentElement : null),
    });
    try {
      const { answers } = await asked([], refusal(404, "notFound"));
      await click(buttonWithText(document.body, "Use"));
      await vi.waitFor(() => {
        expect(left).toBe(1);
      });
      expect(answers).toEqual([]);
      expect(document.body.textContent).toContain("Pixel stopped casting.");
      const ok = buttonWithText(document.body, "OK");
      expect(document.activeElement).toBe(ok);
      await click(ok);
      expect(answers).toEqual([undefined]);
    } finally {
      Reflect.deleteProperty(document, "fullscreenElement");
      Reflect.deleteProperty(document, "exitFullscreen");
    }
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
    // The fake clock also moves with real time (shouldAdvanceTime): a millisecond short of the minute,
    // a loaded machine's real milliseconds carried it over and the prompt had closed (2026-10-27).
    const shortOfTheMinuteMs = 5000;
    await act(async () => {
      vi.advanceTimersByTime(castAnswerWithinMs - shortOfTheMinuteMs);
      await Promise.resolve();
    });
    expect(answers).toEqual([]);
    await act(async () => {
      vi.advanceTimersByTime(shortOfTheMinuteMs);
      await Promise.resolve();
    });
    // The end is told and then answered through a few promises: one tick was enough alone and not
    // under the full gate's load (2026-10-27), so wait for the outcome itself.
    await vi.waitFor(() => {
      expect(answers).toEqual([undefined]);
    });
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
