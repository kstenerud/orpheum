// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import type { TogetherSocketListeners } from "./together.ts";
import { firstReconnectWaitMs } from "./together-connection.ts";
import {
  voiceFirstStatsAfterMs,
  voiceStatsEveryMs,
  type VoiceCallStats,
} from "./together-voice-stats.ts";
import { isVoiceLive } from "./voice-live.ts";
import {
  createTogetherVoice,
  iceServersFreshForMs,
  partnerGainMost,
  microphoneFailureOf,
  type MicrophoneProcessing,
  type TogetherVoice,
  type VoiceCall,
  type VoiceCallSetup,
  type VoiceMicrophone,
} from "./together-voice.ts";

/** Processing with gain control and noise suppression, no echo cancelling. */
const levelled: MicrophoneProcessing = { shouldCancelEcho: false, shouldLevel: true };

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

/** The notes of kind the voice device sent, in the order sent. */
function notesOf(server: FakeServer, kind: string): unknown[] {
  return server.sent
    .filter((each) => each["type"] === "note")
    .map((each) => each["note"])
    .filter((note) => (note as { readonly kind?: unknown } | undefined)?.kind === kind);
}

/** What every fake call measures. */
const measuredCall: VoiceCallStats = {
  roundTripMs: 40,
  jitterBufferMs: 35,
  jitterBufferTargetMs: 30,
  concealedShare: 0.001,
  packetsLost: 2,
  packetsLate: 0,
  fecPackets: 1,
  path: "direct",
  receivedLevel: 0.12,
  sentLevel: 0.08,
  playback: "webAudio",
};

/** A call as a test sees it: what it was made with and what was done to it. */
interface FakeCall extends VoiceCall {
  readonly setup: VoiceCallSetup<FakeMicrophone>;
  readonly steps: string[];
  gain: number | undefined;
  /** What it was told to send since it was made: a microphone, or nothing. */
  readonly sent: (VoiceMicrophone | undefined)[];
  isClosed: boolean;
  /** Whether taking the other's answer fails, as a malformed one does. */
  isAnswerRefused: boolean;
}

function fakeCall(setup: VoiceCallSetup<FakeMicrophone>): FakeCall {
  const steps: string[] = [];
  const call: FakeCall = {
    setup,
    steps,
    gain: undefined,
    sent: [],
    isClosed: false,
    isAnswerRefused: false,
    send: (microphone) => {
      call.sent.push(microphone);
    },
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
      return call.isAnswerRefused ? Promise.reject(new Error("bad answer")) : Promise.resolve();
    },
    addCandidate: (candidate) => {
      steps.push(`candidate ${candidate}`);
      return Promise.resolve();
    },
    measure: () => Promise.resolve(measuredCall),
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
  /** Each time the devices were told the microphone closed. */
  readonly ended: number[];
  /** Each time the devices were readied to play the others' voices. */
  readonly prepared: number[];
  /** Whether each opening of the microphone asked for the browser's processing. */
  readonly processedAsked: MicrophoneProcessing[];
}

const relay: RTCIceServer = { urls: ["turn:orpheum.test:443"], username: "u", credential: "p" };

let open: TogetherVoice[] = [];
/** The rig's monotonic clock, which a test moves on. */
let rigNowMs = 1000;

