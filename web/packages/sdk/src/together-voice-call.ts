// SPDX-License-Identifier: MPL-2.0

import type { VoiceCallStats } from "./together-voice-stats.ts";
import type { RosterMember } from "./together.ts";

/**
 * Why the microphone did not open: the person or the browser refused it, there is none, another
 * app holds it, this page cannot reach one at all (no https, or a browser without it), or anything
 * else.
 */
export type MicrophoneFailure = "refused" | "none" | "busy" | "unsupported" | "failed";

/**
 * Why talking stopped: its microphone did not open. A call that fails is made again, whatever its
 * failure (D281 §6).
 */
export type VoiceFailure = MicrophoneFailure;

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
export function browserReasonOf(error: unknown): string {
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

/**
 * One audio call with one other device (a browser's RTCPeerConnection): it hears the other from
 * the start, and sends the microphone only while its person talks (D281 §1, §2).
 */
export interface VoiceCall {
  /** Sends microphone on the call, or nothing when undefined: the call is not made again. */
  readonly send: (microphone: VoiceMicrophone | undefined) => void;
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
  /** The microphone it sends from the start, while its person talks; undefined while they do not. */
  readonly microphone: Microphone | undefined;
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
  /**
   * Readies the others' voices to play, before any microphone: the page's audio, which plays from
   * its person's next tap where the browser waits for one (D281 §1).
   */
  readonly prepareToHear: () => void;
  /** The open microphone's level now, a root-mean-square from 0 to 1; 0 with none open. */
  readonly readMicrophoneLevel: () => number;
  /**
   * The microphone stopped: what the devices did for it is undone, the microphone's levelling and,
   * on iOS, the call it made of the browser's sound (the owner's test, 2026-10-08).
   */
  readonly microphoneClosed: () => void;
}

/** One other person's voice device this one has a call with, and how it goes. */
export interface VoicePartner {
  readonly deviceId: number;
  /** Its member, by whom its volume is kept (D281 §4). */
  readonly memberId: number;
  /** The name its member chose. */
  readonly name: string;
  readonly state: VoiceCallState;
}

export function stopTracks(microphone: VoiceMicrophone): void {
  for (const track of microphone.getAudioTracks()) {
    track.stop();
  }
}

/** The roles of devices that carry a person's voice (D120). */
const voiceRoles: ReadonlySet<string> = new Set(["panel", "both"]);

/** A person's voice device: its id, and its member's id and name. */
export interface PartnerDevice {
  readonly deviceId: number;
  readonly memberId: number;
  readonly name: string;
}

/** Every other person's voice device in roster: none of this member's own. */
export function partnersIn(roster: readonly RosterMember[], ownMemberId: number): PartnerDevice[] {
  return roster
    .filter((member) => member.id !== ownMemberId)
    .flatMap((member) =>
      member.devices
        .filter((device) => voiceRoles.has(device.role))
        .map((device) => ({ deviceId: device.id, memberId: member.id, name: member.name })),
    );
}

/** A call this device keeps with another, and the work done on it, one step after another. */
export class CallEntry {
  readonly deviceId: number;
  readonly memberId: number;
  readonly name: string;
  /** Whether this device makes the offer: the lower device id does, so both never offer at once. */
  readonly isOffering: boolean;
  #state: VoiceCallState = "connecting";
  call: VoiceCall | undefined;
  isClosed = false;
  /** Whether it answered an offer: another offer after that is a new call, made afresh. */
  hasAnswered = false;
  /** Fails the call that has not connected in time (callConnectsWithinMs). */
  connectTimer: ReturnType<typeof setTimeout> | undefined;
  readonly #onFailed: () => void;
  #work: Promise<void> = Promise.resolve();

  constructor(partner: PartnerDevice, isOffering: boolean, onFailed: () => void) {
    this.deviceId = partner.deviceId;
    this.memberId = partner.memberId;
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
    clearTimeout(this.connectTimer);
    this.call?.close();
  }

  setState(state: VoiceCallState): void {
    this.#state = state;
  }

  partner(): VoicePartner {
    return {
      deviceId: this.deviceId,
      memberId: this.memberId,
      name: this.name,
      state: this.#state,
    };
  }
}
