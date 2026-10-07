// SPDX-License-Identifier: MPL-2.0

import { setVoiceLive } from "./voice-live.ts";
import { voiceStatsEveryMs, type VoiceCallStats } from "./together-voice-stats.ts";
import {
  createTogetherConnection,
  type RelaysVoiceSignals,
  type TogetherConnection,
  type VoiceCallStep,
} from "./together-connection.ts";
import type { OpenTogetherSocket, RosterMember, VoiceSignal } from "./together.ts";

/**
 * Why the microphone did not open: the person or the browser refused it, there is none, another
 * app holds it, this page cannot reach one at all (no https, or a browser without it), or anything
 * else.
 */
export type MicrophoneFailure = "refused" | "none" | "busy" | "unsupported" | "failed";

/**
 * Why voice turned itself off: its microphone did not open, or a call with another device could
 * not connect, or dropped and could not connect again. Talk tries again (the owner, 2026-10-06).
 */
export type VoiceFailure = MicrophoneFailure | "unreachable";

/** Which failure an error from opening the microphone is, by the DOMException's name. */
export function microphoneFailureOf(error: unknown): MicrophoneFailure {
  const name: unknown =
    typeof error === "object" && error !== null ? Reflect.get(error, "name") : undefined;
  switch (name) {
    case "NotAllowedError":
    case "SecurityError":
      return "refused";
    case "NotFoundError":
    case "OverconstrainedError":
      return "none";
    case "NotReadableError":
    case "AbortError":
      return "busy";
    case "NotSupportedError":
      return "unsupported";
    default:
      return "failed";
  }
}

/** An error's name and message as the browser gave them, or its text when it has neither. */
function browserReasonOf(error: unknown): string {
  const field = (name: string): string => {
    const value: unknown =
      typeof error === "object" && error !== null ? Reflect.get(error, name) : undefined;
    return typeof value === "string" ? value : "";
  };
  const [name, message] = [field("name"), field("message")];
  if (name === "") {
    return message === "" ? String(error) : message;
  }
  return message === "" ? name : `${name}: ${message}`;
}

/**
 * The microphone, open: a browser's MediaStream is one. Its tracks are enabled while it sends,
 * and stopped when voice goes off.
 */
export type VoiceMicrophone = Pick<MediaStream, "getAudioTracks">;

/** How one call with another device goes: setting up, carrying voice, or given up on by ICE. */
export type VoiceCallState = "connecting" | "connected" | "failed";

/** What a call tells as it goes: each of its own ICE candidates, as JSON text, and its state. */
export interface VoiceCallEvents {
  readonly onCandidate: (candidate: string) => void;
  readonly onState: (state: VoiceCallState) => void;
}

/** One audio call with one other device (a browser's RTCPeerConnection), sending the microphone. */
export interface VoiceCall {
  /** Makes the offer and takes it as its own: the session description to send. */
  readonly offer: () => Promise<string>;
  /** Takes the other's offer and makes the answer: the session description to send back. */
  readonly answer: (offerSdp: string) => Promise<string>;
  /** Takes the other's answer to this call's offer. */
  readonly accept: (answerSdp: string) => Promise<void>;
  /** Takes one of the other's ICE candidates, as the JSON text it sent. */
  readonly addCandidate: (candidate: string) => Promise<void>;
  /** How loud the other device's voice plays, from 0 (silent) through 1 (as it came) to louder. */
  readonly setGain: (gain: number) => void;
  /** The call as its receiver measured it so far (D262 §1). */
  readonly measure: () => Promise<VoiceCallStats>;
  readonly close: () => void;
}

/** What a call is made with. */
export interface VoiceCallSetup<Microphone extends VoiceMicrophone> {
  readonly iceServers: readonly RTCIceServer[];
  readonly microphone: Microphone;
  readonly events: VoiceCallEvents;
}

/** What the browser does to the microphone for voice, as the welcome's tuning says. */
export interface MicrophoneProcessing {
  /** Echo cancelling: on Android it makes a call of voice, turning the film down. */
  readonly shouldCancelEcho: boolean;
  /**
   * Gain control and noise suppression: Chrome on Android runs them without a call when echo
   * cancelling is off, but Firefox on Android still makes a call of them.
   */
  readonly shouldLevel: boolean;
}