function rig(openMicrophone?: () => Promise<FakeMicrophone>): Rig {
  const server = fakeServer();
  const microphone = fakeMicrophone();
  const calls: FakeCall[] = [];
  const iceAsked: number[] = [];
  const ended: number[] = [];
  const prepared: number[] = [];
  const processedAsked: MicrophoneProcessing[] = [];
  const voice = createTogetherVoice({
    credential: "membership",
    openSocket: server.open,
    monotonicMs: () => rigNowMs,
    fetchIceServers: () => {
      iceAsked.push(iceAsked.length);
      return Promise.resolve([relay]);
    },
    devices: {
      openMicrophone: (processing) => {
        processedAsked.push(processing);
        return openMicrophone?.() ?? Promise.resolve(microphone);
      },
      startCall: (setup) => {
        const call = fakeCall(setup);
        calls.push(call);
        return call;
      },
      prepareToHear: () => {
        prepared.push(prepared.length);
      },
      microphoneClosed: () => {
        ended.push(ended.length);
      },
    },
  });
  open.push(voice);
  return { voice, server, microphone, calls, iceAsked, ended, prepared, processedAsked };
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

/** Voice heard, with no microphone, and welcomed as device 20, with the roster above. */
async function hearing(): Promise<Rig> {
  const heard = rig();
  heard.voice.hear();
  heard.server.sockets[0]!.onOpen();
  heard.server.tell(welcome(20));
  await settle();
  return heard;
}

/** Voice talked on and welcomed as device 20, with the roster above. */
async function talking(): Promise<Rig> {
  const started = rig();
  await started.voice.start(levelled);
  started.server.sockets[0]!.onOpen();
  started.server.tell(welcome(20));
  await settle();
  return started;
}

afterEach(() => {
  rigNowMs = 1000;
  for (const voice of open) {
    voice.leave();
  }
  open = [];
  vi.useRealTimers();
});

describe("voice in a Watch Together session (watch-together.md, Voice)", () => {
  it("attaches a second device for voice with the membership's credential as it hears, before any microphone (D281 §1)", async () => {
    const { voice, server, processedAsked, prepared } = rig();
    expect(server.sockets).toHaveLength(0);
    voice.hear();
    expect(prepared).toEqual([0]);
    expect(processedAsked).toEqual([]);
    expect(voice.snapshot()).toMatchObject({ status: "off", isHearing: true });
    await voice.start(levelled);
    expect(processedAsked).toEqual([levelled]); // as the welcome's tuning says
    expect(server.sockets).toHaveLength(1); // Talk makes no second device
    server.sockets[0]!.onOpen();
    expect(server.sent[0]).toEqual({
      v: 2,
      type: "hello",
      credential: "membership",
      role: "voice",
    });
    expect(voice.snapshot()).toMatchObject({ status: "on", isConnected: false });
  });

  it("hears everyone before it talks, sending nothing, and talks on the same calls, made no new (D281 §1, §2)", async () => {
    const { voice, server, calls, microphone } = await hearing();
    expect(calls).toHaveLength(2);
    expect(calls.map((call) => call.setup.microphone)).toEqual([undefined, undefined]);
    expect(server.signalsSent()).toEqual([{ type: "rtc", toDeviceId: 30, sdp: "offer-sdp" }]);
    await voice.start(levelled);
    expect(calls).toHaveLength(2);
    expect(calls.map((call) => call.sent)).toEqual([[microphone], [microphone]]);
    expect(server.signalsSent()).toHaveLength(1); // no new offer
    voice.stop();
    expect(calls.map((call) => call.sent)).toEqual([
      [microphone, undefined],
      [microphone, undefined],
    ]);
    expect(microphone.isStopped()).toBe(true);
    expect(calls.every((call) => !call.isClosed)).toBe(true);
    expect(voice.snapshot()).toMatchObject({ status: "off", isHearing: true });
  });

  it("tells the core's log as its person talks, mutes, unmutes and stops (D281 §2, §3)", async () => {
    const { voice, server } = await hearing();
    voice.setMuted(true); // no microphone: nothing to mute
    await voice.start(levelled);
    voice.setMuted(true);
    voice.setMuted(false);
    voice.stop();
    expect(notesOf(server, "voiceTalk")).toEqual(
      ["talking", "muted", "unmuted", "stopped"].map((why) => ({ kind: "voiceTalk", why })),
    );
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
      memberId: 8,
      name: "Karl",
      state: "connected",
    });
  });

  it("is live while a call carries voice, so the film gives way to it, and not once stopped (D262 §2)", async () => {
    const { voice, calls } = await talking();
    expect(isVoiceLive()).toBe(false); // on, but no call connected yet
    calls[0]!.setup.events.onState("connected");
    expect(isVoiceLive()).toBe(true);
    voice.stop();
    expect(isVoiceLive()).toBe(false);
  });

  it("tells the core's log how each connected call goes, soon after it connects and every ten seconds (D262 §1)", async () => {
    vi.useFakeTimers();
    const { server, calls } = await talking();
    calls[0]!.setup.events.onState("connected"); // with Karl's device 30; Ann's, 15, connecting
    const measured = { kind: "voiceStats", voice: { partnerDeviceId: 30, ...measuredCall } };
    vi.advanceTimersByTime(voiceFirstStatsAfterMs);
    await settle();
    expect(notesOf(server, "voiceStats")).toEqual([measured]);
    vi.advanceTimersByTime(voiceStatsEveryMs - voiceFirstStatsAfterMs);
    await settle();
    expect(notesOf(server, "voiceStats")).toEqual([measured, measured]);
  });

  it("tells its devices voice ended once its microphone stops, so they undo what they did for it", async () => {
    const { voice, ended } = await talking();
    expect(ended).toEqual([]);
    voice.stop();
    expect(ended).toEqual([0]);
    voice.stop(); // already off: nothing to undo again
    expect(ended).toEqual([0]);
  });

  it("tells the core's log why voice turned off: the person left, or a call could not connect", async () => {
    const left = await talking();
    left.voice.leave();
    expect(notesOf(left.server, "voiceOff")).toEqual([{ kind: "voiceOff", why: "left" }]);
    const unreachable = await talking();
    unreachable.calls[0]!.setup.events.onState("failed");
    expect(notesOf(unreachable.server, "voiceOff")).toEqual([
      { kind: "voiceOff", why: "unreachable" },
    ]);
  });

  it("tells the core's log each step a call takes, and a signal it drops", async () => {
    const { server, calls } = await talking();
    server.tell({ type: "rtc", fromDeviceId: 15, sdp: "ann-offer" });
    server.tell({ type: "rtc", fromDeviceId: 30, sdp: "karl-answer" });
    server.tell({ type: "rtc", fromDeviceId: 99, sdp: "a stranger's offer" });
    await settle();
    calls[0]!.setup.events.onState("connected");
    calls[0]!.setup.events.onState("connected"); // told twice, noted once
    // Each call's steps in order; the two calls' run apart.
    const steps = notesOf(server, "voiceCall");
    expect(steps).toHaveLength(5);
    expect(
      steps.filter((note) => (note as { partnerDeviceId: number }).partnerDeviceId === 30),
    ).toEqual([
      { kind: "voiceCall", partnerDeviceId: 30, why: "offered" },
      { kind: "voiceCall", partnerDeviceId: 30, why: "accepted" },
      { kind: "voiceCall", partnerDeviceId: 30, why: "connected" },
    ]);
    expect(steps).toEqual(
      expect.arrayContaining([
        { kind: "voiceCall", partnerDeviceId: 15, why: "answered" },
        { kind: "voiceCall", partnerDeviceId: 99, why: "signalDropped" },
      ]),
    );
  });

  it("mutes by switching the microphone's track off, the calls kept, and stops it and detaches when left", async () => {
    const { voice, server, microphone, calls } = await talking();
    voice.setMuted(true);
    expect(microphone.track.enabled).toBe(false);
    expect(voice.snapshot().isMuted).toBe(true);
    // Muted, it still hears everyone: no call is touched (D281 §3).
    expect(calls.every((call) => !call.isClosed && call.sent.length === 0)).toBe(true);
    voice.setMuted(false);
    expect(microphone.track.enabled).toBe(true);
    voice.leave();
    expect(microphone.isStopped()).toBe(true);
    expect(server.closedHere).toEqual([0]);
    expect(calls.every((call) => call.isClosed)).toBe(true);
    expect(voice.snapshot()).toMatchObject({ status: "off", partners: [] });
  });

  it("plays each person's voice at the gain set for them, apart from the film's volume (D281 §4)", async () => {
    const { voice, server, calls } = await hearing();
    expect(calls.map((call) => call.gain)).toEqual([1, 1]); // Karl's, Ann's
    voice.setPartnerGain(8, 0.4);
    expect(calls.map((call) => call.gain)).toEqual([0.4, 1]);
    expect(voice.snapshot().partnerGains).toEqual({ 8: 0.4 });
    // A quiet microphone is made louder than it came, up to partnerGainMost.
    voice.setPartnerGain(5, 3);
    expect(calls.map((call) => call.gain)).toEqual([0.4, 3]);
    voice.setPartnerGain(5, 9);
    expect(voice.snapshot().partnerGains[5]).toBe(partnerGainMost);
    // Karl on another device is heard at his gain.
    server.tell({
      type: "roster",
      roster: [...roster.slice(0, 1), { ...roster[1], devices: [{ id: 50, role: "voice" }] }],
    });
    await settle();
    expect(calls.at(-1)!.gain).toBe(0.4);
  });

  it("turns itself off, saying so, when a call fails, and Talk tries again (the owner, 2026-10-06)", async () => {
    const { voice, server, microphone, calls } = await talking();
    calls[0]!.setup.events.onState("failed");
    expect(voice.snapshot()).toMatchObject({
      status: "failed",
      failure: "unreachable",
      partners: [],
      isConnected: false,
    });
    expect(microphone.isStopped()).toBe(true);
    expect(server.closedHere).toEqual([0]);
    expect(calls.every((call) => call.isClosed)).toBe(true);
    await voice.start(levelled);
    expect(voice.snapshot()).toMatchObject({ status: "on", failure: undefined });
    expect(server.sockets).toHaveLength(2);
  });

  it("turns itself off when a step of a call fails", async () => {
    const { voice, server, calls } = await talking();
    // Karl's voice device, 30, is higher than this one's 20: this one offered, and takes his answer.
    calls[0]!.isAnswerRefused = true;
    server.tell({ type: "rtc", fromDeviceId: 30, sdp: "answer-sdp" });
    await settle();
    expect(voice.snapshot()).toMatchObject({ status: "failed", failure: "unreachable" });
  });

  it("asks for the relay's servers again for a call made long after it last asked (D243 §8)", async () => {
    const { server, calls, iceAsked } = await talking();
    expect(iceAsked).toHaveLength(1);
    rigNowMs += iceServersFreshForMs + 1;
    server.tell({
      type: "roster",
      roster: [...roster, { id: 6, name: "Bo", devices: [{ id: 50, role: "voice" }] }],
    });
    await settle();
    expect(iceAsked).toHaveLength(2);
    expect(calls).toHaveLength(3);
  });

  it("says why when the microphone is refused, and still hears everyone", async () => {
    const refused = rig(() => Promise.reject(new DOMException("no", "NotAllowedError")));
    await refused.voice.start(levelled);
    expect(refused.voice.snapshot()).toMatchObject({
      status: "failed",
      failure: "refused",
      browserReason: "NotAllowedError: no",
      isHearing: true,
    });
    expect(refused.server.sockets).toHaveLength(1);
    expect(refused.server.closedHere).toEqual([]);
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
    const starting = late.voice.start(levelled);
    expect(late.voice.snapshot().status).toBe("starting");
    late.voice.stop();
    opened(late.microphone);
    await starting;
    expect(late.microphone.isStopped()).toBe(true);
    expect(late.server.sockets).toHaveLength(1); // hearing, as before Talk
    expect(late.calls.flatMap((call) => call.sent)).toEqual([]);
    expect(late.voice.snapshot().status).toBe("off");
  });
});
