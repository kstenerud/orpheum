// SPDX-License-Identifier: MPL-2.0

/**
 * The level a speaking voice is raised to, as a root-mean-square from 0 to 1: about what a phone's
 * microphone with the browser's gain control gives, and what the owner's voice arrived at from an
 * iPhone (0.12) where his own, from Android with the browser's levelling off, arrived at 0.003-0.007
 * and could not be heard (the owner's test, 2026-10-08).
 */
export const voiceTargetLevel = 0.1;

/**
 * The level below which the microphone hears no voice, only the room: a gain is never raised for
 * it, so silence is not pumped up into hiss. The owner's quiet Android voice was above it.
 */
export const voiceSpeechFloor = 0.002;

/** The most a voice is raised: 30 times, about 30 dB, enough for 0.003 to reach the target. */
export const voiceGainMost = 30;

/**
 * How fast the gain rises at each look, while someone speaks below the target: by at most this
 * factor, so 20 times takes about two seconds and a word does not jump; it falls at once, to just
 * above the target, when a voice comes in loud.
 */
export const voiceGainRise = 1.15;

/** How often the microphone's level is looked at, in ms. */
export const voiceLevelLookEveryMs = 100;

/**
 * The gain for the next look, from the gain now and the level the microphone gives (D262; the
 * owner's choice, 2026-10-08: the sender's own gain, the browser's processing off, since Android's
 * makes a call of voice): toward the target while someone speaks, rising slowly and falling at once,
 * between 1 and voiceGainMost; held while only the room is heard.
 */
export function nextVoiceGain(gain: number, level: number): number {
  if (level < voiceSpeechFloor) {
    return gain;
  }
  const wanted = Math.min(voiceGainMost, Math.max(1, voiceTargetLevel / level));
  return wanted < gain ? wanted : Math.min(wanted, gain * voiceGainRise);
}

/** The root-mean-square level of samples, 0 for none. */
export function levelOf(samples: Float32Array): number {
  if (samples.length === 0) {
    return 0;
  }
  let sum = 0;
  for (const sample of samples) {
    sum += sample * sample;
  }
  return Math.sqrt(sum / samples.length);
}

/** A microphone raised to a speaking level, and the track a call sends now. */
export interface LevelledMicrophone {
  /** The levelled track while Web Audio runs, the microphone's own while it does not. */
  readonly sentTrack: () => MediaStreamTrack;
  /** Has sender send sentTrack, switching as Web Audio stops and starts. */
  readonly follow: (sender: RTCRtpSender) => void;
  readonly close: () => void;
}

/**
 * Raises microphone through context: its source, a gain nextVoiceGain sets each look, and a limiter,
 * into a track of their own. A call sends that track while context runs, and the microphone's own
 * while it does not (iOS interrupts it as the microphone opens), so a stopped Web Audio never
 * silences anyone. Muting the microphone's track mutes both.
 */
export function levelMicrophone(
  microphone: MediaStream,
  context: AudioContext,
): LevelledMicrophone | undefined {
  const raw = microphone.getAudioTracks()[0];
  if (raw === undefined) {
    return undefined; // nothing to raise: the call sends what there is
  }
  const source = context.createMediaStreamSource(microphone);
  const analyser = context.createAnalyser();
  const gainNode = context.createGain();
  const limiter = context.createDynamicsCompressor();
  limiter.threshold.value = -6;
  limiter.knee.value = 0;
  limiter.ratio.value = 20;
  const destination = context.createMediaStreamDestination();
  source.connect(analyser);
  source.connect(gainNode).connect(limiter).connect(destination);
  const samples = new Float32Array(analyser.fftSize);
  const looking = setInterval(() => {
    analyser.getFloatTimeDomainData(samples);
    gainNode.gain.value = nextVoiceGain(gainNode.gain.value, levelOf(samples));
  }, voiceLevelLookEveryMs);
  const levelled = destination.stream.getAudioTracks()[0];
  const senders = new Set<RTCRtpSender>();
  const sentTrack = (): MediaStreamTrack =>
    context.state === "running" && levelled !== undefined ? levelled : raw;
  const switchTracks = (): void => {
    for (const sender of senders) {
      void sender.replaceTrack(sentTrack()).catch(() => undefined);
    }
  };
  context.addEventListener("statechange", switchTracks);
  return {
    sentTrack,
    follow: (sender) => {
      senders.add(sender);
    },
    close: () => {
      clearInterval(looking);
      context.removeEventListener("statechange", switchTracks);
      source.disconnect();
      gainNode.disconnect();
      limiter.disconnect();
      levelled?.stop();
      senders.clear();
    },
  };
}
