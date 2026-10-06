// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import type { TogetherSocketListeners } from "./together.ts";
import { firstReconnectWaitMs } from "./together-connection.ts";
import {
  createTogetherVoice,
  microphoneFailureOf,
  type TogetherVoice,
  type VoiceCall,
  type VoiceCallSetup,
  type VoiceMicrophone,
} from "./together-voice.ts";

/** The server's end of each socket voice opens, and every message sent on them, parsed. */
interface FakeServer {
  readonly sockets: TogetherSocketListeners[];
  readonly sent: Record<string, unknown>[];
  readonly closedHere: number[];
  readonly open: (listeners: TogetherSocketListeners) => {
    send: (text: string) => void;
    close: () => void;
  };
  /** Tells the last socket message, as the server's JSON. */
  readonly tell: (message: Record<string, unknown>) => void;
  /** The rtc messages sent, without their v. */
  readonly signalsSent: () => Record<string, unknown>[];
}

function fakeServer(): FakeServer {
  const sockets: TogetherSocketListeners[] = [];
  const sent: Record<string, unknown>[] = [];
  const closedHere: number[] = [];
  return {
    sockets,
    sent,
    closedHere,
    open: (listeners) => {
      sockets.push(listeners);
      const index = sockets.length - 1;
      return {
        send: (text) => {
          sent.push(JSON.parse(text) as Record<string, unknown>);
        },
        close: () => {
          closedHere.push(index);
        },
      };
    },
    tell: (message) => {
      sockets.at(-1)!.onMessage(JSON.stringify({ v: 2, ...message }));
    },
    signalsSent: () =>
      sent
        .filter((each) => each["type"] === "rtc")
        .map((each) => Object.fromEntries(Object.entries(each).filter(([key]) => key !== "v"))),
  };
}

/** A call as a test sees it: what it was made with and what was done to it. */
interface FakeCall extends VoiceCall {
  readonly setup: VoiceCallSetup<FakeMicrophone>;
  readonly steps: string[];
  gain: number | undefined;
  isClosed: boolean;
}

function fakeCall(setup: VoiceCallSetup<FakeMicrophone>): FakeCall {
  const steps: string[] = [];
  const call: FakeCall = {
    setup,
    steps,
    gain: undefined,
    isClosed: false,
    offer: () => {
      steps.push("offer");
      return Promise.resolve("offer-sdp");
    },
    answer: (offerSdp) => {
      steps.push(`answer ${offerSdp}`);
      return Promise.resolve(`answer to ${offerSdp}`);
    },
    accept: (answerSdp) => {
      steps.push(`accept ${answerSdp}`);
      return Promise.resolve();
    },
    addCandidate: (candidate) => {
      steps.push(`candidate ${candidate}`);
      return Promise.resolve();
    },
    setGain: (gain) => {
      call.gain = gain;
    },
    close: () => {
      call.isClosed = true;
    },
  };
  return call;
}

type FakeMicrophone = VoiceMicrophone & {
  readonly track: MediaStreamTrack;
  readonly isStopped: () => boolean;
};

function fakeMicrophone(): FakeMicrophone {
  let isStopped = false;
  const track = {
    enabled: true,
    stop: () => {
      isStopped = true;
    },
  } as unknown as MediaStreamTrack;
  return { track, isStopped: () => isStopped, getAudioTracks: () => [track] };
}

/** Voice over fakes, and what a test reads of them. */
interface Rig {
  readonly voice: TogetherVoice;
  readonly server: FakeServer;
  readonly microphone: FakeMicrophone;
  readonly calls: FakeCall[];
  readonly iceAsked: number[];
}

const relay: RTCIceServer = { urls: ["turn:orpheum.test:443"], username: "u", credential: "p" };

let open: TogetherVoice[] = [];

function rig(openMicrophone?: () => Promise<FakeMicrophone>): Rig {
  const server = fakeServer();
  const microphone = fakeMicrophone();
  const calls: FakeCall[] = [];
  const iceAsked: number[] = [];
  const voice = createTogetherVoice({
    credential: "membership",
    openSocket: server.open,
    monotonicMs: () => 1000,
    fetchIceServers: () => {
      iceAsked.push(iceAsked.length);
      return Promise.resolve([relay]);
    },
    devices: {
      openMicrophone: openMicrophone ?? (() => Promise.resolve(microphone)),
      startCall: (setup) => {
        const call = fakeCall(setup);
        calls.push(call);
        return call;
      },
    },
  });
  open.push(voice);
  return { voice, server, microphone, calls, iceAsked };
}