/** What voice reaches the microphone and the calls through: the browser's, or a test's. */
export interface VoiceDevices<Microphone extends VoiceMicrophone> {
  /**
   * Opens the microphone, processed as processing says, or fails with the browser's error
   * (microphoneFailureOf reads it).
   */
  readonly openMicrophone: (processing: MicrophoneProcessing) => Promise<Microphone>;
  readonly startCall: (setup: VoiceCallSetup<Microphone>) => VoiceCall;
}

/** One other person's voice device this one has a call with, and how it goes. */
export interface VoicePartner {
  readonly deviceId: number;
  /** The name its member chose. */
  readonly name: string;
  readonly state: VoiceCallState;
}

/**
 * Whether voice is off, opening the microphone, on, or turned off by a failure (the snapshot's
 * failure says which).
 */
export type VoiceStatus = "off" | "starting" | "on" | "failed";

/** What voice knows now. */
export interface VoiceSnapshot {
  readonly status: VoiceStatus;
  readonly failure: VoiceFailure | undefined;
  /**
   * The browser's own words for a microphone that did not open, its error's name and message, to
   * read out to whoever is helping; undefined for any other failure.
   */
  readonly browserReason: string | undefined;
  readonly isMuted: boolean;
  /**
   * How loud the others' voices play, from 0 to partnerGainMost, 1 as they came: apart from the
   * film's volume.
   */
  readonly partnerGain: number;
  /** Whether the voice device's own connection to the session is up. */
  readonly isConnected: boolean;
  /** Every other person's voice device, in the roster's order. */
  readonly partners: readonly VoicePartner[];
}

/** A person's voice in their Watch Together session (watch-together.md, Voice). */
export interface TogetherVoice {
  readonly snapshot: () => VoiceSnapshot;
  /** Calls listener whenever the snapshot changes; returns the unsubscribing. */
  readonly subscribe: (listener: () => void) => () => void;
  /**
   * Opens the microphone, processed as processing says (the welcome's tuning), and attaches this
   * device for voice: from a person's gesture.
   */
  readonly start: (processing: MicrophoneProcessing) => Promise<void>;
  /** Ends every call, detaches, and closes the microphone. */
  readonly stop: () => void;
  readonly setMuted: (isMuted: boolean) => void;
  readonly setPartnerGain: (gain: number) => void;
}

/** What voice needs. */
export interface TogetherVoiceOptions<Microphone extends VoiceMicrophone> {
  /** The membership's credential, by which a second device attaches for voice (D120). */
  readonly credential: string;
  readonly sessionToken?: string | undefined;
  readonly openSocket: OpenTogetherSocket;
  readonly monotonicMs: () => number;
  /** The relay's ICE servers for this membership (GET /together/{id}/ice). */
  readonly fetchIceServers: () => Promise<readonly RTCIceServer[]>;
  readonly devices: VoiceDevices<Microphone>;
}

/**
 * The most the others' voices are made louder than they came: a microphone with no gain control
 * (no processing at all) came through too quiet beside the film even at 1 (the owner's
 * test, 2026-10-06).
 */
export const partnerGainMost = 4;

const offSnapshot: VoiceSnapshot = {
  status: "off",
  failure: undefined,
  browserReason: undefined,
  isMuted: false,
  partnerGain: 1,
  isConnected: false,
  partners: [],
};

/**
 * How long the relay's ICE servers are used before a new call asks for them again: half of their
 * credentials' day (D243 §8), so a call made late in a long session is not refused by the relay
 * (M4's audit).
 */
export const iceServersFreshForMs = 12 * 60 * 60 * 1000;

/** The roles of devices that carry a person's voice (D120). */
const voiceRoles: ReadonlySet<string> = new Set(["voice", "both"]);

/** Every other person's voice device in roster: none of this member's own. */
function partnersIn(
  roster: readonly RosterMember[],
  ownMemberId: number,
): { readonly deviceId: number; readonly name: string }[] {
  return roster
    .filter((member) => member.id !== ownMemberId)
    .flatMap((member) =>
      member.devices
        .filter((device) => voiceRoles.has(device.role))
        .map((device) => ({ deviceId: device.id, name: member.name })),
    );
}

