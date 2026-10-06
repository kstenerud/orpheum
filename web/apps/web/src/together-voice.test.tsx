// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { BrowserVoiceDevices, VoiceCall, VoiceCallSetup } from "@orpheum/sdk";
import { act } from "react";
import { beforeEach, describe, expect, it } from "vitest";

import { buttonWithText, click, enter, mount, servedBy, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";
import { HostPlaysTogether } from "./together-host.tsx";
import { TogetherViewer } from "./together-viewer.tsx";
import { VoiceControls } from "./together-voice.tsx";

/** A call as a test sees it. */
interface FakeCall extends VoiceCall {
  readonly setup: VoiceCallSetup<MediaStream>;
  gain: number | undefined;
}

/** Stand-in devices for voice: a microphone of one track, and the calls made. */
interface FakeDevices {
  readonly devices: BrowserVoiceDevices;
  readonly track: MediaStreamTrack;
  readonly calls: FakeCall[];
}

function fakeDevices(openMicrophone?: () => Promise<MediaStream>): FakeDevices {
  const track = { enabled: true, stop: () => undefined } as unknown as MediaStreamTrack;
  const microphone = { getAudioTracks: () => [track] } as unknown as MediaStream;
  const calls: FakeCall[] = [];
  return {
    track,
    calls,
    devices: {
      openMicrophone: openMicrophone ?? (() => Promise.resolve(microphone)),
      startCall: (setup) => {
        const call: FakeCall = {
          setup,
          gain: undefined,
          offer: () => Promise.resolve("offer"),
          answer: () => Promise.resolve("answer"),
          accept: () => Promise.resolve(),
          addCandidate: () => Promise.resolve(),
          setGain: (gain) => {
            call.gain = gain;
          },
          close: () => undefined,
        };
        calls.push(call);
        return call;
      },
      close: () => undefined,
    },
  };
}

const membership = { sessionId: 1, credential: "viewer-credential" };

const relay = { urls: ["turn:orpheum.test:443"], username: "u", credential: "p", expiresAt: 1 };

/** The welcome of this person's voice device 8; Karl's voice device 4 is in the session too. */
const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    {
      id: 1,
      name: "Karl",
      isHost: true,
      devices: [
        { id: 3, role: "playback" },
        { id: 4, role: "voice" },
      ],
    },
    {
      id: 2,
      name: "Gran",
      devices: [
        { id: 7, role: "playback" },
        { id: 8, role: "voice" },
      ],
    },
  ],
};

