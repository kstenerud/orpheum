// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import {
  startingTuning,
  type BrowserVoiceDevices,
  type MicrophoneProcessing,
  type SyncTuning,
  type VoiceCall,
  type VoiceCallSetup,
  type VoiceMicrophone,
} from "@orpheum/sdk";
import { act, type JSX } from "react";
import { beforeEach, describe, expect, it } from "vitest";

import { buttonWithText, click, enter, mount, servedBy, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";
import { HostPlaysTogether } from "./together-host.tsx";
import { TogetherViewer } from "./together-viewer.tsx";
import {
  HostVoiceProvider,
  movesSoundToTheEarpiece,
  talkRingEveryMs,
  talkRingOf,
  useVoice,
  VoiceControls,
} from "./together-voice.tsx";
import type { AudioOutput } from "./accounts.ts";

/** A call as a test sees it. */
interface FakeCall extends VoiceCall {
  readonly setup: VoiceCallSetup<MediaStream>;
  gain: number | undefined;
  /** What it was told to send since it was made: the microphone, or nothing. */
  readonly sent: (VoiceMicrophone | undefined)[];
}

/** Stand-in devices for voice: a microphone of one track, and the calls made. */
interface FakeDevices {
  readonly devices: BrowserVoiceDevices;
  readonly track: MediaStreamTrack;
  readonly calls: FakeCall[];
  /** Whether each opening of the microphone asked for the browser's processing. */
  readonly processedAsked: MicrophoneProcessing[];
  /** How many times the microphone's track was stopped. */
  readonly stoppedTracks: () => number;
}

function fakeDevices(openMicrophone?: () => Promise<MediaStream>): FakeDevices {
  let stopped = 0;
  const track = {
    enabled: true,
    stop: () => {
      stopped += 1;
    },
  } as unknown as MediaStreamTrack;
  const microphone = { getAudioTracks: () => [track] } as unknown as MediaStream;
  const calls: FakeCall[] = [];
  const processedAsked: MicrophoneProcessing[] = [];
  return {
    track,
    calls,
    processedAsked,
    stoppedTracks: () => stopped,
    devices: {
      openMicrophone: (processing) => {
        processedAsked.push(processing);
        return openMicrophone?.() ?? Promise.resolve(microphone);
      },
      startCall: (setup) => {
        const call: FakeCall = {
          setup,
          gain: undefined,
          sent: [],
          send: (sent) => {
            call.sent.push(sent);
          },
          offer: () => Promise.resolve("offer"),
          answer: () => Promise.resolve("answer"),
          accept: () => Promise.resolve(),
          addCandidate: () => Promise.resolve(),
          setGain: (gain) => {
            call.gain = gain;
          },
          measure: () => Promise.reject(new Error("not measured here")),
          close: () => undefined,
        };
        calls.push(call);
        return call;
      },
      readMicrophoneLevel: () => 0.1,
      prepareToHear: () => undefined,
      microphoneClosed: () => undefined,
      close: () => undefined,
    },
  };
}

const membership = { sessionId: 1, credential: "viewer-credential" };

/** A tuning whose microphone processing is the opposite of the starting one's in both parts. */
const echoCancelledTuning: SyncTuning = {
  ...startingTuning,
  shouldCancelMicrophoneEcho: true,
  shouldBrowserLevelMicrophone: false,
};

const relay = { urls: ["turn:orpheum.test:443"], username: "u", credential: "p", expiresAt: 1 };

/** The welcome of this person's voice device 8; Karl's voice device 4 is in the session too. */
const welcome = {
  type: "welcome",
  tuning: echoCancelledTuning,
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

/** The Talk button, found by its name, which is for screen readers: it shows only an icon. */
function talkButton(container: Element, name: string): Element | undefined {
  return container.querySelector(`.together-voice__talk[aria-label="${name}"]`) ?? undefined;
}

/** A person's voice and its controls, as a page has them. */
function Voice(props: { readonly audioOutput: AudioOutput }): JSX.Element {
  const voice = useVoice(membership, undefined);
  return <VoiceControls {...voice} audioOutput={props.audioOutput} />;
}

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("voice controls in a Watch Together session (watch-together.md, Voice; D281)", () => {
  it("hears everyone from the start: attaches for voice with no microphone, asks for the relay, and says whom it hears", async () => {
    const devices = fakeDevices();
    const heard: Heard[] = [];
    const { services, server } = voiceServices(devices, heard);
    const mounted = await mount(withServices(<Voice audioOutput="headphones" />, services));
    expect(mounted.container.textContent).toContain("Use headphones");
    await server.accept();
    expect(server.heard[0]).toEqual({
      v: 2,
      type: "hello",
      credential: "viewer-credential",
      role: "voice",
    });
    expect(devices.processedAsked).toEqual([]); // no microphone until Talk
    expect(mounted.container.textContent).toContain("Voice: connecting…");
    await server.say(welcome);
    expect(heard.map((each) => each.methodAndPath)).toEqual(["GET /together/1/ice"]);
    expect(devices.calls).toHaveLength(1);
    expect(devices.calls[0]!.setup.microphone).toBeUndefined();
    expect(devices.calls[0]!.setup.iceServers).toEqual([
      { urls: relay.urls, username: "u", credential: "p" },
    ]);
    expect(mounted.container.textContent).toContain("Voice: connecting to Karl…");
    act(() => {
      devices.calls[0]!.setup.events.onState("connected");
    });
    expect(mounted.container.textContent).toContain("Voice: hearing Karl");
    mounted.unmount();
  });

  it("talks on the call already up, the microphone processed as the session says", async () => {
    const devices = fakeDevices();
    const { services, server } = voiceServices(devices);
    const mounted = await mount(withServices(<Voice audioOutput="headphones" />, services));
    await server.accept();
    await server.say(welcome);
    await click(talkButton(mounted.container, "Talk"));
    expect(devices.processedAsked).toEqual([{ shouldCancelEcho: true, shouldLevel: false }]);
    expect(devices.calls).toHaveLength(1);
    expect(devices.calls[0]!.sent).toHaveLength(1);
    expect(server.openedCount()).toBe(1);
    expect(mounted.container.textContent).toContain("The others hear you.");
    mounted.unmount();
  });

  it("says when no one else is here to hear", async () => {
    const devices = fakeDevices();
    const { services, server } = voiceServices(devices, []);
    const mounted = await mount(withServices(<Voice audioOutput="headphones" />, services));
    await server.accept();
    await server.say({
      ...welcome,
      roster: welcome.roster.map((member) => ({
        ...member,
        devices: member.devices.filter((device) => device.role !== "voice"),
      })),
    });
    expect(devices.calls).toHaveLength(0);
    expect(mounted.container.textContent).toContain("Voice: no one else is here yet.");
    mounted.unmount();
  });

  it("talks by one toggle, closing the microphone and opening it again, still hearing, and sets each person's voice volume", async () => {
    const devices = fakeDevices();
    const { services, server } = voiceServices(devices);
    const mounted = await mount(withServices(<Voice audioOutput="headphones" />, services));
    await server.accept();
    await server.say(welcome);
    const talk = (): Element | undefined =>
      mounted.container.querySelector(".together-voice__talk") ?? undefined;
    expect(talk()?.getAttribute("aria-pressed")).toBe("false");
    await click(talk());
    expect(talk()?.getAttribute("aria-pressed")).toBe("true");
    expect(talk()?.getAttribute("aria-label")).toBe("Stop talking");
    expect(buttonWithText(mounted.container, "Mute")).toBeUndefined();
    expect(devices.calls[0]!.sent).toHaveLength(1);
    // Its ring grows with the voice going out: the fake microphone's 0.1, a speaking voice, fills it.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, talkRingEveryMs + 50));
    });
    expect((talk() as HTMLElement | undefined)?.style.getPropertyValue("--level")).toBe("1");
    const karl = [...mounted.container.querySelectorAll("label")].find((label) =>
      label.textContent.startsWith("Karl's voice"),
    );
    await enter(karl?.querySelector("input"), "40");
    expect(devices.calls[0]!.gain).toBe(0.4);
    // The second press closes the microphone, rather than muting it, and the call stays up.
    await click(talk());
    expect(devices.stoppedTracks()).toBe(1);
    expect(devices.calls[0]!.sent.at(-1)).toBeUndefined();
    expect(talk()?.getAttribute("aria-pressed")).toBe("false");
    expect(talk()?.getAttribute("aria-label")).toBe("Talk");
    expect(mounted.container.textContent).toContain("Voice: connecting to Karl…");
    // And a third opens it again.
    await click(talk());
    expect(devices.processedAsked).toHaveLength(2);
    expect(talk()?.getAttribute("aria-pressed")).toBe("true");
    mounted.unmount();
  });

  it("turns off saying so when its call fails, and connects again at a press", async () => {
    const devices = fakeDevices();
    const { services, server } = voiceServices(devices);
    const mounted = await mount(withServices(<Voice audioOutput="headphones" />, services));
    await server.accept();
    await server.say(welcome);
    act(() => {
      devices.calls[0]!.setup.events.onState("failed");
    });
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toBe(
      "Voice could not connect to the others and is off.",
    );
    expect(talkButton(mounted.container, "Stop talking")).toBeUndefined();
    await click(buttonWithText(mounted.container, "Connect voice again"));
    expect(server.openedCount()).toBe(2);
    expect(mounted.container.querySelector('[role="alert"]')).toBeNull();
    mounted.unmount();
  });

  it("says the microphone was not allowed when it is refused, still hearing, and Talk tries again", async () => {
    const devices = fakeDevices(() => Promise.reject(new DOMException("no", "NotAllowedError")));
    const { services, server } = voiceServices(devices);
    const mounted = await mount(withServices(<Voice audioOutput="headphones" />, services));
    await server.accept();
    await click(talkButton(mounted.container, "Talk"));
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "Your microphone was not allowed",
    );
    // The browser's own words, to read out to whoever is helping.
    expect(mounted.container.textContent).toContain("The browser said: NotAllowedError: no");
    expect(talkButton(mounted.container, "Talk")).toBeDefined();
    expect(server.openedCount()).toBe(1);
    mounted.unmount();
  });

  it("says this page cannot use a microphone where the browser offers none", async () => {
    const devices = fakeDevices(() => Promise.reject(new DOMException("no", "NotSupportedError")));
    const { services } = voiceServices(devices);
    const mounted = await mount(withServices(<Voice audioOutput="headphones" />, services));
    await click(talkButton(mounted.container, "Talk"));
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "needs the https:// address",
    );
    mounted.unmount();
  });

  it("is on the viewer's page and with the host's player, the host's one voice for the tab", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "headphones");
    window.localStorage.setItem("orpheum.together.viewer", JSON.stringify(membership));
    const hostMembership = { sessionId: 1, credential: "host-credential" };
    window.sessionStorage.setItem("orpheum.together.host", JSON.stringify(hostMembership));
    const { services } = voiceServices(fakeDevices());
    const viewer = await mount(withServices(<TogetherViewer />, services));
    expect(
      viewer.container
        .querySelector('section[aria-label="Voice"] button')
        ?.getAttribute("aria-label"),
    ).toBe("Talk");
    viewer.unmount();
    const host = await mount(
      withServices(
        <HostVoiceProvider membership={hostMembership}>
          <HostPlaysTogether
            fileId={5}
            video={null}
            startPositionMs={0}
            cannotKeepUp={undefined}
            audioOutput="headphones"
          />
        </HostVoiceProvider>,
        services,
      ),
    );
    expect(
      host.container
        .querySelector('section[aria-label="Voice"] button')
        ?.getAttribute("aria-label"),
    ).toBe("Talk");
    host.unmount();
  });
});

