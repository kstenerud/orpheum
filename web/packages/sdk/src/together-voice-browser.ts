// SPDX-License-Identifier: MPL-2.0

import { voiceCodecOrder, withOpusSettings } from "./together-voice-codec.ts";
import {
  levelMicrophone,
  meterMicrophone,
  mixToMono,
  type LevelledMicrophone,
} from "./together-voice-level.ts";
import {
  voiceCallStatsOf,
  type VoiceCallStats,
  type VoicePlayback,
} from "./together-voice-stats.ts";
import type {
  MicrophoneProcessing,
  VoiceMicrophone,
  VoiceCall,
  VoiceCallSetup,
  VoiceCallState,
  VoiceDevices,
} from "./together-voice.ts";

/**
 * The microphone as voice asks for it. Echo of the film is kept out by headphones (D63, tier 1),
 * so echo cancelling is rarely needed, and on Android it puts the phone in a call, which turns the
 * film down and takes its volume buttons (the owner's test, 2026-10-06). Gain control and noise
 * suppression apart from it are Chrome's own on Android, no call, but Firefox on Android still
 * makes a call of them (the owner's test, 2026-10-07); without gain control a voice is quiet.
 */
export function microphoneConstraints(processing: MicrophoneProcessing): MediaStreamConstraints {
  return {
    audio: {
      echoCancellation: processing.shouldCancelEcho,
      noiseSuppression: processing.shouldLevel,
      autoGainControl: processing.shouldLevel,
    },
    video: false,
  };
}

/** The browser's microphone; a page that cannot reach one (no https) fails as NotSupportedError. */
async function openBrowserMicrophone(processing: MicrophoneProcessing): Promise<MediaStream> {
  // Absent, not merely refused, outside a secure context: the http address on a home network.
  if (!("mediaDevices" in navigator)) {
    throw new DOMException("this page cannot reach a microphone", "NotSupportedError");
  }
  return await navigator.mediaDevices.getUserMedia(microphoneConstraints(processing));
}

/** A call's state, from its RTCPeerConnection's; a dropped path is one being set up again. */
function callStateOf(state: RTCPeerConnectionState): VoiceCallState {
  switch (state) {
    case "connected":
      return "connected";
    case "failed":
    case "closed":
      return "failed";
    case "new":
    case "connecting":
    case "disconnected":
      return "connecting";
  }
}

/** One of the other's ICE candidates, from the JSON text it sent. */
function candidateFrom(text: string): RTCIceCandidateInit {
  const parsed: unknown = JSON.parse(text);
  const field = (name: string): unknown =>
    typeof parsed === "object" && parsed !== null ? Reflect.get(parsed, name) : undefined;
  const candidate = field("candidate");
  const sdpMid = field("sdpMid");
  const sdpMLineIndex = field("sdpMLineIndex");
  const usernameFragment = field("usernameFragment");
  return {
    candidate: typeof candidate === "string" ? candidate : "",
    sdpMid: typeof sdpMid === "string" ? sdpMid : null,
    sdpMLineIndex: typeof sdpMLineIndex === "number" ? sdpMLineIndex : null,
    usernameFragment: typeof usernameFragment === "string" ? usernameFragment : null,
  };
}

/** Where the other's voice plays, at a gain the person sets, and how it plays now. */
interface PlayedVoice {
  readonly setGain: (gain: number) => void;
  readonly playback: () => VoicePlayback;
  readonly stop: () => void;
}

/** How a voice plays through context now: Web Audio while it runs, its element otherwise. */
export function playbackOf(context: Pick<AudioContext, "state"> | undefined): VoicePlayback {
  // Safari's "interrupted" is not in the type: iOS sets it as the microphone opens.
  const state: string | undefined = context?.state;
  switch (state) {
    case "running":
      return "webAudio";
    case "suspended":
      return "elementSuspended";
    case "interrupted":
      return "elementInterrupted";
    case "closed":
      return "elementClosed";
    case undefined:
    default:
      return "element";
  }
}

/**
 * Where a voice at gain plays as playback says: through Web Audio, at the gain, its element muted;
 * or through its element, no louder than it came, Web Audio silent so it is never heard twice.
 */
