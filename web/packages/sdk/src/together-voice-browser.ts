// SPDX-License-Identifier: MPL-2.0

import { voiceCodecOrder, withOpusSettings } from "./together-voice-codec.ts";
import { voiceCallStatsOf, type VoiceCallStats } from "./together-voice-stats.ts";
import type {
  MicrophoneProcessing,
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

/** Where the other's voice plays, at a gain the person sets. */
interface PlayedVoice {
  readonly setGain: (gain: number) => void;
  readonly stop: () => void;
}

/**
 * Plays stream through a gain node of context, one per remote voice (watch-together.md: per-source
 * gain), or through the audio element alone where Web Audio is not running. Chrome plays a remote
 * stream into Web Audio only while a media element holds it too, so a muted one does.
 */
function playVoice(stream: MediaStream, context: AudioContext | undefined): PlayedVoice {
  const element = new Audio();
  element.srcObject = stream;
  const isMixed = context?.state === "running";
  element.muted = isMixed;
  void element.play().catch(() => undefined);
  if (context === undefined || !isMixed) {
    return {
      setGain: (gain) => {
        element.volume = Math.min(1, gain); // an element plays no louder than it came
      },
      stop: () => {
        element.srcObject = null;
      },
    };
  }
  const source = context.createMediaStreamSource(stream);
  const gainNode = context.createGain();
  source.connect(gainNode).connect(context.destination);
  return {
    setGain: (gain) => {
      gainNode.gain.value = gain;
    },
    stop: () => {
      source.disconnect();
      gainNode.disconnect();
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

/** The call as its receiver measured it so far (voiceCallStatsOf). */
async function measureCall(peer: RTCPeerConnection): Promise<VoiceCallStats> {
  const reports: Record<string, unknown>[] = [];
  (await peer.getStats()).forEach((report: Record<string, unknown>) => {
    reports.push({ ...report });
  });
  return voiceCallStatsOf(reports);
}

/** One call over the browser's RTCPeerConnection, sending the microphone, playing what comes. */
function startBrowserCall(
  setup: VoiceCallSetup<MediaStream>,
  context: AudioContext | undefined,
): VoiceCall {
  const peer = new RTCPeerConnection({
    iceServers: [...setup.iceServers],
    iceTransportPolicy: "all", // standard ICE picks the path: direct, or through the relay (D193 §4)
  });
  const senders = setup.microphone
    .getAudioTracks()
    .map((track) => peer.addTrack(track, setup.microphone));
  preferRed(peer);
  let gain = 1;
  let played: PlayedVoice | undefined;
  peer.addEventListener("icecandidate", (event) => {
    if (event.candidate !== null) {
      setup.events.onCandidate(JSON.stringify(event.candidate.toJSON()));
    }
  });
  peer.addEventListener("connectionstatechange", () => {
    if (peer.connectionState === "connected") {
      markHighPriority(senders);
    }
    setup.events.onState(callStateOf(peer.connectionState));
  });
  peer.addEventListener("track", (event) => {
    played?.stop();
    played = playVoice(event.streams[0] ?? new MediaStream([event.track]), context);
    played.setGain(gain);
  });
  return {
    offer: async () => await takeOwn(peer, await peer.createOffer()),
    answer: async (offerSdp) => {
      await peer.setRemoteDescription({ type: "offer", sdp: offerSdp });
      return await takeOwn(peer, await peer.createAnswer());
    },
    accept: async (answerSdp) => {
      await peer.setRemoteDescription({ type: "answer", sdp: answerSdp });
    },
    addCandidate: async (candidate) => {
      await peer.addIceCandidate(candidateFrom(candidate));
    },
    setGain: (next) => {
      gain = next;
      played?.setGain(next);
    },
    measure: async () => await measureCall(peer),
    close: () => {
      played?.stop();
      peer.close();
    },
  };
}

/** The browser's devices for voice, and the audio context its voices play through. */
export interface BrowserVoiceDevices extends VoiceDevices<MediaStream> {
  /** Closes the audio context: once voice is over. */
  readonly close: () => void;
}

/**
 * The browser's microphone and RTCPeerConnection for voice, its voices played through one Web
 * Audio context. The context is made as the microphone is first asked for, which voice does from
 * a person's gesture (the Talk button): that is what lets it play.
 */
export function browserVoiceDevices(): BrowserVoiceDevices {
  let context: AudioContext | undefined;
  return {
    openMicrophone: async (processing) => {
      if (context === undefined && "AudioContext" in window) {
        context = new AudioContext();
        void context.resume().catch(() => undefined);
      }
      const microphone = await openBrowserMicrophone(processing);
      // iOS can pause the context as the microphone opens: it plays on once resumed. Never waited
      // for: Firefox keeps a resume pending until it lets the page play sound, and voice hung at
      // "Asking for your microphone" (the browser tier, 2026-10-06).
      void context?.resume().catch(() => undefined);
      return microphone;
    },
    startCall: (setup) => startBrowserCall(setup, context),
    close: () => {
      void context?.close().catch(() => undefined);
      context = undefined;
    },
  };
}