/** Lets every promise waiting settle. */
async function settle(): Promise<void> {
  for (let turn = 0; turn < 20; turn += 1) {
    await Promise.resolve();
  }
}

/**
 * The roster: this person (member 7) with their playback device 9 and voice device 20; Karl
 * (member 8) with playback 11 and voice 30; Ann (member 5), whose one device 15 does both.
 */
const roster = [
  {
    id: 7,
    name: "Gran",
    devices: [
      { id: 9, role: "playback" },
      { id: 20, role: "voice" },
    ],
  },
  {
    id: 8,
    name: "Karl",
    devices: [
      { id: 11, role: "playback" },
      { id: 30, role: "voice" },
    ],
  },
  { id: 5, name: "Ann", devices: [{ id: 15, role: "both" }] },
];

function welcome(deviceId: number, members: readonly unknown[] = roster): Record<string, unknown> {
  return {
    type: "welcome",
    seq: 1,
    sessionId: 1,
    memberId: 7,
    deviceId,
    group: { state: "Idle" },
    roster: members,
  };
}

/** Voice turned on and welcomed as device 20, with the roster above. */
async function talking(): Promise<Rig> {
  const started = rig();
  await started.voice.start();
  started.server.sockets[0]!.onOpen();
  started.server.tell(welcome(20));
  await settle();
  return started;
}

afterEach(() => {
  for (const voice of open) {
    voice.stop();
  }
  open = [];
  vi.useRealTimers();
});