export function routeOf(
  playback: VoicePlayback,
  gain: number,
): {
  readonly isElementMuted: boolean;
  readonly elementVolume: number;
  readonly webAudioGain: number;
} {
  const isMixed = playback === "webAudio";
  return {
    isElementMuted: isMixed,
    elementVolume: Math.min(1, gain),
    webAudioGain: isMixed ? gain : 0,
  };
}

/**
 * Plays stream through a gain node of context, one per remote voice (watch-together.md: per-source
 * gain), while the context runs, and through its audio element while it does not, switching as the
 * context's state changes. iOS suspends or interrupts the context as the microphone opens, and a
 * voice that went only to Web Audio was then heard by no one: an iPhone and an Android phone
 * connected and neither heard the other (the owner's test, 2026-10-08). Chrome plays a remote
 * stream into Web Audio only while a media element holds it too, so a muted one does.
 */
function playVoice(stream: MediaStream, context: AudioContext | undefined): PlayedVoice {
  const element = new Audio();
  element.srcObject = stream;
  void element.play().catch(() => undefined);
  let gain = 1;
  let nodes: { readonly source: AudioNode; readonly gainNode: GainNode } | undefined;
  if (context !== undefined) {
    nodes = { source: context.createMediaStreamSource(stream), gainNode: context.createGain() };
    // One channel, which the destination plays in both ears: a voice sent as two, one of them
    // silent, reached one ear alone (the owner's test, 2026-10-08).
    mixToMono(nodes.gainNode);
    nodes.source.connect(nodes.gainNode).connect(context.destination);
  }
  const route = (): void => {
    const routed = routeOf(playbackOf(context), gain);
    element.muted = routed.isElementMuted;
    element.volume = routed.elementVolume;
    if (nodes !== undefined) {
      nodes.gainNode.gain.value = routed.webAudioGain;
    }
  };
  route();
  context?.addEventListener("statechange", route);
  return {
    setGain: (next) => {
      gain = next;
      route();
    },
    playback: () => playbackOf(context),
    stop: () => {
      context?.removeEventListener("statechange", route);
      nodes?.source.disconnect();
      nodes?.gainNode.disconnect();
      element.srcObject = null;
    },
  };
}

/**
 * Lists RED first among the codecs each of peer's audio transceivers takes (voiceCodecOrder), where
 * the browser lets a page choose them.
 */
function preferRed(peer: RTCPeerConnection): void {
  const codecs = RTCRtpReceiver.getCapabilities("audio")?.codecs;
  if (codecs === undefined) {
    return;
  }
  for (const transceiver of peer.getTransceivers()) {
    if ("setCodecPreferences" in transceiver) {
      transceiver.setCodecPreferences(voiceCodecOrder(codecs));
    }
  }
}

/**
 * Marks voice's packets high priority, and, where the browser can (Chrome, Safari), as expedited
 * for the network (DSCP EF), which a home router or Wi-Fi that honours it sends ahead of the film
 * (D262 §3). A browser that refuses the change sends voice as before.
 */
function markHighPriority(senders: readonly RTCRtpSender[]): void {
  for (const sender of senders) {
    const parameters = sender.getParameters();
    for (const encoding of parameters.encodings) {
      Object.assign(encoding, { priority: "high", networkPriority: "high" });
    }
    void sender.setParameters(parameters).catch(() => undefined);
  }
}

/** Takes a description peer made as its own, with voice's Opus settings, and returns what it took. */
async function takeOwn(peer: RTCPeerConnection, made: RTCSessionDescriptionInit): Promise<string> {
  const sdp = withOpusSettings(made.sdp ?? "");
  await peer.setLocalDescription({ type: made.type, sdp });
  return sdp;
}

/** The call as its receiver measured it so far, and how its sound plays (voiceCallStatsOf). */
async function measureCall(
  peer: RTCPeerConnection,
  playback: VoicePlayback,
): Promise<VoiceCallStats> {
  const reports: Record<string, unknown>[] = [];
  (await peer.getStats()).forEach((report: Record<string, unknown>) => {
    reports.push({ ...report });
  });
  return voiceCallStatsOf(reports, playback);
}

