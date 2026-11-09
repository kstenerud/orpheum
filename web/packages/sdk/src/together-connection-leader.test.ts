// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { PictureChoice, PlaybackPicture } from "./playback-picture.ts";
import type { PlaybackSound, SoundChoice } from "./playback-sound.ts";
import { createTogetherConnection } from "./together-connection.ts";
import { PlaybackTold } from "./together-leader.ts";
import { isLeaderOf, leaderOf } from "./together-roster.ts";
import type { TogetherSocketListeners } from "./together.ts";

/** A stand-in for the server's end of one socket: what it heard, and its listeners. */
function oneSocket(): {
  readonly sent: Record<string, unknown>[];
  readonly listeners: () => TogetherSocketListeners;
  readonly open: (listeners: TogetherSocketListeners) => {
    send: (text: string) => void;
    close: () => void;
  };
} {
  const sent: Record<string, unknown>[] = [];
  let opened: TogetherSocketListeners | undefined;
  return {
    sent,
    listeners: () => {
      if (opened === undefined) {
        throw new Error("no socket opened");
      }
      return opened;
    },
    open: (listeners) => {
      opened = listeners;
      return {
        send: (text) => {
          sent.push(JSON.parse(text) as Record<string, unknown>);
        },
        close: () => undefined,
      };
    },
  };
}

/** A connection, welcomed as device 9 with the roster given, and its socket. */
function welcomed(roster: readonly unknown[]): {
  readonly connection: ReturnType<typeof createTogetherConnection>;
  readonly socket: ReturnType<typeof oneSocket>;
} {
  const socket = oneSocket();
  const connection = createTogetherConnection({
    credential: "c",
    openSocket: socket.open,
    monotonicMs: () => 1000,
  });
  connection.open();
  socket.listeners().onOpen();
  socket.listeners().onMessage(
    JSON.stringify({
      v: 3,
      type: "welcome",
      seq: 4,
      memberId: 7,
      deviceId: 9,
      group: { state: "playing", positionMs: 10_000, rate: 1, fileId: 3 },
      roster,
    }),
  );
  return { connection, socket };
}

describe("a session's leader, as its roster says (D371 §1)", () => {
  it("reads which device leads and how each says it plays the film, and whether this one leads", () => {
    const { connection } = welcomed([
      {
        id: 7,
        name: "Karl",
        devices: [
          { id: 9, role: "video" },
          {
            id: 12,
            role: "video",
            isLeader: true,
            playback: { delivery: "converted", qualityKbps: 4200, linkKbps: 18_000 },
          },
          { id: 13, role: "video", playback: { delivery: "teleported" } },
        ],
      },
    ]);
    const snapshot = connection.snapshot();
    expect(leaderOf(snapshot)).toMatchObject({
      id: 12,
      playback: { delivery: "converted", qualityKbps: 4200, linkKbps: 18_000 },
    });
    expect(isLeaderOf(snapshot)).toBe(false);
    expect(snapshot.roster[0]?.devices[2]?.playback).toBeUndefined();
    expect(
      isLeaderOf(
        welcomed([
          { id: 7, name: "Karl", devices: [{ id: 9, isLeader: true }] },
        ]).connection.snapshot(),
      ),
    ).toBe(true);
    expect(leaderOf(welcomed([]).connection.snapshot())).toBeUndefined();
  });

  it("reads why the leader leads, as the server says it, of the leader alone (a debug view's)", () => {
    const snapshot = welcomed([
      {
        id: 7,
        name: "Karl",
        devices: [
          { id: 9, role: "video", leadsBecause: "" },
          { id: 12, role: "video", isLeader: true, leadsBecause: "leaderGone:hidden" },
        ],
      },
    ]).connection.snapshot();
    expect(leaderOf(snapshot)?.leadsBecause).toBe("leaderGone:hidden");
    expect(snapshot.roster[0]?.devices[0]).not.toHaveProperty("leadsBecause");
  });

  it("tells the session in its state how its player plays the film, in whole numbers", () => {
    const { connection, socket } = welcomed([]);
    connection.setPlayer(() => ({
      fileId: 7,
      positionMs: 10_000,
      isPlaying: true,
      isBuffering: false,
      playback: { delivery: "converted", qualityKbps: 4200.4, linkKbps: 17_999.6 },
    }));
    connection.reportState();
    expect(socket.sent.at(-1)).toMatchObject({
      type: "state",
      playback: { delivery: "converted", qualityKbps: 4200, linkKbps: 18_000 },
    });
  });

  it("takes a seek of a group that plays on, with no leader to stand for, for a new timeline (D371 §2)", () => {
    const { connection, socket } = welcomed([]);
    socket.listeners().onMessage(
      JSON.stringify({
        v: 3,
        type: "group",
        seq: 5,
        group: {
          state: "playing",
          positionMs: 600_000,
          rate: 1,
          fileId: 3,
          event: "seeked",
          seekedFromMs: 10_000,
        },
      }),
    );
    expect(connection.snapshot().group?.positionMs).toBe(600_000);
  });
});

