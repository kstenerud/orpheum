// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  CastReceiver,
  castNoteShownMs,
  ControlledMode,
  doublePressWithinMs,
  moveOnOrLeave,
} from "./cast-receiver.tsx";
import { onCommand } from "./input-commands.ts";
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

/** Clicks the page as a tap does: a click with a count, which a key's never has. */
function tap(): void {
  act(() => {
    document.body.dispatchEvent(
      new MouseEvent("click", { bubbles: true, cancelable: true, detail: 1 }),
    );
  });
}

describe("the receiver's controlled mode (D338 §6)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  /** A receiver's controlled mode, and what it was told of leaving. */
  async function controlled(
    services = testServices("token", "/play/3"),
  ): Promise<{ readonly mounted: Mounted; readonly onLeave: ReturnType<typeof vi.fn> }> {
    const onLeave = vi.fn();
    mounted = await mount(
      withServices(<ControlledMode fromName="Pixel" onTakeBack={onLeave} />, services),
    );
    return { mounted, onLeave };
  }

  it("says who controls it and how to leave, and lets the line fade", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    const shown = await controlled();
    expect(shown.mounted.container.textContent).toContain(
      "Controlled from Pixel · Back, or OK twice, to take control here",
    );
    act(() => {
      vi.advanceTimersByTime(castNoteShownMs);
    });
    expect(shown.mounted.container.textContent).not.toContain("Controlled from Pixel");
  });

  it("leaves at Back, before anything else on the page hears it", async () => {
    const { onLeave } = await controlled();
    const heard = vi.fn();
    const stop = onCommand(document, heard, { isCapture: true });
    const back = press("Escape");
    stop();
    expect(onLeave).toHaveBeenCalledOnce();
    expect(back.defaultPrevented).toBe(true);
    expect(heard).not.toHaveBeenCalled();
  });

  it("leaves at a Fire TV's Back, a step back from the entry it keeps", async () => {
    const services = testServices("token", "/play/3");
    const pushed = vi.spyOn(services.navigation, "go");
    const { onLeave } = await controlled(services);
    expect(pushed).toHaveBeenCalledWith({ screen: "play", fileId: 3 });
    act(() => {
      window.dispatchEvent(new PopStateEvent("popstate"));
    });
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("leaves at OK pressed twice within the window, and leaves one OK to the player", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { onLeave } = await controlled();
    const heard = vi.fn();
    const stop = onCommand(document, heard, { isCapture: true });
    press("Enter");
    vi.advanceTimersByTime(doublePressWithinMs + 1);
    press("Enter");
    expect(onLeave).not.toHaveBeenCalled();
    expect(heard).toHaveBeenCalledTimes(2); // each the player's own
    vi.advanceTimersByTime(doublePressWithinMs - 1);
    const second = press("Enter");
    stop();
    expect(onLeave).toHaveBeenCalledOnce();
    expect(second.defaultPrevented).toBe(true);
  });

  it("leaves every other key to the player", async () => {
    const { onLeave } = await controlled();
    for (const [key, keyCode] of [
      ["MediaPlayPause", 179],
      ["ArrowRight", 39],
      ["ArrowUp", 38],
    ] as const) {
      expect(press(key, keyCode).defaultPrevented).toBe(false);
    }
    expect(onLeave).not.toHaveBeenCalled();
  });

  it("leaves at a tap twice, counting no click a key made nor the click following OK", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { onLeave } = await controlled();
    act(() => {
      document.body.click(); // a key's click: no count
    });
    tap();
    vi.advanceTimersByTime(doublePressWithinMs + 1);
    press("Enter");
    tap(); // OK's own click, the same press
    vi.advanceTimersByTime(doublePressWithinMs + 1);
    tap();
    expect(onLeave).not.toHaveBeenCalled();
    vi.advanceTimersByTime(doublePressWithinMs - 1);
    tap();
    expect(onLeave).toHaveBeenCalledOnce();
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
    readonly onLeave: ReturnType<typeof vi.fn>;
  }> {
    const heard: Heard[] = [];
    const services = servedBy(answers, heard, "token");
    const server = fakeTogetherServer();
    const video = document.createElement("video");
    document.body.append(video);
    const onOver = vi.fn();
    const onLeave = vi.fn();
    mounted = await mount(
      withServices(
        <CastReceiver
          receiving={{ cast, onOver, onLeave }}
          fileId={4}
          video={video}
          startPositionMs={0}
          hasNext={hasNext}
        />,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    return { server, video, heard, services, onOver, onLeave };
  }

  it("leads its session of one from where the sender stood", async () => {
    const { server } = await receiver();
    expect(server.heard[0]).toMatchObject({
      type: "hello",
      credential: "receiver",
      role: "video",
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

  it("goes back to the page it was on before the cast when the remote stops casting (D338 §1)", async () => {
    const { server, onOver, onLeave } = await receiver();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "stopped" });
    expect(onLeave).toHaveBeenCalledOnce();
    expect(onOver).not.toHaveBeenCalled();
  });

  it("plays on here, the remote told, when its control is taken back here (D338 §6)", async () => {
    const { server, heard, onOver, onLeave } = await receiver({
      "POST /me/casts/9/end": [200, { positionMs: 61_000, isPlaying: true }],
    });
    await server.say(welcome);
    press("Escape");
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
      reason: "takenBack",
    });
    expect(onOver).toHaveBeenCalledOnce();
    expect(onLeave).not.toHaveBeenCalled();
  });

  it("goes on receiving when another cast replaces this one", async () => {
    const { server, onOver, onLeave } = await receiver();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "replaced" });
    expect(onOver).not.toHaveBeenCalled();
    expect(onLeave).not.toHaveBeenCalled();
  });

  it("ends the cast at the film's end and goes back to the page it was on, the remote told", async () => {
    const { server, video, heard, onLeave } = await receiver({
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
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("leaves its film's end to Up next where an episode plays next (plans/up-next.md)", async () => {
    const { server, video, heard, onOver, onLeave } = await receiver({}, true);
    await server.say(welcome);
    await act(async () => {
      video.dispatchEvent(new Event("ended"));
      await Promise.resolve();
    });
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/end")).toBe(false);
    expect(onOver).not.toHaveBeenCalled();
    expect(onLeave).not.toHaveBeenCalled();
  });
});

describe("a receiver's Up next (plans/up-next.md; D337's §9 answers, Q4)", () => {
  it("moves the cast on to the next episode, which the server hands it again", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "POST /me/casts/9/next": [200, { castId: 9, fileId: 5 }] }, heard);
    const onLeave = vi.fn();
    await act(async () => {
      moveOnOrLeave(services.api, { cast, onOver: vi.fn(), onLeave });
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /me/casts/9/next"]);
    expect(onLeave).not.toHaveBeenCalled();
  });

  it("ends the cast and goes back to the page it was on, the remote told, where it cannot move on", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /me/casts/9/next": refusal(409, "conflict"),
        "POST /me/casts/9/end": [200, { positionMs: 600_000, isPlaying: false }],
      },
      heard,
    );
    const onLeave = vi.fn();
    await act(async () => {
      moveOnOrLeave(services.api, { cast, onOver: vi.fn(), onLeave });
      for (let turn = 0; turn < 5; turn += 1) {
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
    });
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
      reason: "filmEnded",
    });
    expect(onLeave).toHaveBeenCalledOnce();
  });
});