/** A call this device keeps with another, and the work done on it, one step after another. */
class CallEntry {
  readonly deviceId: number;
  readonly name: string;
  /** Whether this device makes the offer: the lower device id does, so both never offer at once. */
  readonly isOffering: boolean;
  #state: VoiceCallState = "connecting";
  call: VoiceCall | undefined;
  isClosed = false;
  readonly #onFailed: () => void;
  #work: Promise<void> = Promise.resolve();

  constructor(
    partner: { readonly deviceId: number; readonly name: string },
    isOffering: boolean,
    onFailed: () => void,
  ) {
    this.deviceId = partner.deviceId;
    this.name = partner.name;
    this.isOffering = isOffering;
    this.#onFailed = onFailed;
  }

  /**
   * Runs step once every step before it is done, unless the call is closed by then: so an offer
   * is answered before the candidates after it are added. A step that fails fails the call.
   */
  run(step: () => Promise<void>): void {
    this.#work = this.#work
      .then(async () => {
        if (!this.isClosed) {
          await step();
        }
      })
      .catch(() => {
        this.#onFailed();
      });
  }

  close(): void {
    this.isClosed = true;
    this.call?.close();
  }

  setState(state: VoiceCallState): void {
    this.#state = state;
  }

  partner(): VoicePartner {
    return { deviceId: this.deviceId, name: this.name, state: this.#state };
  }
}

function stopTracks(microphone: VoiceMicrophone): void {
  for (const track of microphone.getAudioTracks()) {
    track.stop();
  }
}

class VoiceSession<Microphone extends VoiceMicrophone> implements TogetherVoice {
  readonly #options: TogetherVoiceOptions<Microphone>;
  readonly #listeners = new Set<() => void>();
  readonly #calls = new Map<number, CallEntry>();
  #snapshot = offSnapshot;
  /** Which start is the current one: a microphone that opens after a stop is closed at once. */
  #generation = 0;
  #microphone: Microphone | undefined;
  #connection: (TogetherConnection & RelaysVoiceSignals) | undefined;
  #unsubscribes: (() => void)[] = [];
  /** This device's id on the voice connection now: a new one on each welcome. */
  #ownDeviceId: number | undefined;
  #iceServers: Promise<readonly RTCIceServer[]> = Promise.resolve([]);
  /** When the ICE servers were last asked for, on the monotonic clock. */
  #iceServersAskedAtMs = 0;
  #lastIceServers: readonly RTCIceServer[] = [];
  #measuring: ReturnType<typeof setInterval> | undefined;

  constructor(options: TogetherVoiceOptions<Microphone>) {
    this.#options = options;
  }