// A Fire TV's own controls are hidden while it is controlled, so its sound could only be changed
// after taking control back (the owner, VQ116's run): its remote shows and changes it.
describe("a film's sound between a player and its remote", () => {
  const sound: PlaybackSound = {
    isBoostOn: false,
    isLevellingOn: true,
    audioStreamId: 2,
    audioTracks: [
      { streamId: 1, language: "en", kind: "plain" },
      { streamId: 2, language: "de", containerTitle: "Director", kind: "commentary" },
    ],
    channels: [{ channel: "film", level: 0.4, isMuted: true }], // D440 §2
  };

  it("reads how each player says its film sounds, and tells the session its own", () => {
    const { connection, socket } = welcomed([
      {
        id: 7,
        name: "Karl",
        devices: [
          { id: 12, role: "video", isLeader: true, playback: { delivery: "converted", sound } },
          {
            id: 13,
            role: "video",
            playback: {
              delivery: "direct",
              sound: {
                audioTracks: [{ streamId: 4, kind: "x" }],
                // The server omits a zero level; a channel no player mixes is none.
                channels: [{ channel: "film" }, { channel: "voices", level: 0.5 }],
              },
            },
          },
        ],
      },
    ]);
    expect(leaderOf(connection.snapshot())?.playback?.sound).toEqual(sound);
    expect(connection.snapshot().roster[0]?.devices[1]?.playback?.sound).toEqual({
      isBoostOn: undefined,
      isLevellingOn: false,
      audioStreamId: undefined,
      audioTracks: [{ streamId: 4, language: "", kind: "plain" }],
      channels: [{ channel: "film", level: 0, isMuted: false }],
    });
    connection.setPlayer(() => ({
      fileId: 7,
      positionMs: 10_000,
      isPlaying: true,
      isBuffering: false,
      playback: { delivery: "converted", sound: { ...sound, isBoostOn: undefined } },
    }));
    connection.reportState();
    const told = socket.sent.at(-1)?.["playback"] as { sound?: Record<string, unknown> };
    expect(told.sound).toEqual({
      isLevellingOn: true,
      audioStreamId: 2,
      audioTracks: sound.audioTracks,
      channels: sound.channels,
    });
  });

  it("sends a remote's choice, and hands a choice made on another device to its listeners", () => {
    const { connection, socket } = welcomed([]);
    const heard: SoundChoice[] = [];
    const unsubscribe = connection.subscribeToSoundChoices((choice) => {
      heard.push(choice);
    });
    connection.chooseSound({ kind: "boost", isOn: true });
    connection.chooseSound({ kind: "audioTrack", streamId: 2 });
    connection.chooseSound({ kind: "channel", channel: "film", level: 0.3, isMuted: false });
    expect(socket.sent.filter((each) => each["type"] === "sound")).toEqual([
      { v: 3, type: "sound", sound: { kind: "boost", isOn: true } },
      { v: 3, type: "sound", sound: { kind: "audioTrack", streamId: 2 } },
      {
        v: 3,
        type: "sound",
        sound: { kind: "channel", channel: "film", level: 0.3, isMuted: false },
      },
    ]);
    const before = connection.snapshot();
    const hear = (sent: unknown): void => {
      socket
        .listeners()
        .onMessage(JSON.stringify({ v: 3, type: "sound", fromDeviceId: 3, sound: sent }));
    };
    hear({ kind: "levelling" });
    hear({ kind: "audioTrack", streamId: 5 });
    hear({ kind: "louder" });
    hear({ kind: "audioTrack" });
    hear({ kind: "soundOffset", ms: -250 });
    hear({ kind: "soundOffset" }); // the server omits a zero
    hear({ kind: "channel", channel: "film", level: 0.6 });
    hear({ kind: "channel", channel: "film", isMuted: true }); // the server omits a zero level
    hear({ kind: "channel", channel: "voices", level: 0.5 });
    hear({ kind: "channel", channel: "film", level: 2 });
    expect(heard).toEqual([
      { kind: "levelling", isOn: false },
      { kind: "audioTrack", streamId: 5 },
      { kind: "soundOffset", ms: -250 },
      { kind: "soundOffset", ms: 0 },
      { kind: "channel", channel: "film", level: 0.6, isMuted: false },
      { kind: "channel", channel: "film", level: 0, isMuted: true },
    ]);
    expect(connection.snapshot()).toBe(before);
    unsubscribe();
    hear({ kind: "boost", isOn: false });
    expect(heard).toHaveLength(6);
  });

  it("says its playback alone, with no player, as asked (the owner, 2026-11-06)", () => {
    // A cast's receiver says it in the cast's session while it leads Watch Together: "It shouldn't
    // be watch together's business at all which TV has what offset or playback characteristics".
    const { connection, socket } = welcomed([]);
    let offsetMs = -400;
    connection.setPlayback(() => ({
      delivery: "converted",
      qualityKbps: 4000.4,
      sound: { isLevellingOn: true, audioTracks: [], soundOffsetMs: offsetMs },
    }));
    offsetMs = -450;
    connection.reportPlayback();
    const said = (): unknown[] => socket.sent.filter((each) => each["type"] === "playback");
    expect(said()).toEqual([
      {
        v: 3,
        type: "playback",
        playback: {
          delivery: "converted",
          qualityKbps: 4000,
          sound: { isLevellingOn: true, audioTracks: [], soundOffsetMs: -400 },
        },
      },
      {
        v: 3,
        type: "playback",
        playback: {
          delivery: "converted",
          qualityKbps: 4000,
          sound: { isLevellingOn: true, audioTracks: [], soundOffsetMs: -450 },
        },
      },
    ]);
    expect(socket.sent.some((each) => each["type"] === "state")).toBe(false);
    connection.setPlayback(undefined);
    connection.reportPlayback();
    expect(said()).toHaveLength(2);
  });

  it("tells its film's sound with how it plays it, as its page reads it", () => {
    let now: PlaybackSound | undefined = sound;
    const told = new PlaybackTold("converted", () => now);
    expect(told.report()).toEqual({ delivery: "converted", sound });
    now = undefined;
    expect(told.report()).toEqual({ delivery: "converted" });
  });
});