describe("the Talk button's ring (D281 §7)", () => {
  it("is full for a speaking voice, still shows a quiet one, and is empty in silence", () => {
    expect(talkRingOf(0.1)).toBe(1);
    expect(talkRingOf(0.4)).toBe(1);
    expect(talkRingOf(0.004)).toBeCloseTo(0.2);
    expect(talkRingOf(0)).toBe(0);
  });
});

describe("voice on an iPhone without headphones", () => {
  it("is told by a touch screen and Safari's audio session, not by the browser's name", () => {
    expect(movesSoundToTheEarpiece({ maxTouchPoints: 5, audioSession: {} })).toBe(true);
    expect(movesSoundToTheEarpiece({ maxTouchPoints: 0, audioSession: {} })).toBe(false);
    expect(movesSoundToTheEarpiece({ maxTouchPoints: 5 })).toBe(false);
  });

  it("warns before Talk that the film goes quiet, unless headphones are on", async () => {
    Object.defineProperty(navigator, "maxTouchPoints", { value: 5, configurable: true });
    Object.defineProperty(navigator, "audioSession", { value: {}, configurable: true });
    try {
      for (const [audioOutput, isWarned] of [
        ["built_in_speakers", true],
        ["headphones", false],
      ] as const) {
        const devices = fakeDevices();
        const { services } = voiceServices(devices);
        const mounted = await mount(withServices(<Voice audioOutput={audioOutput} />, services));
        const warning = (): Element | null =>
          mounted.container.querySelector('[role="alertdialog"]');
        // It pops up at the press of Talk, before the microphone opens (the owner, 2026-10-11).
        expect(warning()).toBeNull();
        await click(talkButton(mounted.container, "Talk"));
        expect(warning() !== null).toBe(isWarned);
        expect(
          mounted.container.textContent.includes("the film's sound moves to the earpiece"),
        ).toBe(isWarned);
        // The microphone opens at the press either way; OK only closes the notice.
        expect(devices.processedAsked).toHaveLength(1);
        if (isWarned) {
          await click(buttonWithText(mounted.container, "OK"));
          expect(warning()).toBeNull();
          expect(devices.processedAsked).toHaveLength(1);
          // Read once, it does not come back at the next press.
          await click(talkButton(mounted.container, "Stop talking"));
          await click(talkButton(mounted.container, "Talk"));
          expect(warning()).toBeNull();
          expect(devices.processedAsked).toHaveLength(2);
        }
        mounted.unmount();
      }
    } finally {
      Reflect.deleteProperty(navigator, "audioSession");
      Reflect.deleteProperty(navigator, "maxTouchPoints");
    }
  });
});
