// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CastReceiver, castNoteShownMs, moveOnOrGoHome, TakeBackPrompt } from "./cast-receiver.tsx";
import { mount, refusal, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";

/** Presses key on what has focus, as a remote's press reaches the page. */
function press(key: string, keyCode = 0): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, keyCode, bubbles: true, cancelable: true });
  act(() => {
    (document.activeElement ?? document.body).dispatchEvent(event);
  });
  return event;
}

/** The take-back prompt's button, if it shows. */
function takeBackButton(mounted: Mounted): HTMLButtonElement | null {
  return mounted.container.querySelector(".cast-receiver__take-back");
}

describe("the receiver's take-back (D337 §4)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  it("says who controls it, and lets the line fade", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    mounted = await mount(
      withServices(<TakeBackPrompt fromName="Pixel" onTakeBack={() => undefined} />),
    );
    expect(mounted.container.textContent).toContain("Controlled from Pixel");
    act(() => {
      vi.advanceTimersByTime(castNoteShownMs);
    });
    expect(mounted.container.textContent).not.toContain("Controlled from Pixel");
  });

  it("shows Take back control at OK, and takes it at a second OK", async () => {
    const onTakeBack = vi.fn();
    mounted = await mount(
      withServices(<TakeBackPrompt fromName="Pixel" onTakeBack={onTakeBack} />),
    );
    expect(takeBackButton(mounted)).toBeNull();
    press("Enter");
    expect(takeBackButton(mounted)).not.toBeNull();
    expect(onTakeBack).not.toHaveBeenCalled();
    press("Enter");
    expect(onTakeBack).toHaveBeenCalledOnce();
  });

  it("leaves the media keys to the player, raising nothing", async () => {
    const onTakeBack = vi.fn();
    mounted = await mount(
      withServices(<TakeBackPrompt fromName="Pixel" onTakeBack={onTakeBack} />),
    );
    press("MediaPlayPause", 179);
    press("ArrowRight");
    expect(takeBackButton(mounted)).toBeNull();
  });

  it("hears Back, a Fire TV's history step, as a press, and keeps its entry for the next", async () => {
    const onTakeBack = vi.fn();
    const services = testServices("token", "/play/3");
    const pushed = vi.spyOn(services.navigation, "go");
    mounted = await mount(
      withServices(<TakeBackPrompt fromName="Pixel" onTakeBack={onTakeBack} />, services),
    );
    expect(pushed).toHaveBeenCalledWith({ screen: "play", fileId: 3 });
    act(() => {
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(takeBackButton(mounted)).not.toBeNull();
    act(() => {
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(onTakeBack).toHaveBeenCalledOnce();
    expect(pushed).toHaveBeenCalledTimes(3);
  });

  it("hears a tap as a press, and hides the prompt again when no second one comes", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    const onTakeBack = vi.fn();
    mounted = await mount(
      withServices(<TakeBackPrompt fromName="Pixel" onTakeBack={onTakeBack} />),
    );
    act(() => {
      document.body.click();
    });
    expect(takeBackButton(mounted)).not.toBeNull();
    act(() => {
      vi.advanceTimersByTime(castNoteShownMs);
    });
    expect(takeBackButton(mounted)).toBeNull();
    expect(onTakeBack).not.toHaveBeenCalled();
  });
});

/** A cast handed to this device: file 4 from 61 s, cast from the Pixel. */
const cast = {
  castId: 9,
  membershipCredential: "receiver",
  fileId: 4,
  positionMs: 61_000,
  fromDeviceId: 1,
  fromDeviceName: "Pixel",
};

/** The receiver's welcome to its cast's session of one, nothing on yet. */
const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 9,
  memberId: 2,
  deviceId: 6,
  isHost: true,
  rights: { canPlayPause: true, canSeek: true },
  group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [],
};