// A cast's remote shows the receiver's subtitles and its offer of the original, and chooses them
// (D440 §5), as it does its sound.
describe("what a film shows, between a player and its remote", () => {
  const picture: PlaybackPicture = {
    subtitleKey: "f7",
    subtitles: [
      { key: "s3", label: "English" },
      { key: "f7", label: "Deutsch" },
    ],
    canPlayOriginal: true,
  };

  it("reads what each player says it shows, and tells the session its own", () => {
    const { connection, socket } = welcomed([
      {
        id: 7,
        name: "Karl",
        devices: [
          { id: 12, role: "video", isLeader: true, playback: { delivery: "converted", picture } },
          {
            id: 13,
            role: "video",
            playback: { delivery: "direct", picture: { subtitles: [{ key: "", label: "x" }] } },
          },
          { id: 14, role: "video", playback: { delivery: "direct" } },
        ],
      },
    ]);
    const pictures = (connection.snapshot().roster[0]?.devices ?? []).map(
      (device) => device.playback?.picture,
    );
    expect(pictures).toEqual([
      picture,
      { subtitleKey: undefined, subtitles: [], canPlayOriginal: false },
      undefined,
    ]);
    connection.setPlayback(() => ({
      delivery: "converted",
      picture: { subtitles: picture.subtitles, canPlayOriginal: false },
    }));
    connection.reportPlayback();
    const told = socket.sent.at(-1)?.["playback"] as { picture?: Record<string, unknown> };
    expect(told.picture).toEqual({ subtitles: picture.subtitles, canPlayOriginal: false });
  });

  it("sends a remote's choice, and hands a choice made on another device to its listeners", () => {
    const { connection, socket } = welcomed([]);
    const heard: PictureChoice[] = [];
    const unsubscribe = connection.subscribeToPictureChoices((choice) => {
      heard.push(choice);
    });
    connection.choosePicture({ kind: "subtitles", key: "s3" });
    connection.choosePicture({ kind: "subtitles", key: undefined });
    connection.choosePicture({ kind: "original" });
    expect(socket.sent.filter((each) => each["type"] === "picture")).toEqual([
      { v: 3, type: "picture", picture: { kind: "subtitles", key: "s3" } },
      { v: 3, type: "picture", picture: { kind: "subtitles" } },
      { v: 3, type: "picture", picture: { kind: "original" } },
    ]);
    const before = connection.snapshot();
    const hear = (sent: unknown): void => {
      socket
        .listeners()
        .onMessage(JSON.stringify({ v: 3, type: "picture", fromDeviceId: 3, picture: sent }));
    };
    hear({ kind: "subtitles", key: "f7" });
    hear({ kind: "subtitles" }); // the server omits the key of subtitles off
    hear({ kind: "zoom" });
    hear({ kind: "original" });
    expect(heard).toEqual([
      { kind: "subtitles", key: "f7" },
      { kind: "subtitles", key: undefined },
      { kind: "original" },
    ]);
    expect(connection.snapshot()).toBe(before);
    unsubscribe();
    hear({ kind: "original" });
    expect(heard).toHaveLength(3);
  });

  it("tells what its film shows with how it plays it, as its page reads it", () => {
    let now: PlaybackPicture | undefined = picture;
    const told = new PlaybackTold("converted", undefined, () => now);
    expect(told.report()).toEqual({ delivery: "converted", picture });
    now = undefined;
    expect(told.report()).toEqual({ delivery: "converted" });
  });
});
