// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { endCast } from "./cast.ts";
import { tellCastSessionLinked, tellCastSessionLost } from "./cast-session-lost.ts";
import { useTellsTheHost } from "./cast-remote-ends.ts";
import { CastRemote } from "./cast-remote.tsx";
import { LiveProvider } from "./live.tsx";
import { currentNotice, dismissNotice } from "./notice.ts";
import {
  cast,
  receiverLeads,
  sending,
  catNext,
  castingRemote,
  type RemoteShown,
  catReads,
  filmReads,
  welcome,
} from "./test-cast-remote.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, liveAndSession } from "./test-together.ts";
import type { Upward } from "./upward.ts";

/** Longer than any wait the remote once gave its receiver up after (D554 §1), none now (D555 §3). */
const longWaitMs = 30_000;

describe("a Play page's video area as the cast's remote as its cast is taken back, ends or moves on (D364 §8; D385)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.sessionStorage.clear();
  });

  /** The remote on film 4's Play page (castingRemote), the one this describe unmounts. */
  async function casting(
    answers: Parameters<typeof castingRemote>[0],
    heard?: Heard[],
    shown: RemoteShown = {},
  ): Promise<Awaited<ReturnType<typeof castingRemote>> & { onPlayHere: ReturnType<typeof vi.fn> }> {
    const onPlayHere = vi.fn();
    const drawn = await castingRemote(answers, heard, { ...shown, remote: CastRemote, onPlayHere });
    mounted = drawn.mounted;
    return { ...drawn, onPlayHere };
  }

  // The owner, staging, 2026-10-30: after a Fire TV took control back, the page offered "Play here
  // from ‹time›", its time rising on its own whatever the TV then did. The owner, 2026-11-25:
  // "if the caster didn't stop the cast, it should say that the other side stopped it or is gone,
  // and then offer to continue playback on the caster device."
  it("stays as control is taken back on the receiver, saying it took control with Play here, which plays the film here from where the receiver stood, paused as it was (D552 §4, D553 §2)", async () => {
    const heard: Heard[] = [];
    const { sockets, onPlayHere } = await casting({}, heard);
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    expect(onPlayHere).not.toHaveBeenCalled();
    const note = mounted!.container.querySelector(".cast-remote__note");
    expect(note?.textContent).toContain("Fire TV took control");
    expect(buttonWithText(mounted!.container, "Try again")).toBeUndefined();
    expect(mounted!.container.textContent).not.toContain("took control back");
    expect(buttonWithText(mounted!.container, "Play here from 1:01")).toBeUndefined();
    await click(buttonWithText(mounted!.container, "Play here"));
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(61_000, true);
    // The cast is over: Play here asks the server for no end.
    expect(heard.filter((each) => each.methodAndPath.startsWith("POST /me/casts/"))).toEqual([]);
  });

  it("plays the film of another page on that page, in this one's place, where the cast moved on to it, paused as the receiver had it", async () => {
    const { services, sockets } = await casting({ "GET /files/5/up-next": [200, {}], ...catReads });
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
    });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    await click(buttonWithText(mounted!.container, "Play here"));
    expect(services.navigation.route()).toEqual({
      screen: "play",
      fileId: 5,
      atMs: 61_000,
      shouldPlayHere: true,
      isPaused: true,
    });
  });

  // The owner, 2026-10-30: "We could just have it take over at the last known playback position once
  // it's sure the castee is gone."
  // The owner, 2026-11-25, as the cast took the film over by itself two minutes after the television
  // was switched off: "not auto-changeover. The user has to select play here if they want to
  // continue … but the cast session is over."
  it("stays as the cast ends with its receiver gone, saying so with Play here alone, which plays the film here, playing, from where the receiver likely got to, two ping gaps before the roster lost it, not where the clock has moved it to since (D552 §1–2, D385 §3)", async () => {
    vi.useFakeTimers({ toFake: ["performance", "setInterval", "clearInterval"] });
    try {
      const heard: Heard[] = [];
      const { sockets, onPlayHere } = await casting({}, heard);
      const playing = { state: "playing", positionMs: 61_000, rate: 1, fileId: 4 };
      await sockets.session.say({ type: "group", seq: 4, group: { ...playing, event: "played" } });
      await sockets.session.say({ type: "roster", roster: receiverLeads });
      act(() => {
        vi.advanceTimersByTime(25_000);
      });
      await sockets.session.say({ type: "roster", roster: receiverLeads }); // heard: its pings are answered
      act(() => {
        vi.advanceTimersByTime(25_000);
      });
      // The receiver's connection lost, its silence past the server's limit of three pings: the
      // server detaches it, and no device leads the cast. Its last word came 45 s before this, and it
      // likely played on for one more ping gap: to 61 + 50 − 30 s.
      await sockets.session.say({ type: "roster", roster: [] });
      act(() => {
        // The server sure it is gone: its minutes, shortened, as this stand-in answers no ping.
        vi.advanceTimersByTime(20_000);
      });
      await sockets.live.say({ type: "sending" });
      await sockets.session.say({ type: "ended", reason: "receiverGone" });
      expect(onPlayHere).not.toHaveBeenCalled();
      expect(currentNotice()).toBeUndefined(); // not hosting Watch Together: nothing plays by itself
      const note = mounted!.container.querySelector(".cast-remote__note");
      expect(note?.textContent).toContain("Fire TV unreachable");
      expect(buttonWithText(mounted!.container, "Try again")).toBeUndefined();
      expect(mounted!.container.querySelector(".cast-remote__spinner")).toBeNull();
      await click(buttonWithText(mounted!.container, "Play here"));
      expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(81_000, false);
      // The cast is over: Play here asks the server for no end.
      expect(heard.filter((each) => each.methodAndPath.startsWith("POST /me/casts/"))).toEqual([]);
      expect(mounted!.container.textContent).not.toContain("was gone");
    } finally {
      vi.useRealTimers();
    }
  });

  /** This tab's Stop asked and its answer failed: where the film resumes is the receiver's. */
  const failedEnd = { "POST /me/casts/9/end": [500, {}] } as const;

  it("opens the film here paused, where the receiver had it paused, as the cast is stopped from Cast's dialog (D446 §2)", async () => {
    const { services, sockets, onPlayHere } = await casting(failedEnd);
    await endCast(services.api, 9, "stopped").catch(() => undefined); // this tab's Stop, unanswered
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(onPlayHere).toHaveBeenCalledWith(61_000, true);
  });

  it("plays the film here on, where the receiver played it, as the cast is stopped from Cast's dialog (D446 §2)", async () => {
    const { services, sockets, onPlayHere } = await casting(failedEnd);
    const playing = { state: "playing", positionMs: 61_000, rate: 1, fileId: 4 };
    await sockets.session.say({ type: "group", seq: 4, group: { ...playing, event: "played" } });
    await endCast(services.api, 9, "stopped").catch(() => undefined); // this tab's Stop, unanswered
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(onPlayHere).toHaveBeenCalledOnce();
    expect(onPlayHere.mock.calls[0]?.[1]).toBe(false);
  });

  /** File 4's page's way up, its film's page, item 40's, as a Play page's Up is. */
  const filmsPage: Upward = {
    crumbs: [
      {
        label: "Metropolis",
        to: { screen: "title", itemId: 40 },
        isIt: (route) => route.screen === "title" && route.itemId === 40,
      },
    ],
    here: "Metropolis",
  };

  it("leaves for the film's page, back to its entry, as its receiver has the cast play nothing at the film's end (the owner, 2026-11-17)", async () => {
    // The plan file's "A film played to its end with nothing next goes back to its page", its cast
    // part: as a local play leaves, the cast going on, ready.
    const { services, sockets, onPlayHere } = await casting({}, [], { upward: filmsPage });
    services.navigation.go({ screen: "title", itemId: 40 });
    services.navigation.go({ screen: "play", fileId: 4 });
    await sockets.live.say({ type: "sending", sending: { ...sending, fileId: 0, itemName: "" } });
    expect(services.navigation.route()).toEqual({ screen: "title", itemId: 40 });
    expect(onPlayHere).not.toHaveBeenCalled();
  });

  it("leaves for the episode the cast had moved on to, in its page's place, as that episode ends with nothing next", async () => {
    // The plan file's "A film played to its end with nothing next goes back to its page", the
    // case it left open: the cast moved on by Up next, the page above is the episode that ended.
    const heard: Heard[] = [];
    const { services, sockets } = await casting(
      {
        "GET /files/4/up-next": [200, catNext],
        "GET /files/5/up-next": [200, {}],
        ...catReads,
      },
      heard,
      { upward: filmsPage },
    );
    services.navigation.go({ screen: "title", itemId: 40 });
    services.navigation.go({ screen: "play", fileId: 4 });
    await settleUntil(() => heard.some((each) => each.methodAndPath === "GET /files/4/up-next"));
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
    });
    await sockets.live.say({ type: "sending", sending: { ...sending, fileId: 0, itemName: "" } });
    expect(services.navigation.route()).toEqual({ screen: "title", itemId: 3 });
  });

  it("stays on its page as a cast that has played nothing from its start is said again", async () => {
    const { services, sockets } = await casting({}, [], {
      upward: filmsPage,
      sent: { ...sending, fileId: 0, itemName: "" },
    });
    services.navigation.go({ screen: "play", fileId: 4 });
    await sockets.live.say({ type: "sending", sending: { ...sending, fileId: 0, itemName: "" } });
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 4 });
  });

  it("goes home, playing nothing here, as a cast with nothing on is stopped (D446 §2)", async () => {
    const { services, sockets, onPlayHere } = await casting({});
    await sockets.live.say({ type: "sending", sending: { ...sending, fileId: 0 } });
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(onPlayHere).not.toHaveBeenCalled();
  });

  it("goes home, playing nothing here, as a cast with nothing on ends with its receiver gone (D446 §2, D552 §1)", async () => {
    const { services, sockets, onPlayHere } = await casting({});
    await sockets.live.say({ type: "sending", sending: { ...sending, fileId: 0 } });
    await sockets.session.say({ type: "ended", reason: "receiverGone" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(onPlayHere).not.toHaveBeenCalled();
  });

  it("stays as the server no longer knows the cast, its session gone, saying casting ended with Play here alone and no pop-up (D552 §4, amending D386 §2)", async () => {
    const { services, sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "ended", reason: "gone" });
    expect(services.navigation.route().screen).not.toBe("home");
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(buttonWithText(mounted!.container, "Play here from 1:01")).toBeUndefined();
    expect(mounted!.container.querySelector(".cast-remote__note")?.textContent).toContain(
      "Casting ended",
    );
    expect(currentNotice()).toBeUndefined();
    await click(buttonWithText(mounted!.container, "Play here"));
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(61_000, true);
  });

  it("goes home, saying in a pop-up there that the server no longer knows the cast, as a cast with nothing on has its session gone (D386 §2)", async () => {
    const { services, sockets, onPlayHere } = await casting({});
    await sockets.live.say({ type: "sending", sending: { ...sending, fileId: 0 } });
    await sockets.session.say({ type: "ended", reason: "gone" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(onPlayHere).not.toHaveBeenCalled();
    const notice = currentNotice();
    expect(notice?.text).toBe("The cast ended: the server no longer knows it.");
    if (notice !== undefined) {
      dismissNotice(notice);
    }
  });

  it("stays as the cast is stopped by another tab of this device, which this tab did not ask, saying casting ended with Play here alone (D552 §4)", async () => {
    const { sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(mounted!.container.querySelector(".cast-remote__note")?.textContent).toContain(
      "Casting ended",
    );
    await click(buttonWithText(mounted!.container, "Play here"));
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(61_000, true);
  });

  it("stays as its cast ends with the Watch Together session it took a part in, saying casting ended, with no pop-up (D552 §4, amending D389 §1)", async () => {
    const { services, sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "ended", reason: "sessionEnded" });
    expect(services.navigation.route().screen).not.toBe("home");
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(currentNotice()).toBeUndefined();
    expect(mounted!.container.querySelector(".cast-remote__note")?.textContent).toContain(
      "Casting ended",
    );
  });

  it("stays, saying casting ended, where the server lists the cast no more, Play here alone playing the film here from its own start (D552 §4)", async () => {
    const services = servedBy(
      {
        "GET /me/casts": [200, { casts: [] }],
        "GET /files/4/up-next": [200, {}],
        "GET /files/4/segments": [200, { segments: [] }],
      },
      [],
      "token",
    );
    const sockets = liveAndSession();
    const onPlayHere = vi.fn();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastRemote castId={9} fileId={4} onPlayHere={onPlayHere} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "sending", sending });
    await settleUntil(
      () => buttonWithText(mounted?.container ?? document.body, "Play here") !== undefined,
    );
    expect(services.navigation.route().screen).not.toBe("home");
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(mounted.container.querySelector(".cast-remote__note")?.textContent).toContain(
      "Casting ended",
    );
    expect(currentNotice()).toBeUndefined();
    await click(buttonWithText(mounted.container, "Play here"));
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(undefined, false);
  });

  /**
   * The remote on film 4's Play page while this tab hosts Watch Together (D364 §11): its sockets the
   * live connection's, the hosted session's, its film paused at 61 s, and the cast's.
   */
  async function castingWhileHosting(heard: Heard[] = []): Promise<{
    readonly castSession: ReturnType<typeof fakeTogetherServer>;
    readonly onPlayHere: ReturnType<typeof vi.fn>;
  }> {
    window.sessionStorage.setItem(
      "orpheum.together.host",
      JSON.stringify({ sessionId: 3, credential: "host-credential" }),
    );
    const live = fakeTogetherServer();
    const hosted = fakeTogetherServer();
    const castSession = fakeTogetherServer();
    let opened = 0;
    const services = servedBy(
      {
        "GET /me/casts": [200, { casts: [cast] }],
        "GET /files/4/up-next": [200, {}],
        "POST /me/casts/9/end": [200, { positionMs: 61_000, isPlaying: false }],
        ...filmReads,
      },
      heard,
      "token",
    );
    const onPlayHere = vi.fn();
    mounted = await mount(
      withServices(
        <LiveProvider>
          <CastRemote castId={9} fileId={4} onPlayHere={onPlayHere} />
        </LiveProvider>,
        {
          ...services,
          openTogetherSocket: (listeners) => {
            opened += 1;
            return ([live, hosted][opened - 1] ?? castSession).openSocket(listeners);
          },
        },
      ),
    );
    await live.accept();
    await live.say({ type: "sending", sending });
    await hosted.accept();
    await hosted.say({ ...welcome, sessionId: 3 });
    await castSession.accept();
    await castSession.say(welcome);
    await castSession.say({ type: "roster", roster: receiverLeads });
    await live.say({ type: "sending" });
    return { castSession, onPlayHere };
  }

  // The owner, 2026-11-25: hosting Watch Together, the session is an obligation; the host's
  // television lost, the host's own player goes on with it, saying why (D553 §3).
  it("plays the film here by itself, from where it stood, saying why in a notice, as a cast ends with its receiver gone while this tab hosts Watch Together (D553 §3)", async () => {
    const { castSession, onPlayHere } = await castingWhileHosting();
    await castSession.say({ type: "ended", reason: "receiverGone" });
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(61_000, true);
    const notice = currentNotice();
    expect(notice?.text).toBe("Fire TV unreachable. Playing here so your session goes on");
    if (notice !== undefined) {
      dismissNotice(notice);
    }
  });

  // The Chromecast switched off while hosting: the server ends its silent cast (D555 §3).
  it("ends nothing itself as its receiver goes unreached while this tab hosts Watch Together, and plays the film here as the server's end comes, its receiver gone (D555 §3, D553 §3)", async () => {
    const heard: Heard[] = [];
    const { castSession, onPlayHere } = await castingWhileHosting(heard);
    const ends = (): unknown[] =>
      heard
        .filter((each) => each.methodAndPath === "POST /me/casts/9/end")
        .map((each) => bodyField(each, "reason"));
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      await castSession.say({ type: "roster", roster: [] }); // the receiver no longer leads
      act(() => {
        vi.advanceTimersByTime(longWaitMs);
      });
      expect(ends()).toEqual([]);
      expect(buttonWithText(mounted!.container, "Try again")).toBeUndefined();
      await castSession.say({ type: "ended", reason: "receiverGone" });
      await settleUntil(() => onPlayHere.mock.calls.length > 0);
      expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(61_000, true);
    } finally {
      vi.useRealTimers();
      const notice = currentNotice();
      if (notice !== undefined) {
        dismissNotice(notice);
      }
    }
  });

  // The owner's Fire TV, 2026-11-25: a receiver page held for its press (D468 §2) is connected and
  // answering with no place yet; the remote said it wasn't answering.
  it("gives up no receiver that is heard but holds no place yet, held for its press, and says nothing of it not answering (D554 §1, D468 §2)", async () => {
    const heard: Heard[] = [];
    const noPlace = { ...welcome, group: { state: "paused", rate: 1, fileId: 4 } };
    const { sockets } = await casting(
      { "POST /me/casts/9/end": [200, { positionMs: 0, isPlaying: false }] },
      heard,
      { welcome: noPlace },
    );
    const devices = { type: "devices", inFrontDeviceIds: [], receivingDeviceIds: [2] };
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    await sockets.live.say({ ...devices, deviceIds: [1, 2] });
    const ends = (): unknown[] =>
      heard
        .filter((each) => each.methodAndPath === "POST /me/casts/9/end")
        .map((each) => bodyField(each, "reason"));
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      act(() => {
        vi.advanceTimersByTime(longWaitMs);
      });
      expect(ends()).toEqual([]);
      expect(mounted!.container.querySelector(".cast-remote__note")).toBeNull();
      expect(mounted!.container.textContent).not.toContain("answering");
    } finally {
      vi.useRealTimers();
    }
  });

  // The owner's phone, 2026-11-25: "When I press the OK button, nothing happens (it doesn't go
  // away)." The end met again, a remote shown again for the same ended cast, showed it anew (D554 §3).
  it("tells the host once per cast end, so that its notice, dismissed, stays closed as the same end is met again (D554 §3)", async () => {
    const played = vi.fn();
    const ended = { castId: 77, reason: "receiverGone", isHosting: true, name: "Fire TV" };
    function MeetsTheEnd(): null {
      const takeOver = useTellsTheHost(played, ended);
      useEffect(() => {
        takeOver(61_000, true);
      }, [takeOver]);
      return null;
    }
    const first = await mount(<MeetsTheEnd />);
    const notice = currentNotice();
    expect(notice?.text).toBe("Fire TV unreachable. Playing here so your session goes on");
    if (notice !== undefined) {
      dismissNotice(notice); // its OK
    }
    first.unmount();
    const again = await mount(<MeetsTheEnd />);
    const shownAgain = currentNotice();
    again.unmount();
    if (shownAgain !== undefined) {
      dismissNotice(shownAgain); // so no later test finds it
    }
    expect(shownAgain).toBeUndefined();
    expect(played).toHaveBeenCalledWith(61_000, true); // the film played here each time, told once
  });

  it("stays, saying the receiver took control with Play here, playing nothing by itself, as control is taken back while this tab hosts Watch Together (D553 §3)", async () => {
    const { castSession, onPlayHere } = await castingWhileHosting();
    await castSession.say({ type: "ended", reason: "takenBack" });
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(currentNotice()).toBeUndefined();
    expect(mounted!.container.querySelector(".cast-remote__note")?.textContent).toContain(
      "Fire TV took control",
    );
  });

  // The owner, 2026-11-25, a Chromecast switched off: Chrome's Cast session ended with no Stop, and
  // the remote said nothing of it; the cast is ended by the server's receiverGone alone (D553 §5).
  it("reaches a guest receiver whose Cast session was lost, no leader shown, ends nothing itself however long, and, the server ending the cast with its receiver gone, says it is unreachable, with Play here (D553 §5, D554 §4, D555 §3)", async () => {
    const heard: Heard[] = [];
    const { sockets } = await casting(
      { "POST /me/casts/9/end": [200, { positionMs: 61_000, isPlaying: false }] },
      heard,
      { sent: { ...sending, isGuestReceiver: true } },
    );
    // The cast's session shows no leader: the hint counts (D554 §4).
    const pill = (): string | undefined =>
      mounted?.container.querySelector(".cast-remote__centre .pill")?.textContent ?? undefined;
    expect(pill()).toBeUndefined();
    vi.useFakeTimers({
      toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval", "performance"],
      shouldAdvanceTime: true,
    });
    try {
      act(() => {
        tellCastSessionLost(9);
      });
      expect(pill()).toBe("Reaching Fire TV…");
      act(() => {
        vi.advanceTimersByTime(longWaitMs);
      });
      const note = (): string | undefined =>
        mounted?.container.querySelector(".cast-remote__note")?.textContent ?? undefined;
      const ends = (): unknown[] =>
        heard
          .filter((each) => each.methodAndPath.startsWith("POST /me/casts/"))
          .map((each) => bodyField(each, "reason"));
      expect(ends()).toEqual([]);
      await sockets.live.say({ type: "sending" });
      await sockets.session.say({ type: "ended", reason: "receiverGone" });
      expect(note()).toContain("Fire TV unreachable");
      expect(buttonWithText(mounted!.container, "Play here")).toBeDefined();
      expect(buttonWithText(mounted!.container, "Try again")).toBeUndefined();
    } finally {
      vi.useRealTimers();
      act(() => {
        tellCastSessionLinked(9); // forgotten for the next test's cast 9
      });
    }
  });
});
