// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CastRemote } from "./cast-remote.tsx";
import { LiveProvider } from "./live.tsx";
import { currentNotice, dismissNotice } from "./notice.ts";
import {
  receiverLeads,
  sending,
  catNext,
  castingRemote,
  type RemoteShown,
  catReads,
} from "./test-cast-remote.tsx";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";
import type { Upward } from "./upward.ts";

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
  // from ‹time›", its time rising on its own whatever the TV then did.
  it("plays the film here at once from where the receiver stood, paused as it was, as control is taken back there, offering nothing (D385 §2)", async () => {
    const { sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "roster", roster: receiverLeads });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
    expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(61_000, true);
    expect(mounted!.container.textContent).not.toContain("took control back");
    expect(buttonWithText(mounted!.container, "Play here from 1:01")).toBeUndefined();
  });

  it("plays the film of another page on that page, in this one's place, where the cast moved on to it, paused as the receiver had it", async () => {
    const { services, sockets } = await casting({ "GET /files/5/up-next": [200, {}], ...catReads });
    await sockets.live.say({
      type: "sending",
      sending: { ...sending, fileId: 5, itemName: "Cat" },
    });
    await sockets.live.say({ type: "sending" });
    await sockets.session.say({ type: "ended", reason: "takenBack" });
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
  it("stays as the cast ends with its receiver gone, saying so with Play here alone, which plays the film here, playing, from where the receiver last stood, not where the clock has moved it to since (D552 §1, D385 §3)", async () => {
    vi.useFakeTimers({ toFake: ["performance", "setInterval", "clearInterval"] });
    try {
      const heard: Heard[] = [];
      const { sockets, onPlayHere } = await casting({}, heard);
      const playing = { state: "playing", positionMs: 61_000, rate: 1, fileId: 4 };
      await sockets.session.say({ type: "group", seq: 4, group: { ...playing, event: "played" } });
      await sockets.session.say({ type: "roster", roster: receiverLeads });
      act(() => {
        vi.advanceTimersByTime(5000);
      });
      // The receiver's connection lost: the server detaches it, and no device leads the cast.
      await sockets.session.say({ type: "roster", roster: [] });
      act(() => {
        // The server sure it is gone: its minutes, shortened, as this stand-in answers no ping.
        vi.advanceTimersByTime(20_000);
      });
      await sockets.live.say({ type: "sending" });
      await sockets.session.say({ type: "ended", reason: "receiverGone" });
      expect(onPlayHere).not.toHaveBeenCalled();
      const note = mounted!.container.querySelector(".cast-remote__note");
      expect(note?.textContent).toContain("Fire TV isn't answering, so casting ended");
      expect(buttonWithText(mounted!.container, "Try again")).toBeUndefined();
      expect(mounted!.container.querySelector(".cast-remote__spinner")).toBeNull();
      await click(buttonWithText(mounted!.container, "Play here"));
      expect(onPlayHere).toHaveBeenCalledExactlyOnceWith(66_000, false);
      // The cast is over: Play here asks the server for no end.
      expect(heard.filter((each) => each.methodAndPath.startsWith("POST /me/casts/"))).toEqual([]);
      expect(mounted!.container.textContent).not.toContain("was gone");
    } finally {
      vi.useRealTimers();
    }
  });

  it("opens the film here paused, where the receiver had it paused, as the cast is stopped from Cast's dialog (D446 §2)", async () => {
    const { sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "ended", reason: "stopped" });
    expect(onPlayHere).toHaveBeenCalledWith(61_000, true);
  });

  it("plays the film here on, where the receiver played it, as the cast is stopped from Cast's dialog (D446 §2)", async () => {
    const { sockets, onPlayHere } = await casting({});
    const playing = { state: "playing", positionMs: 61_000, rate: 1, fileId: 4 };
    await sockets.session.say({ type: "group", seq: 4, group: { ...playing, event: "played" } });
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

  it("goes home, saying in a pop-up there that the server no longer knows the cast, as its session is gone (D386 §2)", async () => {
    const { services, sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "ended", reason: "gone" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(buttonWithText(mounted!.container, "Play here from 1:01")).toBeUndefined();
    const notice = currentNotice();
    expect(notice?.text).toBe("The cast ended: the server no longer knows it.");
    if (notice !== undefined) {
      dismissNotice(notice);
    }
  });

  it("goes home with no word as its cast ends with the Watch Together session it took a part in, a normal end (D389 §1)", async () => {
    const { services, sockets, onPlayHere } = await casting({});
    await sockets.session.say({ type: "ended", reason: "sessionEnded" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(onPlayHere).not.toHaveBeenCalled();
    expect(currentNotice()).toBeUndefined();
  });

  it("goes home, saying so, where the server lists the cast no more", async () => {
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
    await settleUntil(() => services.navigation.route().screen === "home");
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(onPlayHere).not.toHaveBeenCalled();
    const notice = currentNotice();
    expect(notice?.text).toBe("The cast ended: the server no longer knows it.");
    if (notice !== undefined) {
      dismissNotice(notice);
    }
  });
});