/** Sends microphone on sender, raised to a speaking level where the browser's levelling is off; or nothing. */
function sendOn(
  sender: RTCRtpSender,
  microphone: VoiceMicrophone | undefined,
  levelOf: (microphone: VoiceMicrophone) => LevelledMicrophone | undefined,
): void {
  const levelled = microphone === undefined ? undefined : levelOf(microphone);
  levelled?.follow(sender);
  const track = levelled?.sentTrack() ?? microphone?.getAudioTracks()[0] ?? null;
  void sender.replaceTrack(track).catch(() => undefined);
}

/** What a browser call is made with, beside its setup: where its voice plays and how it is sent. */
interface BrowserCallWays {
  readonly context: AudioContext | undefined;
  /** The microphone raised to a speaking level, where the browser's levelling is off. */
  readonly levelOf: (microphone: VoiceMicrophone) => LevelledMicrophone | undefined;
  /** The senders of every call, each while it lasts, whose statistics give the level sent. */
  readonly sending: Set<RTCRtpSender>;
}

/**
 * One call over the browser's RTCPeerConnection, playing what comes from the start and sending the
 * microphone while its person talks: one audio transceiver that sends and receives, whose sender's
 * track is put and taken away with no new offer (D281 §1, §2). The offerer adds its transceiver;
 * the answerer takes the one the offer made, as a transceiver added before an offer arrives is never
 * matched to it, and a microphone put on it went nowhere (the owner's test, 2026-10-11: an
 * iPhone's voice unheard).
 */
class BrowserCall implements VoiceCall {
  readonly #peer: RTCPeerConnection;
  readonly #ways: BrowserCallWays;
  #transceiver: RTCRtpTransceiver | undefined;
  #microphone: VoiceMicrophone | undefined;
  #gain = 1;
  #played: PlayedVoice | undefined;

