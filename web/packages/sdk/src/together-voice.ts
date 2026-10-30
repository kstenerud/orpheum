// SPDX-License-Identifier: MPL-2.0

import { setVoiceLive } from "./voice-live.ts";
import { voiceFirstStatsAfterMs, voiceStatsEveryMs } from "./together-voice-stats.ts";
import {
  CallEntry,
  browserReasonOf,
  microphoneFailureOf,
  partnersIn,
  stopTracks,
  type MicrophoneProcessing,
  type PartnerDevice,
  type VoiceCallState,
  type VoiceDevices,
  type VoiceFailure,
  type VoiceMicrophone,
  type VoicePartner,
} from "./together-voice-call.ts";
import {
  createTogetherConnection,
  type RelaysVoiceSignals,
  type TogetherConnection,
  type TogetherConnectionOptions,
  type VoiceCallStep,
} from "./together-connection.ts";
import { startingTuning, type SyncTuning, type VoiceSignal } from "./together.ts";

/**
 * Whether this person talks: not, opening the microphone, talking, or stopped by a failure (the
 * snapshot's failure says which). They hear the others whatever it is, while isHearing (D281 §1).
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
  /** Whether this person hears the others: from the start, until voice fails or is left. */
  readonly isHearing: boolean;
  /** Whether the microphone, open, sends nothing: the others are heard as ever (D281 §3). */
  readonly isMuted: boolean;
  /**
   * How loud each other person's voice plays, by their member id, from 0 to partnerGainMost, 1 as
   * it came: apart from the film's volume, and 1 for one not set (D281 §4).
   */
  readonly partnerGains: Readonly<Record<number, number>>;
  /** Whether the voice device's own connection to the session is up. */
  readonly isConnected: boolean;
  /** Every other person's voice device, in the roster's order. */
  readonly partners: readonly VoicePartner[];
  /** How the session says the microphone is processed, as its welcome said (D250 §1). */
  readonly tuning: SyncTuning;
}

/** A person's voice in their Watch Together session (watch-together.md, Voice). */
export interface TogetherVoice {
  readonly snapshot: () => VoiceSnapshot;
  /** Calls listener whenever the snapshot changes; returns the unsubscribing. */
  readonly subscribe: (listener: () => void) => () => void;
  /**
   * Attaches this device for voice with no microphone, keeping a call with every other person's
   * voice device, hearing them (D281 §1); again after a failure, it tries again.
   */
  readonly hear: () => void;
  /**
   * Talk: opens the microphone, processed as processing says (the welcome's tuning), and sends it
   * on every call, hearing first if it was not: from a person's gesture (D281 §2).
   */
  readonly start: (processing: MicrophoneProcessing) => Promise<void>;
  /** Stop talking: closes the microphone and sends nothing, still hearing everyone (D281 §2). */
  readonly stop: () => void;
  /** Ends every call, detaches, and closes the microphone: the page is done with voice. */
  readonly leave: () => void;
  readonly setMuted: (isMuted: boolean) => void;
  /** How loud memberId's voice plays, from 0 to partnerGainMost (D281 §4). */
  readonly setPartnerGain: (memberId: number, gain: number) => void;
  /** This person's voice going out now, a root-mean-square from 0 to 1; 0 while not talking. */
  readonly readMicrophoneLevel: () => number;
}

/**
 * What voice needs: what its second device of the membership, attached for voice (D120), connects
 * by, its page's showing again checking its socket at once; its relay's ICE servers; its devices.
 */
export interface TogetherVoiceOptions<Microphone extends VoiceMicrophone> extends Pick<
  TogetherConnectionOptions,
  "credential" | "client" | "sessionToken" | "openSocket" | "monotonicMs" | "page"
> {
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
  isHearing: false,
  isMuted: false,
  partnerGains: {},
  isConnected: false,
  partners: [],
  tuning: startingTuning,
};

/**
 * How long the relay's ICE servers are used before a new call asks for them again: half of their
 * credentials' day (D243 §8), so a call made late in a long session is not refused by the relay
 * (M4's audit).
 */
export const iceServersFreshForMs = 12 * 60 * 60 * 1000;

/**
 * How long a failed call waits before it is made again, by how many times in a row it has failed;
 * the last wait for every one after (D281 §6). A call that connects starts the count again.
 */
export const callRetryWaitsMs: readonly number[] = [1000, 2000, 4000, 8000, 10_000];

