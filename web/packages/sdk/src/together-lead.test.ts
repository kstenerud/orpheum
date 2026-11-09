// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { everyRight } from "./together.ts";
import { browserPauseWindowMs, checkDriftEveryMs, scrubPauseWindowMs } from "./together-follow.ts";
import { playerMovedItselfEvent } from "./together-player-events.ts";
import { ownMarkLastsMs } from "./together-own-moves.ts";
import {
  FakeMedia,
  FakePage,
  fakeConnection,
  paused,
  followGroup,
  stopEveryFollower,
} from "./test-follow.ts";

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 0;
});

afterEach(() => {
  stopEveryFollower();
  vi.useRealTimers();
});

describe("a leading player's person's moves", () => {
  it("turns the host's own play, pause and seek into commands, but not its own following", () => {
    const media = new FakeMedia();
    const { connection, setGroup, commands } = fakeConnection(paused);
    followGroup(media, connection, {
      fileId: 5,
      leads: everyRight,
      monotonicMs,
    });
    media.fire("seeking"); // the follower's own seek to the group's position
    media.fire("seeked");
    expect(commands).toEqual([]);
    media.paused = false;
    media.fire("play");
    expect(commands).toEqual([{ command: "play" }]);
    setGroup({ ...paused, state: "playing" }, 0);
    media.paused = true;
    media.fire("pause");
    vi.advanceTimersByTime(browserPauseWindowMs);
    expect(commands.at(-1)).toEqual({ command: "pause" });
    setGroup({ ...paused, state: "paused" }, 0);
    media.currentTime = 600;
    media.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands.at(-1)).toEqual({ command: "seek", positionMs: 600_000 });
  });

  it("sends a guest's moves their link gives, and puts back the ones it does not (D271 §4)", () => {
    const pausingGuest = new FakeMedia();
    const pausing = fakeConnection(paused);
    followGroup(pausingGuest, pausing.connection, {
      fileId: 5,
      leads: { canPlayPause: true, canSeek: false },
      monotonicMs,
    });
    pausingGuest.fire("seeked"); // the follower's own seek to the group's position
    pausingGuest.paused = false;
    pausingGuest.fire("play");
    expect(pausing.commands).toEqual([{ command: "play" }]);
    pausing.setGroup({ ...paused, state: "paused" }, 0);
    pausingGuest.paused = true;
    pausingGuest.currentTime = 600;
    pausingGuest.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(pausing.commands).toEqual([{ command: "play" }]);
    expect(pausingGuest.currentTime).toBe(paused.positionMs / 1000);

    const seekingGuest = new FakeMedia();
    const seeking = fakeConnection(paused);
    followGroup(seekingGuest, seeking.connection, {
      fileId: 5,
      leads: { canPlayPause: false, canSeek: true },
      monotonicMs,
    });
    seekingGuest.fire("seeked");
    seekingGuest.paused = false;
    seekingGuest.fire("play");
    expect(seeking.commands).toEqual([]);
    expect(seekingGuest.paused).toBe(true);
    seekingGuest.currentTime = 600;
    seekingGuest.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(seeking.commands).toEqual([{ command: "seek", positionMs: 600_000 }]);
  });

  it("takes the pause an iPhone makes as its full-screen player closes for no pause, whichever comes first", () => {
    for (const order of [
      ["pause", "webkitendfullscreen"],
      ["webkitendfullscreen", "pause"],
    ] as const) {
      const media = new FakeMedia();
      const { connection, commands } = fakeConnection({ ...paused, state: "playing" });
      followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs });
      media.fire("seeked"); // the follower's own seek to the group's position
      vi.advanceTimersByTime(scrubPauseWindowMs);
      expect(media.paused).toBe(false);
      for (const event of order) {
        if (event === "pause") {
          media.paused = true;
        }
        media.fire(event);
      }
      vi.advanceTimersByTime(scrubPauseWindowMs * 2);
      expect(commands, order.join(" then ")).toEqual([]);
      // And it plays on with the group.
      expect(media.paused, order.join(" then ")).toBe(false);
    }
  });

  it("plays nothing while its page is hidden, the film paused there, and follows the group once shown", () => {
    const media = new FakeMedia();
    const page = new FakePage();
    const { connection, commands } = fakeConnection({ ...paused, state: "playing" });
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs, page });
    media.fire("seeked"); // the follower's own seek to the group's position
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(media.paused).toBe(false);
    media.paused = true; // the page's own pause as it hides (pauseWhenHidden)
    page.hide();
    media.fire("pause");
    vi.advanceTimersByTime(3 * checkDriftEveryMs);
    expect(media.paused).toBe(true);
    expect(commands).toEqual([]);
    page.show();
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(media.paused).toBe(false);
  });

  it("takes the pause iOS makes as its screen locks for no pause, whichever it tells first", () => {
    for (const order of [
      ["pause", "hidden"],
      ["hidden", "pause"],
    ] as const) {
      const media = new FakeMedia();
      const page = new FakePage();
      const { connection, commands } = fakeConnection({ ...paused, state: "playing" });
      followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs, page });
      media.fire("seeked"); // the follower's own seek to the group's position
      vi.advanceTimersByTime(scrubPauseWindowMs);
      for (const event of order) {
        if (event === "pause") {
          media.paused = true;
          media.fire("pause");
        } else {
          vi.advanceTimersByTime(browserPauseWindowMs - 100); // the lock tells the page late
          page.hide();
        }
      }
      vi.advanceTimersByTime(browserPauseWindowMs * 2);
      expect(commands, order.join(" then ")).toEqual([]);
    }
  });

  it("sends a pause its person makes on a page that stays shown", () => {
    const media = new FakeMedia();
    const page = new FakePage();
    const { connection, commands } = fakeConnection({ ...paused, state: "playing" });
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs, page });
    media.fire("seeked");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    media.paused = true;
    media.fire("pause");
    vi.advanceTimersByTime(browserPauseWindowMs);
    expect(commands).toEqual([{ command: "pause" }]);
  });

  it("takes the host's player stepping a fraction of a second over a gap for no seek", () => {
    const media = new FakeMedia();
    const { connection, commands } = fakeConnection({ ...paused, state: "playing" });
    followGroup(media, connection, {
      fileId: 5,
      leads: everyRight,
      monotonicMs,
    });
    media.fire("seeked"); // the follower's own seek to the group's position
    media.currentTime = 60.5; // hls.js steps over a gap at the start of what it loaded
    media.seeking = true;
    media.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([]);
  });

  it("takes a seek its player says it made itself, a step over a gap however long, for no seek", () => {
    const media = new FakeMedia();
    const { connection, commands } = fakeConnection({ ...paused, state: "playing" });
    followGroup(media, connection, {
      fileId: 5,
      leads: everyRight,
      monotonicMs,
    });
    media.fire("seeked"); // the follower's own seek to the group's position
    media.currentTime = 66; // hls.js steps over a six-second gap in what it loaded
    media.fire(playerMovedItselfEvent);
    media.seeking = true;
    media.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([]);
  });

  it("sends the host's seek as its person makes it, before the place has loaded, once per drag", () => {
    const media = new FakeMedia();
    const { connection, commands, reports } = fakeConnection({ ...paused, state: "playing" });
    followGroup(media, connection, {
      fileId: 5,
      leads: everyRight,
      monotonicMs,
    });
    media.fire("seeked"); // the follower's own seek to the group's position
    const reportsBefore = reports.length;
    // A drag: a seek at each step, and the last place never loads while the test runs.
    for (const positionS of [300, 900, 1500]) {
      media.currentTime = positionS;
      media.seeking = true;
      media.fire("seeking");
    }
    // Its loading is not told before the seek, or the group would take it for a stall.
    expect(reports).toHaveLength(reportsBefore);
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([{ command: "seek", positionMs: 1_500_000 }]);
    expect(reports.at(-1)).toMatchObject({ positionMs: 1_500_000, isBuffering: true });
    expect(media.currentTime).toBe(1500); // left where its person put it, not pulled back
  });

  it("pauses the host's video while the group waits for a viewer, and sends no command", () => {
    const media = new FakeMedia();
    const { connection, setGroup, commands } = fakeConnection({ ...paused, state: "playing" });
    followGroup(media, connection, {
      fileId: 5,
      leads: everyRight,
      monotonicMs,
    });
    expect(media.paused).toBe(false);
    setGroup({ ...paused, state: "waiting", event: "played", byMemberId: 2 }, 0);
    expect(media.paused).toBe(true);
    media.fire("pause");
    expect(commands).toEqual([]);
  });

  it("takes a pause the browser makes while its person drags the timeline for the seek it is", () => {
    const media = new FakeMedia();
    const { connection, commands } = fakeConnection({ ...paused, state: "playing" });
    followGroup(media, connection, {
      fileId: 5,
      leads: everyRight,
      monotonicMs,
    });
    media.fire("seeked"); // the follower's own seek to the group's position
    media.paused = true;
    media.fire("pause");
    media.currentTime = 5400;
    media.fire("seeking");
    media.fire("seeked");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([{ command: "seek", positionMs: 5_400_000 }]);
  });
});

