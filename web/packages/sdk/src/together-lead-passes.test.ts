// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { everyRight, noRight, type RosterDevice, type RosterMember } from "./together.ts";
import type { TogetherConnection } from "./together-connection.ts";
import { isHostAwaitingLead } from "./together-roster.ts";
import { checkDriftEveryMs } from "./together-follow.ts";
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

/** This device, 8, in the roster. */
const thisDevice = 8;

/** A player of the roster, leading its session or not. */
function player(id: number, isLeader = false): RosterDevice {
  return {
    id,
    role: "video",
    isBuffering: false,
    cannotKeepUp: undefined,
    status: "watching",
    isLeader,
  };
}

/**
 * connection, its snapshot's roster the host's devices and a guest's, this device the host's when
 * isHost, else the guest's.
 */
function withMembers(
  connection: TogetherConnection,
  members: { readonly host: readonly RosterDevice[]; readonly guest: readonly RosterDevice[] },
  isHost: boolean,
): TogetherConnection {
  const roster: RosterMember[] = [
    { id: 1, name: "owner", isHost: true, devices: members.host },
    { id: 2, name: "partner", isHost: false, devices: members.guest },
  ];
  return {
    ...connection,
    snapshot: () => ({ ...connection.snapshot(), deviceId: thisDevice, isHost, roster }),
  };
}

describe("the lead passing down the line (D372)", () => {
  it("tells its session as its page is hidden, where hiding pauses its film, and as it is shown again (§2, §5)", () => {
    const media = new FakeMedia();
    media.currentTime = 60;
    const page = new FakePage();
    const { connection, reports } = fakeConnection({ ...paused, state: "playing" });
    const roster = { host: [], guest: [player(thisDevice, true)] };
    followGroup(media, withMembers(connection, roster, false), {
      fileId: 5,
      leads: noRight,
      monotonicMs,
      page,
    });
    page.hide();
    expect(reports.at(-1)?.isPageHidden).toBe(true);
    page.show();
    expect(reports.at(-1)?.isPageHidden).toBeUndefined();
  });

  it("says nothing of a hidden page where hiding pauses nothing: a computer's plays on and may lead", () => {
    const media = new FakeMedia();
    const { connection, reports } = fakeConnection({ ...paused, state: "playing" });
    followGroup(media, connection, { fileId: 5, leads: everyRight, monotonicMs });
    connection.reportState();
    expect(reports.at(-1)?.isPageHidden).toBeUndefined();
  });

  it("follows as the host's player come back while a guest's leads, telling its state each second until it leads (§3)", () => {
    const media = new FakeMedia();
    media.currentTime = 60;
    const { connection, setGroup, reports, commands } = fakeConnection(paused);
    let isLeading = false;
    const session = withMembers(connection, { host: [], guest: [] }, true);
    const host: TogetherConnection = {
      ...session,
      snapshot: () => ({
        ...session.snapshot(),
        roster: [
          { id: 1, name: "owner", isHost: true, devices: [player(thisDevice, isLeading)] },
          { id: 2, name: "partner", isHost: false, devices: [player(40, !isLeading)] },
        ],
      }),
    };
    expect(isHostAwaitingLead(host.snapshot())).toBe(true);
    followGroup(media, host, { fileId: 5, leads: everyRight, monotonicMs });
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "playing", positionMs: 70_000 }, 1000);
    expect(media.paused).toBe(false); // it plays along, caught up to the guest's lead
    const before = reports.length;
    monotonicNowMs = 4000;
    vi.advanceTimersByTime(3 * checkDriftEveryMs);
    expect(reports.length - before).toBeGreaterThanOrEqual(3);
    expect(commands).toEqual([]);
    isLeading = true;
    const leading = reports.length;
    monotonicNowMs = 7000;
    vi.advanceTimersByTime(3 * checkDriftEveryMs);
    expect(reports.length).toBe(leading); // leading, it tells its state as its player changes
  });

  it("puts back where the group is a guest's leading player its person paused without the right (§4)", () => {
    const media = new FakeMedia();
    media.currentTime = 60;
    const { connection, commands, notes } = fakeConnection({ ...paused, state: "playing" });
    const roster = { host: [], guest: [player(thisDevice, true)] };
    followGroup(media, withMembers(connection, roster, false), {
      fileId: 5,
      leads: noRight,
      monotonicMs,
    });
    media.paused = true;
    media.fire("pause");
    vi.advanceTimersByTime(1000);
    expect(media.paused).toBe(false);
    media.currentTime = 600;
    media.fire("seeking");
    expect(media.currentTime).toBe(60);
    expect(notes.filter((note) => note.kind === "sought")).toEqual([
      { kind: "sought", positionMs: 600_000, toMs: 60_000, why: "undone" },
    ]);
    expect(commands).toEqual([]);
  });
});
