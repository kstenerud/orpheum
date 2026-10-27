// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import type { PlaybackDecision } from "./watching.ts";
import { Play, PlayView } from "./play.tsx";
import { BoostSwitch } from "./sound.tsx";
import {
  bodyField,
  choose,
  click,
  mount,
  playbackCauses,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { directDecision, television } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { YourDevices } from "./your-devices.tsx";

/** A decision to play the original, its sound 5.1 where isSurround. */
function direct(isSurround = false): PlaybackDecision {
  const plain = directDecision();
  if (!isSurround) {
    return plain;
  }
  return directDecision({
    tracks: {
      ...plain.tracks,
      sounds: [
        {
          audioTrackIndex: 0,
          isBoosted: false,
          codec: "eac3",
          channelCount: 6,
          kbps: 0,
          isOriginal: true,
        },
      ],
      fileSounds: [{ audioTrackIndex: 0, codec: "eac3", channelCount: 6 }],
    },
  });
}

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The dialogue boost's box, by its label; undefined where it is not offered. */
function boostBox(): HTMLInputElement | null | undefined {
  return [...(mounted?.container.querySelectorAll("label") ?? [])]
    .find((label) => label.textContent === "Dialogue boost")
    ?.querySelector("input");
}

describe("how a device makes sound, asked of no one (D336 §4)", () => {
  it("is not asked before a device's first playback: the film plays at once", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /playback": [200, direct(true)],
        "GET /files/2/waits": [200, {}],
        "GET /files/2/up-next": [200, {}],
        "DELETE /playback/t": [204, null],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<Play fileId={2} />, services));
    expect(mounted.container.querySelector("video")).not.toBeNull();
    expect(mounted.container.textContent).not.toContain("How does this device play sound?");
    expect(mounted.container.textContent).not.toContain("Built-in speakers");
    expect(playbackCauses(heard)).toEqual(["playPage"]);
    expect(heard.some((each) => each.methodAndPath.startsWith("PUT /me/devices"))).toBe(false);
  });

  it("is not chosen for one's devices in Your devices", async () => {
    mounted = await mount(
      withServices(
        <YourDevices
          canManage
          devices={{ state: "loaded", value: [television()] }}
          onChanged={() => undefined}
        />,
        servedBy({}),
      ),
    );
    expect(mounted.container.querySelector("select")).toBeNull();
    expect(mounted.container.textContent).not.toContain("Not asked yet");
  });
});