describe("a leading player held at a sync point", () => {
  it("does not take the hold's own pause for its person's, once the group plays on", () => {
    const media = new FakeMedia();
    const { connection, setGroup, commands } = fakeConnection({ ...paused, state: "playing" });
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs });
    media.paused = false;
    // The host pressed play: the group waits at its sync point, then plays.
    setGroup({ ...paused, state: "waiting" }, 0);
    expect(media.paused).toBe(true);
    setGroup({ ...paused, state: "playing" }, 0);
    media.fire("pause"); // the hold's pause, its event arriving once the group plays on
    vi.advanceTimersByTime(scrubPauseWindowMs * 2);
    expect(commands).toEqual([]);
  });
});

describe("a leading player's own moves", () => {
  it("gives up a mark whose event never came, noting it, so its person's later pause is a command (D268 §4)", () => {
    const media = new FakeMedia();
    const { connection, setGroup, commands, notes } = fakeConnection({
      ...paused,
      state: "playing",
    });
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs });
    media.paused = false;
    // The host pressed play: the group waits, and the hold's pause fires no event.
    setGroup({ ...paused, state: "waiting" }, 0);
    expect(media.paused).toBe(true);
    monotonicNowMs = ownMarkLastsMs + 1;
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(notes).toContainEqual({ kind: "ownMarkExpired", why: "pause" });
    setGroup({ ...paused, state: "playing" }, monotonicNowMs);
    expect(media.paused).toBe(false);
    media.paused = true;
    media.fire("pause"); // its person's pause, a minute later
    vi.advanceTimersByTime(scrubPauseWindowMs * 2);
    expect(commands).toEqual([{ command: "pause" }]);
  });
});

