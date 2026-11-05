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
  moveOnOrReady,
} from "./cast-receiver.tsx";
import { playedMutedEvent } from "@orpheum/sdk";

import { leaveFullScreen, type CastAnswer } from "./cast-asked.tsx";
import { fillsWindowClass, followFillsWindow } from "./film-fullscreen.ts";
import { onCommand } from "./input-commands.ts";
import { currentNotice, dismissNotice } from "./notice.ts";
import { mount, refusal, servedBy, unanswered, type Heard, type Mounted } from "./test-dom.tsx";
import { hideTheDocument, playingVideo } from "./test-hidden.ts";
import { testServices, withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";

/** Presses key on what has focus and lets it go, as a remote's press reaches the page. */
function press(key: string, keyCode = 0): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, keyCode, bubbles: true, cancelable: true });
  act(() => {
    const target = document.activeElement ?? document.body;
    target.dispatchEvent(event);
    target.dispatchEvent(new KeyboardEvent("keyup", { key, keyCode, bubbles: true }));
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

  // A click on the receiver's video did nothing in Chrome, its video having a television's controls
  // and none of the browser's (the owner, 2026-10-27).
  it("pauses or plays at a mouse's click on the video, which a double click undoes as it leaves", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { onLeave } = await controlled();
    const video = document.createElement("video");
    document.body.append(video);
    let isPaused = false;
    Object.defineProperty(video, "paused", { configurable: true, get: () => isPaused });
    vi.spyOn(video, "pause").mockImplementation(() => {
      isPaused = true;
    });
    vi.spyOn(video, "play").mockImplementation(() => {
      isPaused = false;
      return Promise.resolve();
    });
    const clickWith = (pointerType: string): void => {
      act(() => {
        video.dispatchEvent(
          new PointerEvent("click", { bubbles: true, cancelable: true, detail: 1, pointerType }),
        );
      });
    };
    clickWith("mouse");
    expect(isPaused).toBe(true);
    vi.advanceTimersByTime(doublePressWithinMs + 1);
    clickWith("touch"); // a tap opens the controls alone, as on an ordinary page on a phone
    expect(isPaused).toBe(true);
    vi.advanceTimersByTime(doublePressWithinMs + 1);
    clickWith("mouse");
    expect(isPaused).toBe(false);
    vi.advanceTimersByTime(doublePressWithinMs + 1);
    clickWith("mouse");
    expect(isPaused).toBe(true);
    expect(onLeave).not.toHaveBeenCalled();
    vi.advanceTimersByTime(doublePressWithinMs - 1);
    clickWith("mouse");
    expect(isPaused).toBe(false); // playing on, as before the double click
    expect(onLeave).toHaveBeenCalledOnce();
    video.remove();
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
  itemName: "Metropolis",
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
    readonly onLeave: ReturnType<typeof vi.fn>;
  }> {
    const heard: Heard[] = [];
    const services = servedBy(answers, heard, "token");
    const server = fakeTogetherServer();
    const video = document.createElement("video");
    document.body.append(video);
    const onLeave = vi.fn();
    mounted = await mount(
      withServices(
        <CastReceiver
          receiving={{ cast, onLeave }}
          fileId={4}
          video={video}
          startPositionMs={0}
          hasNext={hasNext}
          delivery="direct"
        />,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    return { server, video, heard, services, onLeave };
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

  it("pauses at once as its remote pauses, though its page is hidden, telling the log it followed at once (the owner, 2026-10-28)", async () => {
    // The clock stands still, so "at once" is 0 ms however loaded the machine.
    vi.useFakeTimers({ toFake: ["performance"] });
    const { server, video } = await receiver();
    // A computer's video, playing: its hidden page plays on (pauseWhenHidden pauses a handheld's alone).
    const isPaused = playingVideo(video, 70);
    await server.say(welcome);
    // Behind its remote's window on the same Mac, the receiver's page is hidden.
    const show = hideTheDocument();
    try {
      await server.say({
        type: "group",
        seq: 2,
        group: { state: "Paused", positionMs: 70_000, rate: 1, fileId: 4, event: "paused" },
      });
      expect(isPaused()).toBe(true);
      expect(server.heard).toContainEqual(
        expect.objectContaining({
          type: "note",
          note: { kind: "groupFollowed", seq: 2, heldMs: 0, why: "atOnce" },
        }),
      );
    } finally {
      show();
    }
  });

  it("plays as the device plays a film alone while its roster says it leads, telling its remote how it plays (D371 §1, §4, §5)", async () => {
    const { server, video } = await receiver();
    const isPaused = playingVideo(video, 61);
    const leads = [
      { id: 2, name: "owner", isHost: true, devices: [{ id: 6, role: "video", isLeader: true }] },
    ];
    await server.say({ ...welcome, roster: leads });
    // The group stands for it as its film is put on, and again as it buffers: nothing pauses it.
    for (const [seq, event] of [
      [2, "changedItem"],
      [3, "waitingFor"],
    ] as const) {
      await server.say({
        type: "group",
        seq,
        group: { state: "Waiting", positionMs: 61_000, rate: 1, fileId: 4, event },
      });
      expect(isPaused(), event).toBe(false);
    }
    expect(server.heard).toContainEqual(
      expect.objectContaining({ type: "state", playback: { delivery: "direct" } }),
    );
    const notes = server.heard.filter((each) => each["type"] === "note");
    expect(JSON.stringify(notes)).not.toContain("playedWhileHeld");
  });

  it("goes home when the remote stops casting (D384 §1)", async () => {
    const { server, onLeave } = await receiver();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "stopped" });
    expect(onLeave).toHaveBeenCalledOnce();
  });

  // The owner, staging, 2026-10-30: a Fire TV that took control back played on, and the sender's
  // "Play here from" rose on its own whatever the TV then did.
  it("ends the cast as its control is taken back here, and goes home once it has ended, the film as it stood until then (D385 §1)", async () => {
    const { server, video, heard, onLeave } = await receiver({
      "POST /me/casts/9/end": [unanswered],
    });
    const isPaused = playingVideo(video, 61);
    await server.say(welcome);
    press("Escape");
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/end")?.body).toEqual({
      reason: "takenBack",
    });
    // Until the end reaches the sender, which takes the film over from where it stands (D385 §2).
    expect(onLeave).not.toHaveBeenCalled();
    expect(isPaused()).toBe(false);
    await server.say({ type: "ended", reason: "takenBack" });
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("goes home, its receiving over, when its cast ended unseen with no one controlling it (D384 §1)", async () => {
    const { server, onLeave } = await receiver();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "receiverGone" });
    expect(onLeave).toHaveBeenCalledOnce();
    expect(currentNotice()).toBeUndefined();
  });

  // The owner, staging, 2026-10-30: the host ended the session while a guest cast the group's film
  // to the Fire TV, and the TV played on under a cast no page controlled (VQ193).
  it("goes home with no word as its cast ends with the Watch Together session it took a part in (D389 §1)", async () => {
    const { server, onLeave } = await receiver();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "sessionEnded" });
    expect(onLeave).toHaveBeenCalledOnce();
    expect(currentNotice()).toBeUndefined();
  });

  it("goes home, saying in a pop-up there that the server no longer knows its cast (D386 §2)", async () => {
    const { server, onLeave } = await receiver();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "gone" });
    expect(onLeave).toHaveBeenCalledOnce();
    const notice = currentNotice();
    expect(notice?.text).toBe("The cast ended: the server no longer knows it.");
    if (notice !== undefined) {
      dismissNotice(notice);
    }
  });

  it("waits ready for the next video at the film's end, the session going on (D364 §5)", async () => {
    const { server, video, heard, onLeave } = await receiver({
      "POST /me/casts/9/play": [200, { castId: 9, fileId: 0 }],
    });
    await server.say(welcome);
    await act(async () => {
      video.dispatchEvent(new Event("ended"));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/play")?.body).toEqual({});
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/end")).toBe(false);
    expect(onLeave).not.toHaveBeenCalled();
  });

  it("leads its person's Watch Together session as their player while they host one, hearing the cast's end on its own (D364 §11)", async () => {
    const heard: Heard[] = [];
    const services = servedBy({}, heard, "token");
    const sockets = { cast: fakeTogetherServer(), hosted: fakeTogetherServer() };
    let opened = 0;
    const video = document.createElement("video");
    document.body.append(video);
    const onLeave = vi.fn();
    mounted = await mount(
      withServices(
        <CastReceiver
          receiving={{
            cast: { ...cast, togetherCredential: "hosted" },
            onLeave,
          }}
          fileId={4}
          video={video}
          startPositionMs={0}
        />,
        {
          ...services,
          openTogetherSocket: (listeners) => {
            opened += 1;
            return opened === 1
              ? sockets.cast.openSocket(listeners)
              : sockets.hosted.openSocket(listeners);
          },
        },
      ),
    );
    await sockets.cast.accept();
    await sockets.hosted.accept();
    expect(sockets.cast.heard[0]).toMatchObject({ credential: "receiver", role: "panel" });
    expect(sockets.hosted.heard[0]).toMatchObject({ credential: "hosted", role: "video" });
    await sockets.hosted.say(welcome);
    expect(sockets.hosted.heard).toContainEqual(
      expect.objectContaining({ command: "changeItem", fileId: 4, positionMs: 61_000 }),
    );
    expect(sockets.cast.heard.some((each) => each["type"] === "command")).toBe(false);
    await sockets.cast.say({ type: "ended", reason: "stopped" });
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("leaves its film's end to Up next where an episode plays next (plans/up-next.md)", async () => {
    const { server, video, heard, onLeave } = await receiver({}, true);
    await server.say(welcome);
    await act(async () => {
      video.dispatchEvent(new Event("ended"));
      await Promise.resolve();
    });
    expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/end")).toBe(false);
    expect(onLeave).not.toHaveBeenCalled();
  });
});

describe("a receiver's Up next (plans/up-next.md; D337's §9 answers, Q4)", () => {
  it("moves the cast on to the next episode, which the server hands it again", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "POST /me/casts/9/next": [200, { castId: 9, fileId: 5 }] }, heard);
    const onLeave = vi.fn();
    await act(async () => {
      moveOnOrReady(services.api, { cast, onLeave });
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /me/casts/9/next"]);
    expect(onLeave).not.toHaveBeenCalled();
  });

  it("waits ready for the next video where it cannot move on, and goes home where the cast is gone (D364 §5, D384 §1)", async () => {
    for (const [played, isLeft] of [
      [[200, { castId: 9, fileId: 0 }], false],
      [refusal(404, "not_found"), true],
    ] as const) {
      const heard: Heard[] = [];
      const services = servedBy(
        { "POST /me/casts/9/next": refusal(409, "conflict"), "POST /me/casts/9/play": played },
        heard,
      );
      const onLeave = vi.fn();
      await act(async () => {
        moveOnOrReady(services.api, { cast, onLeave });
        for (let turn = 0; turn < 5; turn += 1) {
          await new Promise((resolve) => setTimeout(resolve, 0));
        }
      });
      expect(heard.find((each) => each.methodAndPath === "POST /me/casts/9/play")?.body).toEqual(
        {},
      );
      expect(heard.some((each) => each.methodAndPath === "POST /me/casts/9/end")).toBe(false);
      expect(onLeave).toHaveBeenCalledTimes(isLeft ? 1 : 0);
    }
  });
});

describe("a receiver's sound and full screen as its cast starts (D358 §2, §5)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    document.body.replaceChildren();
    vi.useRealTimers();
  });

  /** A receiver of cast inside a player's stage, answered by answer, connected and welcomed. */
  async function started(answer?: CastAnswer): Promise<{
    readonly server: FakeTogetherServer;
    readonly video: HTMLVideoElement;
    readonly stage: HTMLElement;
    readonly onLeave: ReturnType<typeof vi.fn>;
  }> {
    const services = servedBy(
      { "POST /me/casts/9/end": [200, { positionMs: 61_000, isPlaying: true }] },
      [],
      "token",
    );
    const server = fakeTogetherServer();
    const stage = document.createElement("div");
    stage.className = "film-player__stage";
    const video = document.createElement("video");
    stage.append(video);
    document.body.append(stage);
    const onLeave = vi.fn();
    const receiving = {
      cast,
      onLeave,
      ...(answer === undefined ? {} : { answer }),
    };
    mounted = await mount(
      withServices(
        <CastReceiver receiving={receiving} fileId={4} video={video} startPositionMs={0} />,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say(welcome);
    return { server, video, stage, onLeave };
  }

  /** The browser refusing the film's sound: it plays muted, and says so (playOrPlayMuted). */
  function refuseSound(video: HTMLVideoElement): void {
    act(() => {
      // eslint-disable-next-line no-param-reassign -- muted as the browser's refusal leaves it
      video.muted = true;
      video.dispatchEvent(new Event(playedMutedEvent));
    });
  }

  /** The notes the receiver sent of kind. */
  function notesOf(server: FakeTogetherServer, kind: string): unknown[] {
    return server.heard
      .filter((each) => each["type"] === "note")
      .map((each) => each["note"])
      .filter(
        (note) => typeof note === "object" && note !== null && Reflect.get(note, "kind") === kind,
      );
  }

  it("tells those following its stage that the stage fills the window, for the overlay to draw over the film (D407 §12)", async () => {
    const told = vi.fn();
    const stop = followFillsWindow(told);
    const { stage } = await started();
    expect(stage.classList.contains(fillsWindowClass)).toBe(true);
    expect(told).toHaveBeenCalled();
    told.mockClear();
    mounted?.unmount();
    mounted = undefined;
    stop();
    expect(stage.classList.contains(fillsWindowClass)).toBe(false);
    expect(told).toHaveBeenCalled();
  });

  it("says over the film that a press turns the sound on, and turns it on at a key, which does nothing else", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { server, video, stage, onLeave } = await started();
    refuseSound(video);
    expect(stage.querySelector(".cast-receiver__sound-on")?.textContent).toBe(
      "Click to turn the sound on",
    );
    const heard = vi.fn();
    const stop = onCommand(document, heard, { isCapture: true });
    const ok = press("Enter");
    tap(); // the click a TV browser makes of the same OK
    expect(ok.defaultPrevented).toBe(true);
    expect(heard).not.toHaveBeenCalled(); // the player's own controls never heard it
    expect(video.muted).toBe(false);
    expect(stage.querySelector(".cast-receiver__sound-on")).toBeNull();
    expect(notesOf(server, "unmuted")[0]).toMatchObject({ kind: "unmuted", by: "key" });
    vi.advanceTimersByTime(doublePressWithinMs - 1);
    press("Enter"); // the first OK of a double: the one that turned the sound on is not counted
    stop();
    expect(onLeave).not.toHaveBeenCalled();
    expect(heard).toHaveBeenCalledOnce();
  });

  it("turns the sound on at a tap, and says to press OK on a television", async () => {
    const userAgent = Object.getOwnPropertyDescriptor(Navigator.prototype, "userAgent");
    Object.defineProperty(Navigator.prototype, "userAgent", {
      configurable: true,
      get: () => "Mozilla/5.0 (Linux; Android 9; AFTKA) AppleWebKit/537.36 Silk/131.4.1",
    });
    try {
      const { server, video, stage } = await started();
      refuseSound(video);
      expect(stage.querySelector(".cast-receiver__sound-on")?.textContent).toBe(
        "Press OK to turn the sound on",
      );
      act(() => {
        document.body.dispatchEvent(
          new PointerEvent("click", {
            bubbles: true,
            cancelable: true,
            detail: 1,
            pointerType: "touch",
          }),
        );
      });
      expect(video.muted).toBe(false);
      expect(notesOf(server, "unmuted")[0]).toMatchObject({ by: "tap" });
    } finally {
      if (userAgent !== undefined) {
        Object.defineProperty(Navigator.prototype, "userAgent", userAgent);
      }
    }
  });

  it("leaves Back to leave controlled mode, the sound still off", async () => {
    const { video, onLeave } = await started();
    refuseSound(video);
    press("Escape");
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(onLeave).toHaveBeenCalledOnce();
    expect(video.muted).toBe(true);
  });

  it("tells the log what full screen the Play press got, and that the film played with sound", async () => {
    const { server, video } = await started({
      castId: 9,
      by: "key",
      fullScreen: Promise.resolve("refused"),
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(notesOf(server, "fullScreen")).toEqual([
      { kind: "fullScreen", why: "refused", by: "key" },
    ]);
    act(() => {
      video.currentTime = 61;
      video.dispatchEvent(new Event("playing"));
      video.dispatchEvent(new Event("playing"));
    });
    expect(notesOf(server, "playedWithSound")).toEqual([
      expect.objectContaining({ kind: "playedWithSound", positionMs: 61_000 }),
    ]);
  });
});

describe("a receiver's full screen left by the browser, not the page (D338 §6)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    document.body.replaceChildren();
    Reflect.deleteProperty(document, "fullscreenElement");
    Reflect.deleteProperty(document, "exitFullscreen");
  });

  /** A receiver of cast, connected and welcomed, and what it was told and asked. */
  async function receiving(): Promise<{
    readonly server: FakeTogetherServer;
    readonly heard: Heard[];
    readonly onLeave: ReturnType<typeof vi.fn>;
  }> {
    const heard: Heard[] = [];
    const services = servedBy(
      { "POST /me/casts/9/end": [200, { positionMs: 61_000, isPlaying: true }] },
      heard,
      "token",
    );
    const server = fakeTogetherServer();
    const video = document.createElement("video");
    document.body.append(video);
    const onLeave = vi.fn();
    mounted = await mount(
      withServices(
        <CastReceiver receiving={{ cast, onLeave }} fileId={4} video={video} startPositionMs={0} />,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say(welcome);
    return { server, heard, onLeave };
  }

  /** The page full screen, or not, as the browser says it (fullscreenElement). */
  function isFullScreen(isIt: boolean): void {
    Object.defineProperty(document, "fullscreenElement", {
      value: isIt ? document.documentElement : null,
      configurable: true,
    });
  }

  /** The browser saying the page's full screen changed, and what follows it settled. */
  async function fullScreenChanged(): Promise<void> {
    await act(async () => {
      document.dispatchEvent(new Event("fullscreenchange"));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }

  /** The full-screen notes the receiver sent. */
  function fullScreenNotes(server: FakeTogetherServer): unknown[] {
    return server.heard
      .filter((each) => each["type"] === "note")
      .map((each) => each["note"])
      .filter(
        (note) =>
          typeof note === "object" && note !== null && Reflect.get(note, "kind") === "fullScreen",
      );
  }

  it("takes control back at once when a Fire TV's Back or Escape leaves full screen, telling the log", async () => {
    // In full screen a Fire TV's first Back only left full screen; a second took control back (the
    // owner, 2026-10-27).
    const { server, heard, onLeave } = await receiving();
    isFullScreen(false);
    await fullScreenChanged();
    expect(onLeave).toHaveBeenCalledOnce();
    expect(heard.filter((each) => each.methodAndPath === "POST /me/casts/9/end")).toEqual([
      { methodAndPath: "POST /me/casts/9/end", body: { reason: "takenBack" } },
    ]);
    expect(fullScreenNotes(server)).toEqual([{ kind: "fullScreen", why: "left" }]);
  });

  it("takes control back once where Escape reaches the page as well as leaving full screen", async () => {
    const { heard, onLeave } = await receiving();
    press("Escape");
    isFullScreen(false);
    await fullScreenChanged();
    expect(onLeave).toHaveBeenCalledOnce();
    expect(heard.filter((each) => each.methodAndPath === "POST /me/casts/9/end")).toHaveLength(1);
  });

  it("keeps control where full screen ended as the page took its element away, the next video opening (D364 §3)", async () => {
    const stage = document.createElement("div");
    document.body.append(stage);
    Object.defineProperty(document, "fullscreenElement", { value: stage, configurable: true });
    const { server, onLeave } = await receiving();
    stage.remove(); // the last film's Play page, its stage full screen, gone for the next
    isFullScreen(false);
    await fullScreenChanged();
    expect(onLeave).not.toHaveBeenCalled();
    expect(fullScreenNotes(server)).toEqual([]);
  });

  it("keeps control where the page itself left full screen, as Stop casting and the film's end do", async () => {
    const { server, onLeave } = await receiving();
    isFullScreen(true);
    Object.defineProperty(document, "exitFullscreen", {
      value: () => Promise.resolve(),
      configurable: true,
    });
    leaveFullScreen(document);
    isFullScreen(false);
    await fullScreenChanged();
    expect(onLeave).not.toHaveBeenCalled();
    expect(fullScreenNotes(server)).toEqual([]);
    // Only that one leaving was the page's.
    isFullScreen(true);
    await fullScreenChanged();
    expect(onLeave).not.toHaveBeenCalled();
    isFullScreen(false);
    await fullScreenChanged();
    expect(onLeave).toHaveBeenCalledOnce();
  });
});