describe("voice in a Watch Together session (watch-together.md, Voice)", () => {
  it("attaches a second device for voice with the membership's credential once the microphone opens", async () => {
    const { voice, server } = rig();
    expect(server.sockets).toHaveLength(0);
    await voice.start();
    server.sockets[0]!.onOpen();
    expect(server.sent[0]).toEqual({
      v: 2,
      type: "hello",
      credential: "membership",
      role: "voice",
    });
    expect(voice.snapshot()).toMatchObject({ status: "on", isConnected: false });
  });

  it("calls every other person's voice device, never its own person's, the lower id offering", async () => {
    const { voice, server, calls, iceAsked } = await talking();
    expect(iceAsked).toHaveLength(1);
    expect(calls).toHaveLength(2);
    for (const call of calls) {
      expect(call.setup.iceServers).toEqual([relay]);
    }
    // Device 20 offers to 30, the higher; Ann's 15 is lower, so 20 waits for her offer.
    expect(server.signalsSent()).toEqual([{ type: "rtc", toDeviceId: 30, sdp: "offer-sdp" }]);
    expect(calls.map((call) => call.steps)).toEqual([["offer"], []]);
    expect(voice.snapshot()).toMatchObject({
      isConnected: true,
      partners: [
        { deviceId: 30, name: "Karl", state: "connecting" },
        { deviceId: 15, name: "Ann", state: "connecting" },
      ],
    });
  });

  it("answers the lower device's offer, and takes the higher device's answer to its own", async () => {
    const { server, calls } = await talking();
    server.tell({ type: "rtc", fromDeviceId: 15, sdp: "ann-offer" });
    server.tell({ type: "rtc", fromDeviceId: 30, sdp: "karl-answer" });
    await settle();
    expect(calls[0]!.steps).toEqual(["offer", "accept karl-answer"]);
    expect(calls[1]!.steps).toEqual(["answer ann-offer"]);
    expect(server.signalsSent()).toContainEqual({
      type: "rtc",
      toDeviceId: 15,
      sdp: "answer to ann-offer",
    });
  });

  it("relays candidates both ways, each taken after the offer before it", async () => {
    const { server, calls } = await talking();
    server.tell({ type: "rtc", fromDeviceId: 15, sdp: "ann-offer" });
    server.tell({ type: "rtc", fromDeviceId: 15, candidate: '{"candidate":"a"}' });
    calls[0]!.setup.events.onCandidate('{"candidate":"mine"}');
    await settle();
    expect(calls[1]!.steps).toEqual(["answer ann-offer", 'candidate {"candidate":"a"}']);
    expect(server.signalsSent()).toContainEqual({
      type: "rtc",
      toDeviceId: 30,
      candidate: '{"candidate":"mine"}',
    });
  });

  it("ends the call with a device that leaves the roster, and ignores what it still sends", async () => {
    const { voice, server, calls } = await talking();
    server.tell({ type: "roster", roster: [roster[0], roster[2]] });
    await settle();
    expect(calls[0]!.isClosed).toBe(true);
    expect(calls[1]!.isClosed).toBe(false);
    expect(voice.snapshot().partners.map((partner) => partner.deviceId)).toEqual([15]);
    server.tell({ type: "rtc", fromDeviceId: 30, sdp: "late" });
    await settle();
    expect(calls[0]!.steps).toEqual(["offer"]);
  });

  it("makes every call again when welcomed anew after its socket connected again", async () => {
    vi.useFakeTimers();
    const { server, calls, iceAsked } = await talking();
    server.sockets[0]!.onClose(1006);
    vi.advanceTimersByTime(firstReconnectWaitMs);
    server.sockets[1]!.onOpen();
    server.tell(welcome(40));
    await settle();
    expect(calls.slice(0, 2).every((call) => call.isClosed)).toBe(true);
    expect(calls).toHaveLength(4);
    expect(iceAsked).toHaveLength(2);
    // Now device 40: higher than 30 and 15, so both of them offer.
    expect(calls.slice(2).map((call) => call.steps)).toEqual([[], []]);
  });

  it("shows a call connected when its state says so", async () => {
    const { voice, calls } = await talking();
    calls[0]!.setup.events.onState("connected");
    expect(voice.snapshot().partners[0]).toEqual({
      deviceId: 30,
      name: "Karl",
      state: "connected",
    });
  });

  it("mutes by switching the microphone's track off, and stops it and detaches when stopped", async () => {
    const { voice, server, microphone, calls } = await talking();
    voice.setMuted(true);
    expect(microphone.track.enabled).toBe(false);
    expect(voice.snapshot().isMuted).toBe(true);
    voice.setMuted(false);
    expect(microphone.track.enabled).toBe(true);
    voice.stop();
    expect(microphone.isStopped()).toBe(true);
    expect(server.closedHere).toEqual([0]);
    expect(calls.every((call) => call.isClosed)).toBe(true);
    expect(voice.snapshot()).toMatchObject({ status: "off", partners: [] });
  });

  it("plays the partners' voices at the gain set, apart from the film's volume", async () => {
    const { voice, server, calls } = await talking();
    expect(calls.map((call) => call.gain)).toEqual([1, 1]);
    voice.setPartnerGain(0.4);
    expect(calls.map((call) => call.gain)).toEqual([0.4, 0.4]);
    expect(voice.snapshot().partnerGain).toBe(0.4);
    server.tell({
      type: "roster",
      roster: [...roster, { id: 6, name: "Bo", devices: [{ id: 50, role: "voice" }] }],
    });
    await settle();
    expect(calls[2]!.gain).toBe(0.4);
  });

  it("says why when the microphone is refused, and attaches nothing", async () => {
    const refused = rig(() => Promise.reject(new DOMException("no", "NotAllowedError")));
    await refused.voice.start();
    expect(refused.voice.snapshot()).toMatchObject({ status: "failed", failure: "refused" });
    expect(refused.server.sockets).toHaveLength(0);
  });

  it("names each way a microphone fails to open", () => {
    const failureOf = (name: string): string => microphoneFailureOf(new DOMException("", name));
    expect(failureOf("NotAllowedError")).toBe("refused");
    expect(failureOf("SecurityError")).toBe("refused");
    expect(failureOf("NotFoundError")).toBe("none");
    expect(failureOf("NotReadableError")).toBe("busy");
    expect(failureOf("NotSupportedError")).toBe("unsupported");
    expect(failureOf("Whatever")).toBe("failed");
    expect(microphoneFailureOf("not an error")).toBe("failed");
  });

  it("closes a microphone that opens only after voice was turned off", async () => {
    let opened: (microphone: FakeMicrophone) => void = () => undefined;
    const late = rig(
      () =>
        new Promise((resolve) => {
          opened = resolve;
        }),
    );
    const starting = late.voice.start();
    expect(late.voice.snapshot().status).toBe("starting");
    late.voice.stop();
    opened(late.microphone);
    await starting;
    expect(late.microphone.isStopped()).toBe(true);
    expect(late.server.sockets).toHaveLength(0);
    expect(late.voice.snapshot().status).toBe("off");
  });
});
