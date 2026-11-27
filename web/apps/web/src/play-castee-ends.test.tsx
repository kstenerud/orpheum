// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { endCast } from "./cast.ts";
import { tellCastSessionLinked, tellCastSessionLost } from "./cast-session-lost.ts";
import { CasteePlay } from "./play-castee.tsx";
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
} from "./test-castee.tsx";
import {
  bodyField,
  buttonWithText,
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
    const drawn = await castingRemote(answers, heard, { ...shown, remote: CasteePlay, onPlayHere });
    mounted = drawn.mounted;
    return { ...drawn, onPlayHere };
  }

  it("plays the film of another page on that page, in this one's place, where the cast moved on to it, paused as the receiver had it, as the cast ends (D555 §5)", async () => {
    const { services, sockets } = await casting({ "GET /files/5/up-next": [200, {}], ...catReads });
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
    });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    await settleUntil(() => services.navigation.route().screen === "play");
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
  it("swaps back to the film here as the cast ends with its receiver gone, paused, from where the receiver likely got to, two ping gaps before the roster lost it, not where the clock has moved it to since (D552 §1–2, D555 §5)", async () => {
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
      await settleUntil(() => onPlayHere.mock.calls.length > 0);
      expect(currentNotice()).toBeUndefined(); // no notice explains the end (D555 §5)
      expect(mounted!.container.textContent).not.toContain("Fire TV unreachable");
      expect(buttonWithText(mounted!.container, "Try again")).toBeUndefined();
      expect(buttonWithText(mounted!.container, "Play here")).toBeUndefined();
      expect(mounted!.container.querySelector(".cast-remote__spinner")).toBeNull();
      expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(81_000, true);
      // The cast is over: the swap back asks the server for no end.
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

  it("plays the film here, where the receiver played it, as the cast is stopped from Cast's dialog with the person alone (D446 §2, D555 §5, D564 §1)", async () => {
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

  it("has its address follow the cast to the episode it moves on to, in its page's place, so the page loaded again sends the old one nowhere (the lead's walk, 2026-11-26)", async () => {
    const heard: Heard[] = [];
    const { services, sockets } = await casting(
      { "GET /files/4/up-next": [200, catNext], "GET /files/5/up-next": [200, {}], ...catReads },
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
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 5, itemId: 3 });
    services.navigation.back();
    expect(services.navigation.route()).toEqual({ screen: "title", itemId: 40 });
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

  it("follows its cast into the Watch Together session it moved to, its casts read again and the session joined by their new credential, never saying casting ended (D555 §2, §7)", async () => {
    const moved = { ...cast, membershipCredential: "remote in the party" };
    const heard: Heard[] = [];
    const { services, sockets, onPlayHere } = await casting(
      {
        "GET /me/casts": [
          [200, { casts: [cast] }],
          [200, { casts: [moved] }],
        ],
      },
      heard,
    );
    const castsRead = (): number =>
      heard.filter((each) => each.methodAndPath === "GET /me/casts").length;
    const isEndedShown = (): boolean => mounted!.container.textContent.includes("Casting ended");
    await sockets.session.say({ type: "ended", reason: "castMoved" });
    expect(isEndedShown()).toBe(false);
    await settleUntil(() => castsRead() === 2 && sockets.session.openedCount() === 2);
    expect(isEndedShown()).toBe(false);
    await sockets.session.accept();
    const hellos = sockets.session.heard.filter((each) => each["type"] === "hello");
    expect(hellos.at(-1)).toMatchObject({ credential: "remote in the party", role: "panel" });
    await sockets.session.say(welcome);
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    expect(isEndedShown()).toBe(false);
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(services.navigation.route().screen).not.toBe("home");
    expect(buttonWithText(mounted!.container, "Play here")).toBeUndefined();
  });

  it("swaps back to the film here, paused, from its own start, saying nothing, where the server lists the cast no more (D552 §4, D555 §5)", async () => {
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
          <CasteePlay castId={9} fileId={4} onPlayHere={onPlayHere} />
        </LiveProvider>,
        { ...services, openTogetherSocket: sockets.openSocket },
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({ type: "sending", sending });
    await settleUntil(() => onPlayHere.mock.calls.length > 0);
    expect(services.navigation.route().screen).not.toBe("home");
    expect(buttonWithText(mounted.container, "Play here")).toBeUndefined();
    expect(mounted.container.textContent).not.toContain("Casting ended");
    expect(currentNotice()).toBeUndefined();
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(undefined, true);
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
          <CasteePlay castId={9} fileId={4} onPlayHere={onPlayHere} />
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

  // The Chromecast switched off while hosting: the server ends its silent cast (D555 §3).
  it("ends nothing itself as its receiver goes unreached while this tab hosts Watch Together, and plays the film here on, playing, as the server's end comes, its receiver gone (D555 §3, §5)", async () => {
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
      expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(61_000, false);
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
      expect(mounted!.container.textContent).not.toContain("unreachable");
      expect(mounted!.container.textContent).not.toContain("answering");
    } finally {
      vi.useRealTimers();
    }
  });

  // The owner, 2026-11-25, a Chromecast switched off: Chrome's Cast session ended with no Stop, and
  // the remote said nothing of it; the cast is ended by the server's receiverGone alone (D553 §5).
  it("reaches a guest receiver whose Cast session was lost, no leader shown, ends nothing itself however long, and, the server ending the cast with its receiver gone, swaps back to the film here, saying nothing (D553 §5, D554 §4, D555 §3, §5)", async () => {
    const heard: Heard[] = [];
    const { sockets, onPlayHere } = await casting(
      { "POST /me/casts/9/end": [200, { positionMs: 61_000, isPlaying: false }] },
      heard,
      { sent: { ...sending, isGuestReceiver: true } },
    );
    // The cast's session shows no leader: the hint counts (D554 §4).
    const pill = (): string | undefined =>
      mounted?.container.querySelector(".castee-play__centre .pill")?.textContent ?? undefined;
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
      const ends = (): unknown[] =>
        heard
          .filter((each) => each.methodAndPath.startsWith("POST /me/casts/"))
          .map((each) => bodyField(each, "reason"));
      expect(ends()).toEqual([]);
      await sockets.live.say({ type: "sending" });
      await sockets.session.say({ type: "ended", reason: "receiverGone" });
      await settleUntil(() => onPlayHere.mock.calls.length > 0);
      expect(mounted!.container.textContent).not.toContain("Fire TV unreachable");
      expect(buttonWithText(mounted!.container, "Play here")).toBeUndefined();
      expect(buttonWithText(mounted!.container, "Try again")).toBeUndefined();
      expect(onPlayHere).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
      act(() => {
        tellCastSessionLinked(9); // forgotten for the next test's cast 9
      });
    }
  });
});