describe("the dialogue boost over the original (D336 §3)", () => {
  it("is offered where the track has a centre to raise, and turned on asks again from where the video stands", async () => {
    const heard: Heard[] = [];
    const asked: { cause: string; at: unknown }[] = [];
    mounted = await mount(
      withServices(
        <PlayView
          fileId={2}
          decision={direct(true)}
          onDecideAgain={(cause, at) => asked.push({ cause, at })}
        />,
        servedBy(
          {
            "PUT /me/preferences": [204, undefined],
            "GET /files/2/waits": [200, {}],
            "GET /files/2/up-next": [200, {}],
            "DELETE /playback/t": [204, null],
          },
          heard,
        ),
      ),
    );
    const video = mounted.container.querySelector("video");
    if (video !== null) {
      Object.defineProperty(video, "readyState", { value: 4, configurable: true });
      Object.defineProperty(video, "currentTime", { value: 1234.5, configurable: true });
      Object.defineProperty(video, "paused", { value: true, configurable: true });
    }
    // The original's own sound plays, which the device mixes down: the boost is not acting.
    expect(boostBox()?.checked).toBe(false);
    await click(boostBox());
    expect(heard).toContainEqual({
      methodAndPath: "PUT /me/preferences",
      body: { isDialogueBoostOn: true },
    });
    expect(asked).toEqual([
      { cause: "boostChosen", at: { positionMs: 1_234_500, isPaused: true } },
    ]);
  });

  it("asks the server again with the boost's cause once it is kept, and plays what it answers", async () => {
    const heard: Heard[] = [];
    const converted = { codec: "aac", channelCount: 2, kbps: 128, isOriginal: false };
    const plain = directDecision();
    const boosted = directDecision({
      delivery: "hls",
      url: "/api/v1/bytes/media/chunks/u/master.m3u8",
      notDirectReasons: ["dialogue_boost"],
      mediaToken: "u",
      preferences: {
        subtitleOffsetMs: 0,
        isDialogueBoostOn: true,
        isLevellingOn: true,
        isAutoplayOn: true,
      },
      onLadder: {
        url: "/api/v1/bytes/media/chunks/u/master.m3u8",
        startAtBps: 1_896_000,
        qualityKbps: 1200,
        height: 480,
        isBelowTheLowest: false,
        soundCarriedGainDb: 0,
        isReady: false,
        filmPictureKbps: 4000,
        linkSharePercent: 70,
        steadiness: 0,
      },
      tracks: {
        ...plain.tracks,
        pictures: [{ codec: "h264", width: 854, height: 480, kbps: 1200, isOriginal: false }],
        sounds: [
          { audioTrackIndex: 0, isBoosted: false, ...converted },
          { audioTrackIndex: 0, isBoosted: true, ...converted },
        ],
        fileSounds: [{ audioTrackIndex: 0, codec: "eac3", channelCount: 6 }],
      },
    });
    const services = servedBy(
      {
        "POST /playback": [
          [200, direct(true)],
          [200, boosted],
        ],
        "PUT /me/preferences": [204, undefined],
        "GET /files/2/waits": [200, {}],
        "GET /files/2/up-next": [200, {}],
        "GET /files/2/availability": [200, {}],
        "DELETE /playback/t": [204, null],
        "POST /playback/u/report": [200, {}],
        "DELETE /playback/u": [204, null],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<Play fileId={2} />, services));
    await click(boostBox());
    const order = heard
      .map((each) => each.methodAndPath)
      .filter((each) => each === "POST /playback" || each === "PUT /me/preferences");
    expect(order).toEqual(["POST /playback", "PUT /me/preferences", "POST /playback"]);
    expect(playbackCauses(heard)).toEqual(["playPage", "boostChosen"]);
    // The film plays from its ladder with the boosted mix (D344 §1).
    expect(mounted.container.textContent).toContain("transcoded from E-AC-3 5.1, dialogue boost");
  });

  it("is not offered over a stereo original, which has no centre to raise", async () => {
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={direct()} onDecideAgain={() => undefined} />,
        servedBy({
          "GET /files/2/waits": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "DELETE /playback/t": [204, null],
        }),
      ),
    );
    expect(boostBox()).toBeUndefined();
  });
});

describe("the dialogue boost's switch", () => {
  it("starts as the person left it, and switching it changes the mix and is kept for every file", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/preferences": [204, undefined] }, heard);
    const switched: boolean[] = [];
    mounted = await mount(
      withServices(
        <BoostSwitch
          isInitiallyOn
          onSwitched={(isOn) => {
            switched.push(isOn);
          }}
        />,
        services,
      ),
    );
    const box = mounted.container.querySelector("input");
    expect(box?.checked).toBe(true);
    await click(box);
    expect(box?.checked).toBe(false);
    expect(switched).toEqual([false]);
    expect(heard).toEqual([
      { methodAndPath: "PUT /me/preferences", body: { isDialogueBoostOn: false } },
    ]);
  });
});