/** How long a call may take to connect before it is failed and made again (D281 §6). */
export const callConnectsWithinMs = 15_000;

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
  /** How many times in a row each partner device's call has failed (D281 §6). */
  readonly #failures = new Map<number, number>();

  constructor(options: TogetherVoiceOptions<Microphone>) {
    this.#options = options;
  }

  readonly snapshot = (): VoiceSnapshot => this.#snapshot;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  readonly hear = (): void => {
    if (this.#connection !== undefined) {
      return;
    }
    this.#options.devices.prepareToHear();
    this.#change({
      ...this.#snapshot,
      isHearing: true,
      failure: undefined,
      browserReason: undefined,
    });
    this.#attach();
  };

  readonly start = async (processing: MicrophoneProcessing): Promise<void> => {
    if (this.#snapshot.status === "starting" || this.#snapshot.status === "on") {
      return;
    }
    this.hear();
    this.#generation += 1;
    const generation = this.#generation;
    this.#change({
      ...this.#snapshot,
      status: "starting",
      failure: undefined,
      browserReason: undefined,
    });
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
    for (const entry of this.#calls.values()) {
      entry.call?.send(microphone);
    }
    this.#connection?.note({ kind: "voiceTalk", why: "talking" });
    this.#change({ ...this.#snapshot, status: "on", isMuted: false });
  };

  readonly stop = (): void => {
    this.#generation += 1;
    if (this.#microphone !== undefined) {
      this.#connection?.note({ kind: "voiceTalk", why: "stopped" });
    }
    this.#closeMicrophone();
    this.#change({ ...this.#snapshot, status: "off", isMuted: false });
  };

  /** Closes the microphone, the calls sending nothing; the others are still heard. */
  #closeMicrophone(): void {
    if (this.#microphone === undefined) {
      return;
    }
    for (const entry of this.#calls.values()) {
      entry.call?.send(undefined);
    }
    stopTracks(this.#microphone);
    this.#microphone = undefined;
    this.#options.devices.microphoneClosed();
  }

  /** Turns voice off: the page is done with it, told to the core's log as left. */
  readonly leave = (): void => {
    this.#end();
  };

  /** Turns voice off, telling the core's log the person left. */
  #end(): void {
    this.#connection?.note({ kind: "voiceOff", why: "left" });
    this.#generation += 1;
    this.#closeMicrophone();
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
    this.#change({ ...offSnapshot, partnerGains: this.#snapshot.partnerGains });
  }

  readonly setMuted = (isMuted: boolean): void => {
    if (this.#microphone === undefined) {
      return;
    }
    for (const track of this.#microphone.getAudioTracks()) {
      track.enabled = !isMuted;
    }
    this.#connection?.note({ kind: "voiceTalk", why: isMuted ? "muted" : "unmuted" });
    this.#change({ ...this.#snapshot, isMuted });
  };

  readonly setPartnerGain = (memberId: number, gain: number): void => {
    const partnerGain = Math.min(partnerGainMost, Math.max(0, gain));
    for (const entry of this.#calls.values()) {
      if (entry.memberId === memberId) {
        entry.call?.setGain(partnerGain);
      }
    }
    this.#change({
      ...this.#snapshot,
      partnerGains: { ...this.#snapshot.partnerGains, [memberId]: partnerGain },
    });
  };

  readonly readMicrophoneLevel = (): number =>
    this.#microphone === undefined || this.#snapshot.isMuted
      ? 0
      : this.#options.devices.readMicrophoneLevel();

  /** How loud memberId's voice plays: as set, else as it came. */
  #gainOf(memberId: number): number {
    return this.#snapshot.partnerGains[memberId] ?? 1;
  }

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
      client: this.#options.client,
      sessionToken: this.#options.sessionToken,
      role: "panel",
      openSocket: this.#options.openSocket,
      monotonicMs: this.#options.monotonicMs,
      page: this.#options.page,
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
      this.#reportCall(connection, entry);
    }
  }

  /** Tells the core's log how one call is going, if it is connected. */
  #reportCall(connection: TogetherConnection, entry: CallEntry): void {
    const { call } = entry;
    if (call === undefined || entry.isClosed || entry.partner().state !== "connected") {
      return;
    }
    call.measure().then(
      (stats) => {
        connection.note({
          kind: "voiceStats",
          // What the Talk button's ring reads now, to say whether a still ring read nothing.
          voice: {
            partnerDeviceId: entry.deviceId,
            ...stats,
            ringLevel: this.readMicrophoneLevel(),
            partnerGain: this.#gainOf(entry.memberId),
          },
        });
      },
      () => undefined, // a call that cannot say is said nothing of; its next measure may
    );
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
    this.#change({
      ...this.#snapshot,
      isConnected,
      partners: this.#partners(),
      tuning: session.tuning,
    });
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

  #keepCallsWith(partners: readonly PartnerDevice[]): void {
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

  /**
   * A call with partner, made once the ICE servers are known, after waitMs when it is made again;
   * the lower device id offers. Signalling heard meanwhile waits its turn on it. One not connected
   * within callConnectsWithinMs is failed, and so made again (D281 §6).
   */
  #startCall(partner: PartnerDevice, waitMs = 0): CallEntry {
    const entry = new CallEntry(partner, (this.#ownDeviceId ?? 0) < partner.deviceId, () => {
      this.#setState(entry, "failed");
    });
    entry.connectTimer = setTimeout(() => {
      if (entry.partner().state !== "connected") {
        this.#setState(entry, "failed");
      }
    }, waitMs + callConnectsWithinMs);
    // A call made long after the last ask asks again: the relay's credentials last a day (D243 §8).
    if (this.#options.monotonicMs() - this.#iceServersAskedAtMs > iceServersFreshForMs) {
      this.#askForIceServers();
    }
    const iceServers = this.#iceServers;
    entry.run(async () => {
      if (waitMs > 0) {
        await new Promise((resolve) => setTimeout(resolve, waitMs));
      }
      const servers = await iceServers;
      if (entry.isClosed) {
        return;
      }
      const call = this.#options.devices.startCall({
        iceServers: servers,
        // Its person talking by the time the call is made sends from the start.
        microphone: this.#microphone,
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
      call.setGain(this.#gainOf(entry.memberId));
      if (entry.isOffering) {
        this.#send(entry, { sdp: await call.offer() });
        this.#noteStep(entry.deviceId, "offered");
      }
    });
    return entry;
  }

  /** Signalling from another device, taken in turn on its call; none from a device not called. */
  #hear(fromDeviceId: number, signal: VoiceSignal): void {
    let entry = this.#calls.get(fromDeviceId);
    if (entry === undefined) {
      this.#noteStep(fromDeviceId, "signalDropped");
      return;
    }
    if ("sdp" in signal && !entry.isOffering && entry.hasAnswered) {
      // The other made the call again, after a failure this side did not see: so does this one.
      entry.close();
      entry = this.#startCall(entry);
      this.#calls.set(fromDeviceId, entry);
    }
    const heard = entry;
    heard.run(async () => {
      const call = heard.call;
      if (call === undefined) {
        return;
      }
      if ("candidate" in signal) {
        // A candidate that does not fit is one path fewer, never a failed call.
        await call.addCandidate(signal.candidate).catch(() => undefined);
      } else if (heard.isOffering) {
        await call.accept(signal.sdp);
        this.#noteStep(heard.deviceId, "accepted");
      } else {
        heard.hasAnswered = true;
        this.#send(heard, { sdp: await call.answer(signal.sdp) });
        this.#noteStep(heard.deviceId, "answered");
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

  /**
   * A call's new state. A call that failed is made again after a wait that grows with each failure
   * in a row, its microphone sent on the new one as it was (D281 §6, the owner: "auto-reconnect
   * until the user turns off the voice button").
   */
  #setState(entry: CallEntry, state: VoiceCallState): void {
    if (entry.isClosed) {
      return;
    }
    if (state !== "connecting" && entry.partner().state !== state) {
      this.#noteStep(entry.deviceId, state);
      const connection = this.#connection;
      if (state === "connected" && connection !== undefined) {
        // The first measure soon after: a call that carries silence, or plays to no one, is said
        // before its people give up on it (the owner's test, 2026-10-08: calls of 8 s).
        setTimeout(() => {
          this.#reportCall(connection, entry);
        }, voiceFirstStatsAfterMs);
      }
    }
    if (state === "failed") {
      this.#makeAgain(entry);
      return;
    }
    if (state === "connected") {
      this.#failures.delete(entry.deviceId);
    }
    entry.setState(state);
    this.#change({ ...this.#snapshot, partners: this.#partners() });
  }

  /** Makes a failed call again, after its wait, in its place; it shows as connecting meanwhile. */
  #makeAgain(failed: CallEntry): void {
    failed.close();
    if (this.#calls.get(failed.deviceId) !== failed) {
      return; // already replaced, or its device gone
    }
    const failures = (this.#failures.get(failed.deviceId) ?? 0) + 1;
    this.#failures.set(failed.deviceId, failures);
    const waitMs = callRetryWaitsMs[Math.min(failures, callRetryWaitsMs.length) - 1] ?? 0;
    this.#calls.set(failed.deviceId, this.#startCall(failed, waitMs));
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
 * A person's voice in their session (watch-together.md, Voice; D68, D193 §4, D281): hearing, it
 * attaches a second device of the membership as its panel (D338 §4), no microphone, then keeps
 * one audio call with every other person's voice device in the roster, the lower device id making
 * the offer, through the relay's ICE servers. Talk opens the microphone and sends it on those calls,
 * and stopping closes it, neither making a call again. A device leaving the roster ends its call; a
 * new welcome, after the voice socket connects again, makes every call again; a call that fails,
 * or does not connect in time, is made again after a wait that grows to callRetryWaitsMs's last. Muting switches the microphone's tracks off;
 * each person's gain sets how loud their voice plays.
 */
export function createTogetherVoice<Microphone extends VoiceMicrophone>(
  options: TogetherVoiceOptions<Microphone>,
): TogetherVoice {
  return new VoiceSession(options);
}
