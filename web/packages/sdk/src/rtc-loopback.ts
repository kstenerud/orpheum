// SPDX-License-Identifier: MPL-2.0

import { waitMs } from "./wait-ms.ts";

/**
 * A WebRTC call of the page with itself, for the lab pages (plans/tv-device-tests.md): two peer
 * connections in one page, signalled by hand, with a data channel whose messages come back and an
 * audio track whose level is read on the receiving side. It shows whether a device runs WebRTC at
 * all (a Cast receiver, a TV's browser) and whether a phone keeps a call's sound flowing while it is
 * locked, with no server and no network beyond the device itself.
 */

/** A page's call with itself. */
export interface Loopback {
  /** What the receiving side gets: the track sent, as it arrived. */
  readonly remote: MediaStream;
  /** One message's round trip over the data channel, in milliseconds. */
  readonly ping: (timeoutMs?: number) => Promise<number>;
  /** The received audio's level now, 0 to 1, by the receiver's statistics; undefined unsaid. */
  readonly level: () => Promise<number | undefined>;
  /** Ends both sides. */
  readonly close: () => void;
}

/** What a loopback test found, as JSON the lab endpoint takes. */
export interface LoopbackResult {
  readonly connectMs: number | undefined;
  readonly roundTripsMs: readonly number[];
  /** The received level at each sample, or null where the statistics said none. */
  readonly levels: readonly (number | null)[];
  /** Why it failed, by the error's name and message; undefined when it did not. */
  readonly failure: string | undefined;
}

/** A level the statistics give, as a number 0 to 1, else undefined. */
function levelOf(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

/**
 * The received audio's level in a receiver's statistics: inbound-rtp's audioLevel, or where a browser
 * keeps it on the older track report, that one's; the highest where several say one.
 */
export function receivedLevelOf(
  reports: Iterable<Readonly<Record<string, unknown>>>,
): number | undefined {
  let highest: number | undefined;
  for (const report of reports) {
    const isAudio = report["kind"] === "audio" || report["mediaType"] === "audio";
    const isReceived =
      report["type"] === "inbound-rtp" ||
      (report["type"] === "track" && report["remoteSource"] === true);
    const level = isAudio && isReceived ? levelOf(report["audioLevel"]) : undefined;
    if (level !== undefined && (highest === undefined || level > highest)) {
      highest = level;
    }
  }
  return highest;
}

/** A failure's name and message, for a report; a thrown non-error is "failed". */
export function failureText(cause: unknown): string {
  return cause instanceof Error ? `${cause.name}: ${cause.message}` : "failed";
}

/** Waits for promise, or fails with a TimeoutError after timeoutMs. */
async function within<T>(promise: Promise<T>, timeoutMs: number, what: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      const error = new Error(`${what} took over ${String(timeoutMs)} ms`);
      error.name = "TimeoutError";
      reject(error);
    }, timeoutMs);
  });
  try {
    return await Promise.race([promise, timeout]);
  } finally {
    clearTimeout(timer);
  }
}

/** Hands each side's candidates to the other, as a signalling server would. */
function signalByHand(sending: RTCPeerConnection, receiving: RTCPeerConnection): void {
  for (const [from, to] of [
    [sending, receiving],
    [receiving, sending],
  ] as const) {
    from.addEventListener("icecandidate", (event) => {
      if (event.candidate !== null) {
        void to.addIceCandidate(event.candidate).catch(() => undefined);
      }
    });
  }
}

/** Offers from sending and answers from receiving, by hand. */
async function negotiate(sending: RTCPeerConnection, receiving: RTCPeerConnection): Promise<void> {
  const offer = await sending.createOffer();
  await sending.setLocalDescription(offer);
  await receiving.setRemoteDescription(offer);
  const answer = await receiving.createAnswer();
  await receiving.setLocalDescription(answer);
  await sending.setRemoteDescription(answer);
}

