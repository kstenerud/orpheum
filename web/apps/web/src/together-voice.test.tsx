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
import { act, useState, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { currentNotice, dismissNotice } from "./notice.ts";
import { buttonWithText, click, enter, mount, servedBy, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";
import { HostPlaysTogether } from "./together-host.tsx";
import { noteHostedElsewhere, readHostedElsewhere } from "./together-mine.ts";
import { TogetherViewer } from "./together-viewer.tsx";
import {
  HostVoiceProvider,
  movesSoundToTheEarpiece,
  talkRingEveryMs,
  talkRingOf,
  useHostVoice,
  useVoice,
  VoiceControls,
} from "./together-voice.tsx";

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
      isWaitingForATap: () => false,
      subscribeToTapWaiting: () => () => undefined,
      playAtTap: () => undefined,
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
        { id: 3, role: "video" },
        { id: 4, role: "panel" },
      ],
    },
    {
      id: 2,
      name: "Gran",
      devices: [
        { id: 7, role: "video" },
        { id: 8, role: "panel" },
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
function Voice(): JSX.Element {
  const voice = useVoice(membership, undefined);
  return <VoiceControls {...voice} />;
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
    const mounted = await mount(withServices(<Voice />, services));
    expect(mounted.container.textContent).toContain("Use headphones");
    await server.accept();
    expect(server.heard[0]).toEqual({
      v: 2,
      type: "hello",
      channel: "together",
      credential: "viewer-credential",
      role: "panel",
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
    const mounted = await mount(withServices(<Voice />, services));
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

  it("asks for a tap while the browser refuses to play the voices, and plays them at it (the owner, 2026-10-11)", async () => {
    const devices = fakeDevices();
    let isWaiting = true;
    let taps = 0;
    const listeners = new Set<() => void>();
    const waitingDevices = {
      ...devices.devices,
      isWaitingForATap: () => isWaiting,
      subscribeToTapWaiting: (listener: () => void) => {
        listeners.add(listener);
        return () => listeners.delete(listener);
      },
      playAtTap: () => {
        taps += 1;
        isWaiting = false;
        for (const listener of listeners) {
          listener();
        }
      },
    };
    const { services, server } = voiceServices({ ...devices, devices: waitingDevices });
    const mounted = await mount(withServices(<Voice />, services));
    await server.accept();
    await server.say(welcome);
    await click(buttonWithText(mounted.container, "Tap to hear everyone's voices"));
    expect(taps).toBe(1);
    expect(buttonWithText(mounted.container, "Tap to hear everyone's voices")).toBeUndefined();
    mounted.unmount();
  });

  it("says when no one else is here to hear", async () => {
    const devices = fakeDevices();
    const { services, server } = voiceServices(devices, []);
    const mounted = await mount(withServices(<Voice />, services));
    await server.accept();
    await server.say({
      ...welcome,
      roster: welcome.roster.map((member) => ({
        ...member,
        devices: member.devices.filter((device) => device.role !== "panel"),
      })),
    });
    expect(devices.calls).toHaveLength(0);
    expect(mounted.container.textContent).toContain("Voice: no one else is here yet.");
    mounted.unmount();
  });

  it("talks by one toggle, closing the microphone and opening it again, still hearing, and sets each person's voice volume", async () => {
    const devices = fakeDevices();
    const { services, server } = voiceServices(devices);
    const mounted = await mount(withServices(<Voice />, services));
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

  it("shows it connecting, a moving status line, while a call is made again after a failure (D281 §6)", async () => {
    const devices = fakeDevices();
    const { services, server } = voiceServices(devices);
    const mounted = await mount(withServices(<Voice />, services));
    await server.accept();
    await server.say(welcome);
    const status = (): Element | null => mounted.container.querySelector(".together-voice__status");
    expect(status()?.hasAttribute("data-connecting")).toBe(true);
    act(() => {
      devices.calls[0]!.setup.events.onState("connected");
    });
    expect(status()?.hasAttribute("data-connecting")).toBe(false);
    act(() => {
      devices.calls[0]!.setup.events.onState("failed");
    });
    // No error and no button: the call is made again by itself, and the line says so as it moves.
    expect(mounted.container.querySelector('[role="alert"]')).toBeNull();
    expect(status()?.hasAttribute("data-connecting")).toBe(true);
    expect(status()?.textContent).toBe("Voice: connecting to Karl…");
    mounted.unmount();
  });

  it("says the microphone was not allowed when it is refused, still hearing, and Talk tries again", async () => {
    const devices = fakeDevices(() => Promise.reject(new DOMException("no", "NotAllowedError")));
    const { services, server } = voiceServices(devices);
    const mounted = await mount(withServices(<Voice />, services));
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
    const mounted = await mount(withServices(<Voice />, services));
    await click(talkButton(mounted.container, "Talk"));
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "needs the https:// address",
    );
    mounted.unmount();
  });

  it("is on the viewer's page and with the host's player, the host's one voice for the tab", async () => {
    window.localStorage.setItem("orpheum.together.viewer", JSON.stringify(membership));
    const hostMembership = { sessionId: 1, credential: "host-credential" };
    window.sessionStorage.setItem("orpheum.together.host", JSON.stringify(hostMembership));
    const { services } = voiceServices(fakeDevices());
    const viewer = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(viewer.container, "Tap to continue"));
    expect(
      viewer.container
        .querySelector('section[aria-label="Voice"] button')
        ?.getAttribute("aria-label"),
    ).toBe("Talk");
    viewer.unmount();
    const host = await mount(
      withServices(
        <HostVoiceProvider membership={hostMembership}>
          <HostPlaysTogether fileId={5} video={null} startPositionMs={0} cannotKeepUp={undefined} />
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

  it("warns before Talk that the film goes quiet unless headphones are on, asking no one whether they are (D336 §4)", async () => {
    Object.defineProperty(navigator, "maxTouchPoints", { value: 5, configurable: true });
    Object.defineProperty(navigator, "audioSession", { value: {}, configurable: true });
    try {
      const devices = fakeDevices();
      const { services } = voiceServices(devices);
      const mounted = await mount(withServices(<Voice />, services));
      const warning = (): Element | null => mounted.container.querySelector('[role="alertdialog"]');
      // It pops up at the press of Talk, before the microphone opens (the owner, 2026-10-11).
      expect(warning()).toBeNull();
      await click(talkButton(mounted.container, "Talk"));
      expect(warning()).not.toBeNull();
      expect(mounted.container.textContent).toContain("please connect your headphones");
      // The microphone opens at the press; OK only closes the notice.
      expect(devices.processedAsked).toHaveLength(1);
      await click(buttonWithText(mounted.container, "OK"));
      expect(warning()).toBeNull();
      expect(devices.processedAsked).toHaveLength(1);
      // Read once, it does not come back at the next press.
      await click(talkButton(mounted.container, "Stop talking"));
      await click(talkButton(mounted.container, "Talk"));
      expect(warning()).toBeNull();
      expect(devices.processedAsked).toHaveLength(2);
      mounted.unmount();
    } finally {
      Reflect.deleteProperty(navigator, "audioSession");
      Reflect.deleteProperty(navigator, "maxTouchPoints");
    }
  });
});

describe("the host's page as the session it hosts ends (D389 §1)", () => {
  const hostMembership = { sessionId: 1, credential: "host-credential" };
  let shown: { readonly unmount: () => void } | undefined;

  afterEach(() => {
    shown?.unmount();
    shown = undefined;
  });

  /** The host's tab on a film's Play page, hosting session 1, its voice its one connection to it. */
  async function hostingOnAPlayPage(): Promise<{
    readonly services: ReturnType<typeof servedBy>;
    readonly server: FakeTogetherServer;
  }> {
    window.sessionStorage.setItem("orpheum.together.host", JSON.stringify(hostMembership));
    const { services, server } = voiceServices(fakeDevices());
    services.navigation.go({ screen: "play", fileId: 9 });
    shown = await mount(
      withServices(
        <HostVoiceProvider membership={hostMembership}>
          <p>the page</p>
        </HostVoiceProvider>,
        services,
      ),
    );
    await server.accept();
    return { services, server };
  }

  /** The notice shown now, taken away. */
  function noticeShown(): string | undefined {
    const notice = currentNotice();
    if (notice !== undefined) {
      dismissNotice(notice);
    }
    return notice?.text;
  }

  it("goes home, told, as the session is ended on another of the host's devices", async () => {
    const { services, server } = await hostingOnAPlayPage();
    await server.say({ type: "ended", reason: "hostEnded" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
    expect(noticeShown()).toBe("The Watch Together session was ended on another of your devices.");
  });

  it("goes home, told, as the session is moved to another of the host's devices, noted as hosted there so no new one is offered over it (D442 §2)", async () => {
    const { services, server } = await hostingOnAPlayPage();
    await server.say({ type: "ended", reason: "movedAway" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
    expect(noticeShown()).toBe("You moved the Watch Together session to another of your devices.");
    expect(readHostedElsewhere()).toEqual({
      sessionId: hostMembership.sessionId,
      deviceName: undefined,
    });
    noteHostedElsewhere(undefined);
  });

  it("goes home, told, as the server no longer knows the session", async () => {
    const { services, server } = await hostingOnAPlayPage();
    await server.refuse();
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
    expect(noticeShown()).toBe("The Watch Together session is over.");
  });

  it("goes home, told, as the server ends the session idle", async () => {
    const { services, server } = await hostingOnAPlayPage();
    await server.say({ type: "ended", reason: "idle" });
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(noticeShown()).toBe(
      "The Watch Together session was closed because nothing had happened in it for hours.",
    );
  });
});

describe("the host's voice as a session starts", () => {
  /** A page under the voice: a count it keeps, and whether it has the host's voice. */
  function APage(): JSX.Element {
    const [count, setCount] = useState(0);
    const voice = useHostVoice();
    return (
      <button
        type="button"
        data-voice={voice === undefined ? "none" : "had"}
        onClick={() => {
          setCount(count + 1);
        }}
      >
        {`pressed ${String(count)}`}
      </button>
    );
  }

  /** The tab, hosting nothing until Start. */
  function TheTab(): JSX.Element {
    const [hosts, setHosts] = useState<{ sessionId: number; credential: string } | undefined>();
    return (
      <>
        <button
          type="button"
          onClick={() => {
            setHosts({ sessionId: 1, credential: "host-credential" });
          }}
        >
          Start
        </button>
        <HostVoiceProvider membership={hosts}>
          <APage />
        </HostVoiceProvider>
      </>
    );
  }

  it("leaves the page under it as it was: a film playing went on, not started again (the owner, 2026-11-07)", async () => {
    const { services, server } = voiceServices(fakeDevices());
    const mounted = await mount(withServices(<TheTab />, services));
    await click(buttonWithText(mounted.container, "pressed 0"));
    await click(buttonWithText(mounted.container, "Start"));
    await server.accept();
    const page = buttonWithText(mounted.container, "pressed 1");
    expect(page?.getAttribute("data-voice")).toBe("had");
    mounted.unmount();
  });
});