function voiceServices(
  devices: FakeDevices,
  heard: Heard[] = [],
): { services: ReturnType<typeof servedBy>; server: FakeTogetherServer } {
  const server = fakeTogetherServer();
  return {
    server,
    services: {
      ...servedBy({ "GET /together/1/ice": [200, { iceServers: [relay] }] }, heard),
      openTogetherSocket: server.openSocket,
      voiceDevices: () => devices.devices,
    },
  };
}

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("voice controls in a Watch Together session (watch-together.md, Voice)", () => {
  it("talks: attaches for voice, asks for the relay, and says who it is connected to", async () => {
    const devices = fakeDevices();
    const heard: Heard[] = [];
    const { services, server } = voiceServices(devices, heard);
    const mounted = await mount(withServices(<VoiceControls membership={membership} />, services));
    expect(mounted.container.textContent).toContain("Use headphones");
    expect(server.openedCount()).toBe(0);
    await click(buttonWithText(mounted.container, "Talk"));
    await server.accept();
    expect(server.heard[0]).toEqual({
      v: 2,
      type: "hello",
      credential: "viewer-credential",
      role: "voice",
    });
    expect(mounted.container.textContent).toContain("Voice: connecting…");
    await server.say(welcome);
    expect(heard.map((each) => each.methodAndPath)).toEqual(["GET /together/1/ice"]);
    expect(devices.calls).toHaveLength(1);
    expect(devices.calls[0]!.setup.iceServers).toEqual([
      { urls: relay.urls, username: "u", credential: "p" },
    ]);
    expect(mounted.container.textContent).toContain("Voice: connecting to Karl…");
    act(() => {
      devices.calls[0]!.setup.events.onState("connected");
    });
    expect(mounted.container.textContent).toContain("Voice: connected to Karl");
    mounted.unmount();
  });

  it("mutes the microphone, sets the partner's volume, and leaves voice", async () => {
    const devices = fakeDevices();
    const { services, server } = voiceServices(devices);
    const mounted = await mount(withServices(<VoiceControls membership={membership} />, services));
    await click(buttonWithText(mounted.container, "Talk"));
    await server.accept();
    await server.say(welcome);
    await click(buttonWithText(mounted.container, "Mute"));
    expect(devices.track.enabled).toBe(false);
    expect(buttonWithText(mounted.container, "Unmute")?.getAttribute("aria-pressed")).toBe("true");
    await click(buttonWithText(mounted.container, "Unmute"));
    expect(devices.track.enabled).toBe(true);
    const partner = [...mounted.container.querySelectorAll("label")].find((label) =>
      label.textContent.startsWith("Partner"),
    );
    await enter(partner?.querySelector("input"), "40");
    expect(devices.calls[0]!.gain).toBe(0.4);
    await click(buttonWithText(mounted.container, "Leave voice"));
    expect(buttonWithText(mounted.container, "Talk")).toBeDefined();
    expect(mounted.container.textContent).not.toContain("Voice: connect");
    mounted.unmount();
  });

  it("starts off, and turns off saying so when its call fails, Talk trying again", async () => {
    const devices = fakeDevices();
    const { services, server } = voiceServices(devices);
    const mounted = await mount(withServices(<VoiceControls membership={membership} />, services));
    expect(buttonWithText(mounted.container, "Talk")).toBeDefined();
    expect(server.openedCount()).toBe(0);
    await click(buttonWithText(mounted.container, "Talk"));
    await server.accept();
    await server.say(welcome);
    act(() => {
      devices.calls[0]!.setup.events.onState("failed");
    });
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toBe(
      "Voice could not connect to the others and is off. Press Talk to try again.",
    );
    expect(buttonWithText(mounted.container, "Mute")).toBeUndefined();
    await click(buttonWithText(mounted.container, "Talk"));
    expect(server.openedCount()).toBe(2);
    expect(mounted.container.querySelector('[role="alert"]')).toBeNull();
    mounted.unmount();
  });

  it("says the microphone was not allowed when it is refused, and Talk tries again", async () => {
    const devices = fakeDevices(() => Promise.reject(new DOMException("no", "NotAllowedError")));
    const { services, server } = voiceServices(devices);
    const mounted = await mount(withServices(<VoiceControls membership={membership} />, services));
    await click(buttonWithText(mounted.container, "Talk"));
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "Your microphone was not allowed",
    );
    // The browser's own words, to read out to whoever is helping.
    expect(mounted.container.textContent).toContain("The browser said: NotAllowedError: no");
    expect(buttonWithText(mounted.container, "Talk")).toBeDefined();
    expect(server.openedCount()).toBe(0);
    mounted.unmount();
  });

  it("says this page cannot use a microphone where the browser offers none", async () => {
    const devices = fakeDevices(() => Promise.reject(new DOMException("no", "NotSupportedError")));
    const { services } = voiceServices(devices);
    const mounted = await mount(withServices(<VoiceControls membership={membership} />, services));
    await click(buttonWithText(mounted.container, "Talk"));
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "needs the https:// address",
    );
    mounted.unmount();
  });

  it("is on the viewer's page and with the host's player", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "headphones");
    window.sessionStorage.setItem("orpheum.together.viewer", JSON.stringify(membership));
    window.sessionStorage.setItem(
      "orpheum.together.host",
      JSON.stringify({ sessionId: 1, credential: "host-credential" }),
    );
    const { services } = voiceServices(fakeDevices());
    const viewer = await mount(withServices(<TogetherViewer />, services));
    expect(viewer.container.querySelector('section[aria-label="Voice"] button')?.textContent).toBe(
      "Talk",
    );
    viewer.unmount();
    const host = await mount(
      withServices(
        <HostPlaysTogether fileId={5} video={null} startPositionMs={0} cannotKeepUp={undefined} />,
        services,
      ),
    );
    expect(host.container.querySelector('section[aria-label="Voice"] button')?.textContent).toBe(
      "Talk",
    );
    host.unmount();
  });
});
