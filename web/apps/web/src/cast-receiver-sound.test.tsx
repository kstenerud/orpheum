// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AudioMenu } from "./audio-tracks.tsx";
import { CastReceiver } from "./cast-receiver.tsx";
import { newSoundPanel, SoundPanelContext } from "./sound-panel.ts";
import { BoostSwitch } from "./sound.tsx";
import { mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";

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

describe("the receiver's sound, worked by its remote (sound-panel.ts)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    document.body.replaceChildren();
  });

  // A Fire TV's own controls are hidden while it is controlled, so its sound could only be changed
  // after taking control back (the owner, VQ116's run).
  it("carries out its remote's sound choices as its own presses, and tells its remote how it sounds", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      { "PUT /me/preferences": [204, undefined], "PUT /files/4/preferences": [204, undefined] },
      heard,
      "token",
    );
    const server = fakeTogetherServer();
    const video = document.createElement("video");
    document.body.append(video);
    const boosted = vi.fn();
    const chosenTrack = vi.fn();
    const tracks = [
      { streamId: 1, audioTrackIndex: 0, language: "en", kind: "plain" as const },
      { streamId: 2, audioTrackIndex: 1, language: "fr", kind: "plain" as const },
    ];
    mounted = await mount(
      withServices(
        <SoundPanelContext value={newSoundPanel()}>
          <CastReceiver
            receiving={{ cast, onLeave: vi.fn() }}
            fileId={4}
            video={video}
            startPositionMs={0}
            delivery="converted"
          />
          <BoostSwitch isInitiallyOn={false} onSwitched={boosted} />
          <AudioMenu
            fileId={4}
            decision={directDecision({ audioTracks: tracks, startingAudioStreamId: 1 })}
            onChosen={chosenTrack}
          />
        </SoundPanelContext>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say(welcome);
    const lastSound = (): unknown =>
      (
        server.heard.filter((each) => each["type"] === "state").at(-1)?.["playback"] as
          { sound?: unknown } | undefined
      )?.sound;
    expect(lastSound()).toEqual({
      isBoostOn: false,
      isLevellingOn: false,
      audioStreamId: 1,
      audioTracks: [
        { streamId: 1, language: "en", kind: "plain" },
        { streamId: 2, language: "fr", kind: "plain" },
      ],
      channels: [{ channel: "film", level: 1, isMuted: false }],
    });

    await server.say({ type: "sound", fromDeviceId: 5, sound: { kind: "boost", isOn: true } });
    await server.say({
      type: "sound",
      fromDeviceId: 5,
      sound: { kind: "audioTrack", streamId: 2 },
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(boosted).toHaveBeenCalledWith(true);
    expect(heard).toContainEqual({
      methodAndPath: "PUT /me/preferences",
      body: { isDialogueBoostOn: true },
    });
    expect(heard).toContainEqual({
      methodAndPath: "PUT /files/4/preferences",
      body: { audioChoice: { streamId: 2 } },
    });
    expect(chosenTrack).toHaveBeenCalledWith(tracks[1]);
    expect(lastSound()).toMatchObject({ isBoostOn: true, audioStreamId: 2 });
  });

  // D440 §2, §5: the film's level and mute on the TV, set from the phone, apart from the TV's own
  // volume, for the session alone.
  it("sets the film's level and mute as its remote chooses, and tells its remote where they stand", async () => {
    const services = servedBy({}, [], "token");
    const server = fakeTogetherServer();
    const video = document.createElement("video");
    document.body.append(video);
    mounted = await mount(
      withServices(
        <SoundPanelContext value={newSoundPanel()}>
          <CastReceiver
            receiving={{ cast, onLeave: vi.fn() }}
            fileId={4}
            video={video}
            startPositionMs={0}
            delivery="converted"
          />
        </SoundPanelContext>,
        { ...services, openTogetherSocket: server.openSocket },
      ),
    );
    await server.accept();
    await server.say(welcome);
    const lastChannels = (): unknown =>
      (
        server.heard.filter((each) => each["type"] === "state").at(-1)?.["playback"] as
          { sound?: { channels?: unknown } } | undefined
      )?.sound?.channels;
    expect(lastChannels()).toEqual([{ channel: "film", level: 1, isMuted: false }]);

    await server.say({
      type: "sound",
      fromDeviceId: 5,
      sound: { kind: "channel", channel: "film", level: 0.35, isMuted: true },
    });
    await act(async () => {
      video.dispatchEvent(new Event("volumechange")); // as a browser does; happy-dom does not
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(video.volume).toBeCloseTo(0.35);
    expect(video.muted).toBe(true);
    expect(lastChannels()).toEqual([{ channel: "film", level: 0.35, isMuted: true }]);

    // A change made on the TV itself is told too.
    await act(async () => {
      video.muted = false;
      video.dispatchEvent(new Event("volumechange"));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(lastChannels()).toEqual([{ channel: "film", level: 0.35, isMuted: false }]);
  });

  // The owner, 2026-11-06: "It shouldn't be watch together's business at all which TV has what offset
  // or playback characteristics": while its person hosts Watch Together it leads that session (D364
  // §11), and still says how it sounds in the cast's, and is worked there.
  it("tells the cast's session how it sounds, and hears its remote there, while it leads Watch Together", async () => {
    const services = servedBy({ "PUT /me/preferences": [204, undefined] }, [], "token");
    const sockets = { cast: fakeTogetherServer(), hosted: fakeTogetherServer() };
    let opened = 0;
    const video = document.createElement("video");
    document.body.append(video);
    const boosted = vi.fn();
    mounted = await mount(
      withServices(
        <SoundPanelContext value={newSoundPanel()}>
          <CastReceiver
            receiving={{ cast: { ...cast, togetherCredential: "hosted" }, onLeave: vi.fn() }}
            fileId={4}
            video={video}
            startPositionMs={0}
            delivery="converted"
          />
          <BoostSwitch isInitiallyOn={false} onSwitched={boosted} />
        </SoundPanelContext>,
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
    await sockets.cast.say(welcome);
    await sockets.hosted.say({ ...welcome, sessionId: 3 });
    const toldTheCast = (): unknown =>
      sockets.cast.heard.filter((each) => each["type"] === "playback").at(-1)?.["playback"];
    expect(toldTheCast()).toMatchObject({ delivery: "converted", sound: { isBoostOn: false } });
    await sockets.cast.say({
      type: "sound",
      fromDeviceId: 5,
      sound: { kind: "boost", isOn: true },
    });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(boosted).toHaveBeenCalledWith(true);
    expect(toldTheCast()).toMatchObject({ sound: { isBoostOn: true } });
    expect(sockets.cast.heard.some((each) => each["type"] === "state")).toBe(false);
  });
});