  constructor(setup: VoiceCallSetup<MediaStream>, ways: BrowserCallWays) {
    this.#ways = ways;
    this.#microphone = setup.microphone;
    this.#peer = new RTCPeerConnection({
      iceServers: [...setup.iceServers],
      iceTransportPolicy: "all", // standard ICE picks the path: direct, or through the relay (D193 §4)
    });
    const peer = this.#peer;
    peer.addEventListener("icecandidate", (event) => {
      if (event.candidate !== null) {
        setup.events.onCandidate(JSON.stringify(event.candidate.toJSON()));
      }
    });
    peer.addEventListener("connectionstatechange", () => {
      if (peer.connectionState === "connected" && this.#transceiver !== undefined) {
        markHighPriority([this.#transceiver.sender]);
      }
      setup.events.onState(callStateOf(peer.connectionState));
    });
    peer.addEventListener("track", (event) => {
      this.#played?.stop();
      this.#played = playVoice(event.streams[0] ?? new MediaStream([event.track]), ways.context);
      this.#played.setGain(this.#gain);
    });
  }

  /** Takes transceiver as the call's own: it sends and receives, and sends the microphone if open. */
  #own(transceiver: RTCRtpTransceiver): void {
    this.#transceiver = transceiver;
    this.#ways.sending.add(transceiver.sender);
    Object.assign(transceiver, { direction: "sendrecv" });
    sendOn(transceiver.sender, this.#microphone, this.#ways.levelOf);
    preferRed(this.#peer);
  }

  readonly offer = async (): Promise<string> => {
    if (this.#transceiver === undefined) {
      this.#own(this.#peer.addTransceiver("audio", { direction: "sendrecv" }));
    }
    return await takeOwn(this.#peer, await this.#peer.createOffer());
  };

  readonly answer = async (offerSdp: string): Promise<string> => {
    await this.#peer.setRemoteDescription({ type: "offer", sdp: offerSdp });
    if (this.#transceiver === undefined) {
      this.#own(
        this.#peer.getTransceivers().find((each) => each.receiver.track.kind === "audio") ??
          this.#peer.addTransceiver("audio", { direction: "sendrecv" }),
      );
    }
    return await takeOwn(this.#peer, await this.#peer.createAnswer());
  };

  readonly accept = async (answerSdp: string): Promise<void> => {
    await this.#peer.setRemoteDescription({ type: "answer", sdp: answerSdp });
  };

  readonly addCandidate = async (candidate: string): Promise<void> => {
    await this.#peer.addIceCandidate(candidateFrom(candidate));
  };

  readonly setGain = (gain: number): void => {
    this.#gain = gain;
    this.#played?.setGain(gain);
  };

  readonly send = (microphone: VoiceMicrophone | undefined): void => {
    this.#microphone = microphone;
    if (this.#transceiver !== undefined) {
      sendOn(this.#transceiver.sender, microphone, this.#ways.levelOf);
    }
  };

  readonly measure = async (): Promise<VoiceCallStats> =>
    await measureCall(this.#peer, this.#played?.playback() ?? playbackOf(this.#ways.context));

  readonly close = (): void => {
    if (this.#transceiver !== undefined) {
      this.#ways.sending.delete(this.#transceiver.sender);
    }
    this.#played?.stop();
    this.#peer.close();
  };
}

/**
 * The level the calls send, from their statistics, for while iOS has interrupted the audio
 * context a microphone's meter reads through, as it does when the microphone opens (WebKit
 * 273511): read from the first call's sender every sentLevelEveryMs while the microphone is open.
 */
function sentLevelReader(sending: ReadonlySet<RTCRtpSender>): {
  readonly start: () => void;
  readonly stop: () => void;
  /** The level last read; undefined while no call's statistics have given one, as Firefox's do not. */
  readonly level: () => number | undefined;
} {
  let level: number | undefined;
  let reading: ReturnType<typeof setInterval> | undefined;
  const read = (): void => {
    const [sender] = sending;
    void sender
      ?.getStats()
      .then((reports) => {
        reports.forEach((report: Record<string, unknown>) => {
          const audioLevel = report["audioLevel"];
          if (report["type"] === "media-source" && typeof audioLevel === "number") {
            level = audioLevel;
          }
        });
      })
      .catch(() => undefined);
  };
  return {
    start: () => {
      clearInterval(reading);
      reading = setInterval(read, sentLevelEveryMs);
    },
    stop: () => {
      clearInterval(reading);
      reading = undefined;
      level = undefined;
    },
    level: () => level,
  };
}

/**
 * Resumes context at the person's next tap anywhere on page while it does not run: iOS suspends or
 * interrupts it as the microphone opens, and resumes it only from a gesture (WebKit 273511). Its
 * voices play through their elements meanwhile (playVoice).
 */
export function resumeAtTaps(
  context: {
    readonly state: string;
    readonly resume: () => Promise<void>;
    readonly addEventListener: (type: "statechange", listener: () => void) => void;
  },
  page: {
    readonly addEventListener: (type: "pointerdown", listener: () => void) => void;
    readonly removeEventListener: (type: "pointerdown", listener: () => void) => void;
  },
): void {
  const resume = (): void => {
    void context.resume().catch(() => undefined);
  };
  const follow = (): void => {
    if (context.state === "running" || context.state === "closed") {
      page.removeEventListener("pointerdown", resume);
    } else {
      page.addEventListener("pointerdown", resume);
    }
  };
  context.addEventListener("statechange", follow);
  follow();
}

/**
 * What the Talk button's ring reads of the microphone (D281 §7). Levelled here, what the levelling
 * measured and sends: a second analyser on the same microphone read zeros on Firefox for Android,
 * hosting, while the levelling's heard the voice, which the iPhone heard at 0.1 (the owner's test,
 * 2026-10-11). Levelled by the browser, an analyser of its own.
 */
function ringMeterOf(
  microphone: MediaStream,
  context: AudioContext | undefined,
  level: LevelledMicrophone | undefined,
): ReturnType<typeof meterMicrophone> {
  if (level !== undefined) {
    return { read: level.sentLevel, close: () => undefined };
  }
  return context === undefined ? undefined : meterMicrophone(microphone, context);
}

/**
 * The louder of the microphone's meter, while its audio context runs, and the level a call's
 * statistics give, while one is up: the meter reads zeros on iOS (the owner's test, 2026-10-11, the
 * Talk ring still while the call's statistics gave 0.173), whose audio context is also interrupted as
 * the microphone opens (WebKit 273511), and Firefox's statistics give no level, its meter reading the
 * voice.
 */
function louderLevel(
  context: AudioContext | undefined,
  meter: { readonly read: () => number } | undefined,
  sent: { readonly level: () => number | undefined } | undefined,
): number {
  const metered = context?.state === "running" && meter !== undefined ? meter.read() : 0;
  return Math.max(metered, sent?.level() ?? 0);
}

/** How often the sent level is read from a call's statistics while the microphone is open, in ms. */
const sentLevelEveryMs = 200;

/** The browser's devices for voice, and the audio context its voices play through. */
export interface BrowserVoiceDevices extends VoiceDevices<MediaStream> {
  /** Closes the audio context: once voice is over. */
  readonly close: () => void;
}

/**
 * The browser's microphone and RTCPeerConnection for voice, its voices played through one Web
 * Audio context. The context is made as voice is first heard, as the page opens, and resumed at
 * once where the browser lets it, else at the person's next tap (resumeAtTaps); its voices play
 * through their elements meanwhile (D281 §1).
 */
export function browserVoiceDevices(): BrowserVoiceDevices {
  let context: AudioContext | undefined;
  // The microphone open now, raised to a speaking level: one at a time, the last Talk's.
  let levelled:
    { readonly microphone: MediaStream; readonly level: LevelledMicrophone } | undefined;
  // The level the calls send, which the Talk button shows (D281 §7), and the open microphone's
  // meter, for when there is no call.
  let meter: ReturnType<typeof meterMicrophone>;
  const sending = new Set<RTCRtpSender>();
  const sent = sentLevelReader(sending);
  const prepareToHear = (): void => {
    if (context === undefined && "AudioContext" in window) {
      context = new AudioContext();
      resumeAtTaps(context, document);
      void context.resume().catch(() => undefined);
    }
  };
  return {
    prepareToHear,
    openMicrophone: async (processing) => {
      prepareToHear();
      startCallAudio();
      const microphone = await openBrowserMicrophone(processing);
      // iOS can pause the context as the microphone opens: it plays on once resumed. Never waited
      // for: Firefox keeps a resume pending until it lets the page play sound, and voice hung at
      // "Asking for your microphone" (the browser tier, 2026-10-06).
      void context?.resume().catch(() => undefined);
      levelled?.level.close();
      levelled = undefined;
      // Without the browser's levelling, which on Android makes a call of voice, a phone's microphone
      // is too quiet to hear: it is raised here instead (the owner's choice, 2026-10-08).
      const level =
        processing.shouldLevel || context === undefined
          ? undefined
          : levelMicrophone(microphone, context);
      levelled = level === undefined ? undefined : { microphone, level };
      meter?.close();
      meter = ringMeterOf(microphone, context, level);
      sent.start();
      return microphone;
    },
    readMicrophoneLevel: () => louderLevel(context, meter, sending.size > 0 ? sent : undefined),
    microphoneClosed: () => {
      levelled?.level.close();
      levelled = undefined;
      meter?.close();
      meter = undefined;
      sent.stop();
      endCallAudio();
    },
    startCall: (setup) =>
      new BrowserCall(setup, {
        context,
        levelOf: (microphone) => (levelled?.microphone === microphone ? levelled.level : undefined),
        sending,
      }),
    close: () => {
      levelled?.level.close();
      levelled = undefined;
      meter?.close();
      meter = undefined;
      sent.stop();
      void context?.close().catch(() => undefined);
      context = undefined;
    },
  };
}

/** What a browser's audio session can be told, where it has one (Safari 16.4 and later). */
type SessionAudio = "auto" | "playback" | "play-and-record";

/** Tells the browser's audio session what it plays, where it has one; elsewhere nothing. */
function setSessionAudio(type: SessionAudio): void {
  const audioSession: unknown = Reflect.get(navigator, "audioSession");
  if (typeof audioSession === "object" && audioSession !== null) {
    Reflect.set(audioSession, "type", type);
  }
}

/** Before the microphone opens: the session back to the browser's own choice. */
function startCallAudio(): void {
  setSessionAudio("auto");
}

/**
 * Once the microphone closes: the session told it plays only, so iOS leaves the call it made of voice at
 * once, the film's sound back on the speaker, rather than some 20 s later (the owner's test,
 * 2026-10-08; unconfirmed on a phone, VQ26).
 */
function endCallAudio(): void {
  setSessionAudio("playback");
}