describe("the dialogue boost on a film's ladder", () => {
  /** A film on its ladder: its surround track's standard mix and boosted one. */
  function onLadder(isDialogueBoostOn: boolean): PlaybackDecision {
    const converted = { codec: "aac", channelCount: 2, kbps: 128, isOriginal: false };
    const plain = directDecision();
    return directDecision({
      onLadder: {
        url: "/api/v1/bytes/media/chunks/t/master.m3u8",
        startAtBps: 1_896_000,
        qualityKbps: 1200,
        height: 480,
        isBelowTheLowest: false,
        soundCarriedGainDb: 0,
        isReady: false,
        filmPictureKbps: 4000,
        linkSharePercent: 70,
        steadiness: 0,
      },
      preferences: { ...plain.preferences, isDialogueBoostOn },
      tracks: {
        ...plain.tracks,
        pictures: [{ codec: "h264", width: 854, height: 480, kbps: 1200, isOriginal: false }],
        sounds: [
          { audioTrackIndex: 0, isBoosted: false, ...converted },
          { audioTrackIndex: 0, isBoosted: true, ...converted },
        ],
        fileSounds: [{ audioTrackIndex: 0, codec: "eac3", channelCount: 6 }],
      },
    });
  }

  it("is switched under the film on its ladder, playing the other mix and kept for every file (the owner, 2026-10-24)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "PUT /me/preferences": [204, undefined],
        "GET /files/2/waits": [200, {}],
        "GET /files/2/availability": [200, {}],
        "GET /files/2/up-next": [200, {}],
        "POST /playback/t/report": [200, {}],
        "DELETE /playback/t": [204, null],
      },
      heard,
    );
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={onLadder(false)} onDecideAgain={() => undefined} />,
        services,
      ),
    );
    const box = [...mounted.container.querySelectorAll("label")]
      .find((label) => label.textContent === "Dialogue boost")
      ?.querySelector("input");
    expect(box?.checked).toBe(false);
    expect(mounted.container.textContent).toContain(
      "Audio: AAC stereo, 128 kb/s (transcoded from E-AC-3 5.1)",
    );
    await click(box);
    expect(heard).toContainEqual({
      methodAndPath: "PUT /me/preferences",
      body: { isDialogueBoostOn: true },
    });
    expect(mounted.container.textContent).toContain(
      "Audio: AAC stereo, 128 kb/s (transcoded from E-AC-3 5.1, dialogue boost)",
    );
  });

  it("starts as the person left it, and is not offered for a track with no centre to raise", async () => {
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={onLadder(true)} onDecideAgain={() => undefined} />,
        servedBy({
          "GET /files/2/waits": [200, {}],
          "GET /files/2/availability": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "POST /playback/t/report": [200, {}],
          "DELETE /playback/t": [204, null],
        }),
      ),
    );
    expect(mounted.container.textContent).toContain("dialogue boost)");
    const stereo = onLadder(false);
    mounted.unmount();
    mounted = await mount(
      withServices(
        <PlayView
          fileId={2}
          decision={{
            ...stereo,
            tracks: { ...stereo.tracks, sounds: stereo.tracks.sounds?.slice(0, 1) ?? [] },
          }}
          onDecideAgain={() => undefined}
        />,
        servedBy({
          "GET /files/2/waits": [200, {}],
          "GET /files/2/availability": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "POST /playback/t/report": [200, {}],
          "DELETE /playback/t": [204, null],
        }),
      ),
    );
    expect(mounted.container.textContent).not.toContain("Dialogue boost");
  });
});

/** A decision to play the original, with two audio tracks to choose between, the first chosen. */
function withTwoTracks(): PlaybackDecision {
  return directDecision({
    audioTracks: [
      { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
      { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "plain" },
    ],
    startingAudioStreamId: 80,
  });
}

/** Stands the page's video at positionS, loaded there and playing, as one playing for a while does. */
function standVideoAt(positionS: number): void {
  const video = mounted?.container.querySelector("video");
  if (video !== null && video !== undefined) {
    Object.defineProperty(video, "readyState", { value: 4, configurable: true });
    Object.defineProperty(video, "currentTime", { value: positionS, configurable: true });
    Object.defineProperty(video, "paused", { value: false, configurable: true });
  }
}

describe("another audio track, chosen mid-film (D230 §5)", () => {
  it("asks how the film plays again from where the video stands, not from its beginning", async () => {
    const asked: { cause: string; at: unknown }[] = [];
    mounted = await mount(
      withServices(
        <PlayView
          fileId={2}
          decision={withTwoTracks()}
          onDecideAgain={(cause, at) => asked.push({ cause, at })}
        />,
        servedBy({
          "PUT /files/2/preferences": [204, undefined],
          "GET /files/2/waits": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "DELETE /playback/t": [204, null],
        }),
      ),
    );
    // The Unknown, its track changed 2:46 in, started again at 0:00 (staging, 2026-10-26).
    standVideoAt(166.502);
    await choose(mounted.container.querySelector("select"), "81");
    expect(asked).toEqual([{ cause: "audioChosen", at: { positionMs: 166_502, isPaused: false } }]);
  });

  it("tells the server where the page goes on from, which its log says beside the cause", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "POST /playback": [
          [200, withTwoTracks()],
          [200, withTwoTracks()],
        ],
        "PUT /files/2/preferences": [204, undefined],
        "GET /files/2/waits": [200, {}],
        "GET /files/2/up-next": [200, {}],
        "DELETE /playback/t": [204, null],
      },
      heard,
    );
    services.session.signIn("token", { deviceId: 5 });
    mounted = await mount(withServices(<Play fileId={2} />, services));
    standVideoAt(166.502);
    await choose(mounted.container.querySelector("select"), "81");
    const asks = heard.filter((each) => each.methodAndPath === "POST /playback");
    expect(playbackCauses(heard)).toEqual(["playPage", "audioChosen"]);
    expect(asks.map((each) => bodyField(each, "startsAtMs"))).toEqual([undefined, 166_502]);
  });
});