describe("a leading player awaiting the answer to its command", () => {
  it("is not held back to the group as it was by a pong or a roster before the answer", () => {
    const media = new FakeMedia();
    const { connection, setGroup, heardNoAnswer, commands } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs });
    media.fire("seeked"); // the follower's own seek to the group's position
    // The host presses play: its command is sent, and its video plays.
    media.paused = false;
    media.fire("play");
    expect(commands).toEqual([{ command: "play" }]);
    heardNoAnswer(); // a pong arrives before the server's answer
    expect(media.paused).toBe(false);
    setGroup({ ...paused, state: "waiting" }, 0); // the answer: it waits at its sync point
    expect(media.paused).toBe(true);
  });
});

describe("a leading player's command", () => {
  it("is noted with the player's last events, its own marked, so the log says what made it", () => {
    const media = new FakeMedia();
    const { connection, notes } = fakeConnection({ ...paused, state: "playing" });
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs });
    media.paused = false;
    monotonicNowMs = 500;
    media.fire("pause"); // its person's pause
    vi.advanceTimersByTime(scrubPauseWindowMs * 2);
    expect(notes.filter((note) => note.kind === "leaderCommand")).toEqual([
      { kind: "leaderCommand", why: "pause", recentEvents: "pause@500" },
    ]);
  });
});