/** The round trip of a message over channel, whose other end sends each message back. */
function pingerOver(channel: RTCDataChannel): Loopback["ping"] {
  let sent = 0;
  return async (timeoutMs = 5000) => {
    sent += 1;
    const text = String(sent);
    const startedAt = performance.now();
    const back = new Promise<number>((resolve) => {
      const heard = (message: MessageEvent): void => {
        if (String(message.data) === text) {
          channel.removeEventListener("message", heard);
          resolve(performance.now() - startedAt);
        }
      };
      channel.addEventListener("message", heard);
    });
    channel.send(text);
    return await within(back, timeoutMs, "a message's round trip");
  };
}

/** The received level, read from receiving's statistics. */
function levelReaderOf(receiving: RTCPeerConnection): Loopback["level"] {
  return async () => {
    const stats = await receiving.getStats();
    const reports: Record<string, unknown>[] = [];
    stats.forEach((report: unknown) => {
      if (typeof report === "object" && report !== null) {
        reports.push({ ...report });
      }
    });
    return receivedLevelOf(reports);
  };
}

/** Opens a call of the page with itself carrying track, once its data channel is open. */
export async function openLoopback(track: MediaStreamTrack, timeoutMs = 10_000): Promise<Loopback> {
  const sending = new RTCPeerConnection();
  const receiving = new RTCPeerConnection();
  const close = (): void => {
    sending.close();
    receiving.close();
  };
  try {
    signalByHand(sending, receiving);
    const remote = new MediaStream();
    receiving.addEventListener("track", (event) => {
      remote.addTrack(event.track);
    });
    receiving.addEventListener("datachannel", (event) => {
      const echo = event.channel;
      echo.addEventListener("message", (message) => {
        echo.send(String(message.data));
      });
    });
    sending.addTrack(track, new MediaStream([track]));
    const channel = sending.createDataChannel("lab");
    const opened = new Promise<void>((resolve) => {
      channel.addEventListener("open", () => {
        resolve();
      });
    });
    await negotiate(sending, receiving);
    await within(opened, timeoutMs, "opening the data channel");
    return { remote, ping: pingerOver(channel), level: levelReaderOf(receiving), close };
  } catch (cause) {
    close();
    throw cause;
  }
}

/** A tone's track, made by context, which the page's call carries. Returns it and its stopping. */
export function toneTrack(
  context: AudioContext,
  frequencyHz = 440,
): { readonly track: MediaStreamTrack; readonly stop: () => void } {
  const oscillator = context.createOscillator();
  oscillator.frequency.value = frequencyHz;
  const destination = context.createMediaStreamDestination();
  oscillator.connect(destination);
  oscillator.start();
  const [track] = destination.stream.getAudioTracks();
  if (track === undefined) {
    throw new Error("the audio context made no track");
  }
  return {
    track,
    stop: () => {
      oscillator.stop();
      track.stop();
    },
  };
}

/**
 * Calls the page with itself carrying track, hands what arrives to playOut to play (a browser
 * decodes a received track only while something plays it), undefined once it ends, and measures: how long it took to connect, three
 * round trips over its data channel, and the received level each sampleEveryMs, samples times.
 */
export async function testLoopback(
  track: MediaStreamTrack,
  playOut: (received: MediaStream | undefined) => void,
  options: { readonly samples?: number; readonly sampleEveryMs?: number } = {},
): Promise<LoopbackResult> {
  const startedAt = performance.now();
  let loopback: Loopback | undefined;
  const roundTripsMs: number[] = [];
  const levels: (number | null)[] = [];
  try {
    loopback = await openLoopback(track);
    const connectMs = Math.round(performance.now() - startedAt);
    playOut(loopback.remote);
    for (let index = 0; index < 3; index += 1) {
      roundTripsMs.push(Math.round((await loopback.ping()) * 10) / 10);
    }
    for (let index = 0; index < (options.samples ?? 4); index += 1) {
      await waitMs(options.sampleEveryMs ?? 500);
      levels.push((await loopback.level()) ?? null);
    }
    return { connectMs, roundTripsMs, levels, failure: undefined };
  } catch (cause) {
    return { connectMs: undefined, roundTripsMs, levels, failure: failureText(cause) };
  } finally {
    loopback?.close();
    playOut(undefined);
  }
}