  readonly snapshot = (): VoiceSnapshot => this.#snapshot;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  readonly start = async (processing: MicrophoneProcessing): Promise<void> => {
    if (this.#snapshot.status === "starting" || this.#snapshot.status === "on") {
      return;
    }
    this.#generation += 1;
    const generation = this.#generation;
    this.#change({ ...offSnapshot, status: "starting", partnerGain: this.#snapshot.partnerGain });
    let microphone: Microphone;
    try {
      microphone = await this.#options.devices.openMicrophone(processing);
    } catch (error) {
      if (generation === this.#generation) {
        this.#change({
          ...this.#snapshot,
          status: "failed",
          failure: microphoneFailureOf(error),
          browserReason: browserReasonOf(error),
        });
      }
      return;
    }
    if (generation !== this.#generation) {
      stopTracks(microphone);
      return;
    }
    this.#microphone = microphone;
    this.#change({ ...this.#snapshot, status: "on" });
    this.#attach();
  };

  readonly stop = (): void => {
    this.#generation += 1;
    this.#closeCalls();
    for (const unsubscribe of this.#unsubscribes) {
      unsubscribe();
    }
    this.#unsubscribes = [];
    clearInterval(this.#measuring);
    this.#measuring = undefined;
    this.#connection?.close();
    this.#connection = undefined;
    this.#ownDeviceId = undefined;
    if (this.#microphone !== undefined) {
      stopTracks(this.#microphone);
    }
    this.#microphone = undefined;
    this.#change({ ...offSnapshot, partnerGain: this.#snapshot.partnerGain });
  };

  readonly setMuted = (isMuted: boolean): void => {
    for (const track of this.#microphone?.getAudioTracks() ?? []) {
      track.enabled = !isMuted;
    }
    this.#change({ ...this.#snapshot, isMuted });
  };

  readonly setPartnerGain = (gain: number): void => {
    const partnerGain = Math.min(partnerGainMost, Math.max(0, gain));
    for (const entry of this.#calls.values()) {
      entry.call?.setGain(partnerGain);
    }
    this.#change({ ...this.#snapshot, partnerGain });
  };

  #change(next: VoiceSnapshot): void {
    this.#snapshot = next;
    setVoiceLive(
      next.status === "on" && next.partners.some((partner) => partner.state === "connected"),
    );
    for (const listener of this.#listeners) {
      listener();
    }
  }

  #partners(): VoicePartner[] {
    return [...this.#calls.values()].map((entry) => entry.partner());
  }

  /** Attaches a second device of this membership, for voice alone (D120). */
  #attach(): void {
    const connection = createTogetherConnection({
      credential: this.#options.credential,
      sessionToken: this.#options.sessionToken,
      role: "voice",
      openSocket: this.#options.openSocket,
      monotonicMs: this.#options.monotonicMs,
    });
    this.#connection = connection;
    this.#unsubscribes = [
      connection.subscribe(() => {
        this.#follow(connection);
      }),
      connection.subscribeToVoiceSignals((fromDeviceId, signal) => {
        this.#hear(fromDeviceId, signal);
      }),
    ];
    connection.open();
    this.#measuring = setInterval(() => {
      this.#reportCalls(connection);
    }, voiceStatsEveryMs);
  }

  /** Tells the core's log how each connected call is going (D262 §1). */
  #reportCalls(connection: TogetherConnection): void {
    for (const entry of this.#calls.values()) {
      const { call } = entry;
      if (call === undefined || entry.partner().state !== "connected") {
        continue;
      }
      call.measure().then(
        (stats) => {
          connection.note({
            kind: "voiceStats",
            voice: { partnerDeviceId: entry.deviceId, ...stats },
          });
        },
        () => undefined, // a call that cannot say is said nothing of; its next measure may
      );
    }
  }

  /** Keeps a call with every other person's voice device the session's roster lists. */
  #follow(connection: TogetherConnection): void {
    const session = connection.snapshot();
    const isConnected = session.status === "connected";
    if (isConnected && session.deviceId !== this.#ownDeviceId) {
      // Welcomed anew, as a new device: the others end their calls with the old one, and so does
      // this one, and every call is made again.
      this.#closeCalls();
      this.#ownDeviceId = session.deviceId;
      this.#askForIceServers();
    }
    if (session.status === "ended") {
      this.#closeCalls();
    } else if (isConnected && session.memberId !== undefined) {
      this.#keepCallsWith(partnersIn(session.roster, session.memberId));
    }
    this.#change({ ...this.#snapshot, isConnected, partners: this.#partners() });
  }

  /** The relay's ICE servers, or the last ones given when asking fails. */
  #askForIceServers(): void {
    this.#iceServersAskedAtMs = this.#options.monotonicMs();
    this.#iceServers = this.#fetchIceServers();
  }

  async #fetchIceServers(): Promise<readonly RTCIceServer[]> {
    try {
      this.#lastIceServers = await this.#options.fetchIceServers();
    } catch {
      // The last ones still serve, as they last a day (D243 §8); with none, ICE tries directly.
    }
    return this.#lastIceServers;
  }

  #keepCallsWith(partners: readonly { readonly deviceId: number; readonly name: string }[]): void {
    const wanted = new Set(partners.map((partner) => partner.deviceId));
    for (const [deviceId, entry] of this.#calls) {
      if (!wanted.has(deviceId)) {
        entry.close();
        this.#calls.delete(deviceId);
      }
    }
    for (const partner of partners) {
      if (!this.#calls.has(partner.deviceId)) {
        this.#calls.set(partner.deviceId, this.#startCall(partner));
      }
    }
  }

  /** A call with partner, made once the ICE servers are known; the lower device id offers. */
  #startCall(partner: { readonly deviceId: number; readonly name: string }): CallEntry {
    const entry = new CallEntry(partner, (this.#ownDeviceId ?? 0) < partner.deviceId, () => {
      this.#setState(entry, "failed");
    });
    const microphone = this.#microphone;
    // A call made long after the last ask asks again: the relay's credentials last a day (D243 §8).
    if (this.#options.monotonicMs() - this.#iceServersAskedAtMs > iceServersFreshForMs) {
      this.#askForIceServers();
    }
    const iceServers = this.#iceServers;
    entry.run(async () => {
      const servers = await iceServers;
      if (entry.isClosed || microphone === undefined) {
        return;
      }
      const call = this.#options.devices.startCall({
        iceServers: servers,
        microphone,
        events: {
          onCandidate: (candidate) => {
            this.#send(entry, { candidate });
          },
          onState: (state) => {
            this.#setState(entry, state);
          },
        },
      });
      entry.call = call;
      call.setGain(this.#snapshot.partnerGain);
      if (entry.isOffering) {
        this.#send(entry, { sdp: await call.offer() });
        this.#noteStep(entry.deviceId, "offered");
      }
    });
    return entry;
  }

  /** Signalling from another device, taken in turn on its call; none from a device not called. */
  #hear(fromDeviceId: number, signal: VoiceSignal): void {
    const entry = this.#calls.get(fromDeviceId);
    if (entry === undefined) {
      this.#noteStep(fromDeviceId, "signalDropped");
      return;
    }
    entry.run(async () => {
      const call = entry.call;
      if (call === undefined) {
        return;
      }
      if ("candidate" in signal) {
        // A candidate that does not fit is one path fewer, never a failed call.
        await call.addCandidate(signal.candidate).catch(() => undefined);
      } else if (entry.isOffering) {
        await call.accept(signal.sdp);
        this.#noteStep(entry.deviceId, "accepted");
      } else {
        this.#send(entry, { sdp: await call.answer(signal.sdp) });
        this.#noteStep(entry.deviceId, "answered");
      }
    });
  }

  /** Tells the core's log the step a call with partnerDeviceId took. */
  #noteStep(partnerDeviceId: number, why: VoiceCallStep): void {
    this.#connection?.note({ kind: "voiceCall", partnerDeviceId, why });
  }

  #send(entry: CallEntry, signal: VoiceSignal): void {
    if (!entry.isClosed) {
      this.#connection?.sendVoiceSignal(entry.deviceId, signal);
    }
  }

  /** A call's new state; a call that failed turns voice off, saying so (the owner, 2026-10-06). */
  #setState(entry: CallEntry, state: VoiceCallState): void {
    if (entry.isClosed) {
      return;
    }
    if (state !== "connecting" && entry.partner().state !== state) {
      this.#noteStep(entry.deviceId, state);
    }
    if (state === "failed") {
      this.stop();
      this.#change({ ...this.#snapshot, status: "failed", failure: "unreachable" });
      return;
    }
    entry.setState(state);
    this.#change({ ...this.#snapshot, partners: this.#partners() });
  }

  #closeCalls(): void {
    for (const entry of this.#calls.values()) {
      entry.close();
    }
    this.#calls.clear();
  }
}

/**
 * A person's voice in their session (watch-together.md, Voice; D68, D193 §4): turned on, it opens
 * the microphone and attaches a second device of the membership with the voice role, then keeps
 * one audio call with every other person's voice device in the roster, the lower device id making
 * the offer, through the relay's ICE servers. A device leaving the roster ends its call; a new
 * welcome, after the voice socket connects again, makes every call again; a call that fails turns
 * voice off, saying so, and Talk tries again. Muting switches the
 * microphone's tracks off; the partners' gain sets how loud their voices play.
 */
export function createTogetherVoice<Microphone extends VoiceMicrophone>(
  options: TogetherVoiceOptions<Microphone>,
): TogetherVoice {
  return new VoiceSession(options);
}