describe("the receiver of a cast (D337 §3, §7; §9 Q4)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    document.body.replaceChildren();
  });

  /** A receiver of cast leading video, served by answers, and what it was told of its end. */
  async function receiver(
    answers: Parameters<typeof servedBy>[0] = {},
    hasNext = false,
  ): Promise<{
    readonly server: FakeTogetherServer;
    readonly video: HTMLVideoElement;
    readonly heard: Heard[];
    readonly services: ReturnType<typeof servedBy>;
    readonly onOver: ReturnType<typeof vi.fn>;
  }> {
    const heard: Heard[] = [];
    const services = servedBy(answers, heard, "token");
    const server = fakeTogetherServer();
    const video = document.createElement("video");
    document.body.append(video);
    const onOver = vi.fn();
    mounted = await mount(
      withServices(
        <CastReceiver
          receiving={{ cast, onOver }}
          fileId={4}
          video={video}
          startPositionMs={0}
          hasNext={hasNext}
        />,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    return { server, video, heard, services, onOver };
  }

  it("leads its session of one from where the sender stood", async () => {
    const { server } = await receiver();
    expect(server.heard[0]).toMatchObject({
      type: "hello",
      credential: "receiver",
      role: "playback",
    });
    await server.say(welcome);
    expect(server.heard).toContainEqual(
      expect.objectContaining({
        type: "command",
        command: "changeItem",
        fileId: 4,
        positionMs: 61_000,
      }),
    );
  });

  it("pauses and plays on as an ordinary player when the remote stops casting", async () => {
    const { server, video, onOver } = await receiver();
    const pause = vi.spyOn(video, "pause").mockImplementation(() => undefined);
    await server.say(welcome);
    await server.say({ type: "ended", reason: "stopped" });
    expect(pause).toHaveBeenCalled();
    expect(onOver).toHaveBeenCalledOnce();
  });

  it("goes on receiving when another cast replaces this one", async () => {
    const { server, onOver } = await receiver();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "replaced" });
    expect(onOver).not.toHaveBeenCalled();
  });

  it("ends the cast at the film's end and goes home, the remote told", async () => {
    const { server, video, heard, services, onOver } = await receiver({
      "POST /me/casts/9/end": [200, { positionMs: 600_000, isPlaying: false }],
    });
    await server.say(welcome);
    await act(async () => {
      video.dispatchEvent(new Event("ended"));
      await Promise.resolve();
    });
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
      reason: "filmEnded",
    });
    expect(onOver).toHaveBeenCalledOnce();
    expect(services.navigation.route()).toEqual({ screen: "home" });
  });

  it("leaves its film's end to Up next where an episode plays next (plans/up-next.md)", async () => {
    const { server, video, heard, onOver } = await receiver({}, true);
    await server.say(welcome);
    await act(async () => {
      video.dispatchEvent(new Event("ended"));
      await Promise.resolve();
    });
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/end")).toBe(false);
    expect(onOver).not.toHaveBeenCalled();
  });
});

describe("a receiver's Up next (plans/up-next.md; D337's §9 answers, Q4)", () => {
  it("moves the cast on to the next episode, which the server hands it again", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "POST /me/casts/9/next": [200, { castId: 9, fileId: 5 }] }, heard);
    const onOver = vi.fn();
    await act(async () => {
      moveOnOrGoHome(services.api, services.navigation, { cast, onOver });
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /me/casts/9/next"]);
    expect(onOver).not.toHaveBeenCalled();
  });

  it("ends the cast and goes home, the remote told, where it cannot move on", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /me/casts/9/next": refusal(409, "conflict"),
        "POST /me/casts/9/end": [200, { positionMs: 600_000, isPlaying: false }],
      },
      heard,
    );
    const onOver = vi.fn();
    await act(async () => {
      moveOnOrGoHome(services.api, services.navigation, { cast, onOver });
      for (let turn = 0; turn < 5; turn += 1) {
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
    });
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
      reason: "filmEnded",
    });
    expect(onOver).toHaveBeenCalledOnce();
    expect(services.navigation.route()).toEqual({ screen: "home" });
  });
});
