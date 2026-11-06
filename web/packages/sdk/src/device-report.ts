// SPDX-License-Identifier: MPL-2.0

/**
 * A device's capability report, for the lab pages of the web app and the cast receiver
 * (docs/contributors/design/plans/tv-device-tests.md): what the browser says of itself, what it
 * decodes in fMP4 through MSE and natively, and which of the APIs casting, extending, voice and a
 * remote rest on it has. It decides nothing and talks to no server: a page shows it and sends it
 * to the server's log through the API.
 */

/** How a page asks what it decodes: MSE's, ManagedMediaSource's and a media element's own answer. */
export interface CodecProbe {
  /** MediaSource.isTypeSupported; undefined where the page has no MediaSource. */
  readonly isTypeSupported: ((type: string) => boolean) | undefined;
  /** ManagedMediaSource.isTypeSupported (Safari); undefined where the page has none. */
  readonly managedIsTypeSupported: ((type: string) => boolean) | undefined;
  /** A video element's canPlayType. */
  readonly canPlayType: (type: string) => string;
}

/** One codec's answers: by MSE, by ManagedMediaSource, and by the element itself. */
export interface CodecAnswer {
  readonly name: string;
  readonly type: string;
  /** MSE's and ManagedMediaSource's answers; undefined where the page has no such source. */
  readonly isMseSupported: boolean | undefined;
  readonly isManagedSupported: boolean | undefined;
  readonly canPlayType: string;
}

/**
 * The codecs the lab asks about, each in fMP4 as our ladder carries it: H.264 at the profiles and
 * levels a TV's decoder is rated by, HEVC, Dolby Vision, AV1 and VP9 at 8 and 10 bits, and every
 * audio codec a film's own track may be in, DTS and TrueHD among them: whether a television plays a
 * film's surround track as it is (the owner's TCL TV, 2026-11-06).
 */
export const labCodecs: readonly (readonly [name: string, type: string])[] = [
  ["H.264 Baseline 3.0", 'video/mp4; codecs="avc1.42E01E"'],
  ["H.264 Main 3.1", 'video/mp4; codecs="avc1.4D401F"'],
  ["H.264 High 4.0", 'video/mp4; codecs="avc1.640028"'],
  ["H.264 High 4.2", 'video/mp4; codecs="avc1.64002A"'],
  ["H.264 High 5.1", 'video/mp4; codecs="avc1.640033"'],
  ["HEVC Main 3.1", 'video/mp4; codecs="hvc1.1.6.L93.B0"'],
  ["HEVC Main 4.0", 'video/mp4; codecs="hvc1.1.6.L120.90"'],
  ["HEVC Main 10 5.1", 'video/mp4; codecs="hvc1.2.4.L153.B0"'],
  ["Dolby Vision 5", 'video/mp4; codecs="dvh1.05.06"'],
  ["AV1 Main 8-bit", 'video/mp4; codecs="av01.0.08M.08"'],
  ["AV1 Main 10-bit", 'video/mp4; codecs="av01.0.08M.10"'],
  ["VP9 Profile 0", 'video/mp4; codecs="vp09.00.40.08"'],
  ["VP9 Profile 2 10-bit", 'video/mp4; codecs="vp09.02.40.10"'],
  ["AAC-LC", 'audio/mp4; codecs="mp4a.40.2"'],
  ["HE-AAC", 'audio/mp4; codecs="mp4a.40.5"'],
  ["HE-AAC v2", 'audio/mp4; codecs="mp4a.40.29"'],
  ["AC-3", 'audio/mp4; codecs="ac-3"'],
  ["E-AC-3", 'audio/mp4; codecs="ec-3"'],
  ["AC-4", 'audio/mp4; codecs="ac-4"'],
  ["DTS", 'audio/mp4; codecs="dtsc"'],
  ["DTS-HD", 'audio/mp4; codecs="dtsh"'],
  ["DTS:X", 'audio/mp4; codecs="dtsx"'],
  ["Dolby TrueHD", 'audio/mp4; codecs="mlpa"'],
  ["Opus", 'audio/mp4; codecs="opus"'],
  ["FLAC", 'audio/mp4; codecs="flac"'],
];

/** The types a media element plays HLS by itself under, as Safari and some TVs do. */
export const nativeHlsTypes: readonly string[] = [
  "application/vnd.apple.mpegurl",
  "application/x-mpegURL",
  "audio/mpegurl",
];

/** What a probe throws is no answer, never the report's end: an old WebView throws on odd types. */
function answerOf<T>(ask: () => T, otherwise: T): T {
  try {
    return ask();
  } catch {
    return otherwise;
  }
}

/** Every lab codec's answers. */
export function codecAnswersFrom(probe: CodecProbe): CodecAnswer[] {
  return labCodecs.map(([name, type]) => ({
    name,
    type,
    isMseSupported:
      probe.isTypeSupported === undefined
        ? undefined
        : answerOf(() => probe.isTypeSupported?.(type) === true, false),
    isManagedSupported:
      probe.managedIsTypeSupported === undefined
        ? undefined
        : answerOf(() => probe.managedIsTypeSupported?.(type) === true, false),
    canPlayType: answerOf(() => probe.canPlayType(type), ""),
  }));
}

/**
 * What a user agent says of a Fire TV: Fire OS is Android's, and names the stick by an "AFT" model
 * code; Vega OS is Linux's and is not Android (its earlier name was Kepler). A hint, never the
 * answer: the stick's own Settings, My Fire TV, About says (tv-device-tests.md). Undefined where the
 * agent names no Fire TV.
 */
export function fireTvHintOf(userAgent: string): string | undefined {
  const model = /\b(AFT[A-Z0-9]{1,8})\b/u.exec(userAgent)?.[1];
  if (/\b(?:Vega|Kepler)\b/u.test(userAgent)) {
    return model === undefined ? "Vega OS" : `Vega OS, model ${model}`;
  }
  if (model !== undefined) {
    return /\bAndroid\b/u.test(userAgent) ? `Fire OS, model ${model}` : `Fire TV, model ${model}`;
  }
  if (/\bSilk\//u.test(userAgent) && !/\bAndroid\b/u.test(userAgent)) {
    return "Silk not on Android: perhaps Vega OS";
  }
  return undefined;
}

/** The APIs the report says the page has or lacks, by name. */
export const labFeatureNames = [
  "mediaSource",
  "managedMediaSource",
  "setSinkId",
  "wakeLock",
  "getUserMedia",
  "rtcPeerConnection",
  "getScreenDetails",
  "getGamepads",
  "mediaSession",
  "fullscreen",
  "webkitFullscreen",
  "presentationRequest",
  "remotePlayback",
  "broadcastChannel",
  "audioContext",
  "requestVideoFrameCallback",
] as const;

/** One of the APIs the report names. */
export type LabFeature = (typeof labFeatureNames)[number];

/**
 * What the page's media queries say of its screen: whether it shows high dynamic range, for the page
 * and for video (a television's video plane may while its page does not), and its widest colour
 * gamut; each undefined where the browser answers neither value, as one without the query does.
 */
export interface DisplayFacts {
  readonly dynamicRange: "high" | "standard" | undefined;
  readonly videoDynamicRange: "high" | "standard" | undefined;
  readonly colorGamut: "rec2020" | "p3" | "srgb" | undefined;
}

/** What the report is assembled from, read off the page by browserDeviceFacts. */
export interface DeviceFacts {
  readonly userAgent: string;
  readonly screen: { readonly width: number; readonly height: number; readonly colorDepth: number };
  readonly viewport: { readonly width: number; readonly height: number };
  /** The window's own size, its toolbar and frame included: a full-screen window's is the screen's. */
  readonly window?: { readonly width: number; readonly height: number };
  /** Where the window stands on the screen, and the screen's area a window may take (its menu bar's out). */
  readonly placement?: {
    readonly screenX: number;
    readonly screenY: number;
    readonly availWidth: number;
    readonly availHeight: number;
    readonly availTop: number;
  };
  readonly devicePixelRatio: number;
  readonly display?: DisplayFacts;
  readonly features: Readonly<Record<LabFeature, boolean>>;
  readonly codecs: CodecProbe;
}

/** A device's capability report, as JSON the lab endpoint takes. */
export interface DeviceReport {
  readonly userAgent: string;
  readonly fireTv: string | undefined;
  readonly screen: { readonly width: number; readonly height: number; readonly colorDepth: number };
  readonly viewport: { readonly width: number; readonly height: number };
  /** The window's own size, its toolbar and frame included: a full-screen window's is the screen's. */
  readonly window?: { readonly width: number; readonly height: number };
  /** Where the window stands on the screen, and the screen's area a window may take (its menu bar's out). */
  readonly placement?: {
    readonly screenX: number;
    readonly screenY: number;
    readonly availWidth: number;
    readonly availHeight: number;
    readonly availTop: number;
  };
  readonly devicePixelRatio: number;
  readonly display?: DisplayFacts;
  /** The names of the APIs the page has, then those it lacks. */
  readonly has: readonly LabFeature[];
  readonly lacks: readonly LabFeature[];
  readonly codecs: readonly CodecAnswer[];
  /** The native HLS types the element answers anything but "" for, with its answer. */
  readonly nativeHls: Readonly<Record<string, string>>;
}

/** The report of a device, from its facts. */
export function deviceReportFrom(facts: DeviceFacts): DeviceReport {
  const nativeHls: Record<string, string> = {};
  for (const type of nativeHlsTypes) {
    const answer = answerOf(() => facts.codecs.canPlayType(type), "");
    if (answer !== "") {
      nativeHls[type] = answer;
    }
  }
  return {
    userAgent: facts.userAgent,
    fireTv: fireTvHintOf(facts.userAgent),
    screen: facts.screen,
    viewport: facts.viewport,
    ...(facts.window === undefined ? {} : { window: facts.window }),
    ...(facts.placement === undefined ? {} : { placement: facts.placement }),
    devicePixelRatio: facts.devicePixelRatio,
    ...(facts.display === undefined ? {} : { display: facts.display }),
    has: labFeatureNames.filter((name) => facts.features[name]),
    lacks: labFeatureNames.filter((name) => !facts.features[name]),
    codecs: codecAnswersFrom(facts.codecs),
    nativeHls,
  };
}

/** Whether holder has a property named name, own or inherited, without touching its value. */
function hasProperty(holder: unknown, name: string): boolean {
  return typeof holder === "object" && holder !== null && name in holder;
}

/** The isTypeSupported of a source constructor the page has by name, else undefined. */
function isTypeSupportedOf(name: string): ((type: string) => boolean) | undefined {
  const source: unknown = Reflect.get(globalThis, name);
  if (typeof source !== "function") {
    return undefined;
  }
  const ask: unknown = Reflect.get(source, "isTypeSupported");
  return typeof ask === "function"
    ? (type: string): boolean => Reflect.apply(ask, source, [type]) === true
    : undefined;
}

/** The first of values the media query feature matches, as (feature: value); undefined for none. */
export function mediaQueryAnswer<T extends string>(
  matches: (query: string) => boolean,
  feature: string,
  values: readonly T[],
): T | undefined {
  return values.find((value) => answerOf(() => matches(`(${feature}: ${value})`), false));
}

/** What the page's media queries say of its screen. */
function displayFacts(): DisplayFacts {
  const matches = (query: string): boolean => globalThis.matchMedia(query).matches;
  return {
    dynamicRange: mediaQueryAnswer(matches, "dynamic-range", ["high", "standard"] as const),
    videoDynamicRange: mediaQueryAnswer(matches, "video-dynamic-range", [
      "high",
      "standard",
    ] as const),
    colorGamut: mediaQueryAnswer(matches, "color-gamut", ["rec2020", "p3", "srgb"] as const),
  };
}

/** The facts of the page this runs in. */
export function browserDeviceFacts(): DeviceFacts {
  const video = document.createElement("video");
  const media = HTMLMediaElement.prototype;
  const features: Record<LabFeature, boolean> = {
    mediaSource: "MediaSource" in globalThis,
    managedMediaSource: "ManagedMediaSource" in globalThis,
    setSinkId: hasProperty(media, "setSinkId"),
    wakeLock: hasProperty(navigator, "wakeLock"),
    getUserMedia: hasProperty(navigator.mediaDevices, "getUserMedia"),
    rtcPeerConnection: "RTCPeerConnection" in globalThis,
    getScreenDetails: "getScreenDetails" in globalThis,
    getGamepads: hasProperty(navigator, "getGamepads"),
    mediaSession: hasProperty(navigator, "mediaSession"),
    fullscreen: hasProperty(Element.prototype, "requestFullscreen"),
    webkitFullscreen: hasProperty(HTMLVideoElement.prototype, "webkitEnterFullscreen"),
    presentationRequest: "PresentationRequest" in globalThis,
    remotePlayback: hasProperty(media, "remote"),
    broadcastChannel: "BroadcastChannel" in globalThis,
    audioContext: "AudioContext" in globalThis,
    requestVideoFrameCallback: hasProperty(HTMLVideoElement.prototype, "requestVideoFrameCallback"),
  };
  return {
    userAgent: navigator.userAgent,
    screen: {
      width: globalThis.screen.width,
      height: globalThis.screen.height,
      colorDepth: globalThis.screen.colorDepth,
    },
    viewport: { width: globalThis.innerWidth, height: globalThis.innerHeight },
    window: { width: globalThis.outerWidth, height: globalThis.outerHeight },
    placement: {
      screenX: globalThis.screenX,
      screenY: globalThis.screenY,
      availWidth: globalThis.screen.availWidth,
      availHeight: globalThis.screen.availHeight,
      availTop: Number(Reflect.get(globalThis.screen, "availTop") ?? 0),
    },
    devicePixelRatio: globalThis.devicePixelRatio,
    display: displayFacts(),
    features,
    codecs: {
      isTypeSupported: isTypeSupportedOf("MediaSource"),
      managedIsTypeSupported: isTypeSupportedOf("ManagedMediaSource"),
      canPlayType: (type) => video.canPlayType(type),
    },
  };
}

/** What a page's audio context says of its delay, in milliseconds; undefined where it says nothing. */
export interface AudioLatency {
  readonly baseLatencyMs: number | undefined;
  readonly outputLatencyMs: number | undefined;
  readonly sampleRate: number | undefined;
  readonly state: string;
}

/** A latency the context gives in seconds, in whole milliseconds, or undefined. */
function inMs(seconds: unknown): number | undefined {
  return typeof seconds === "number" && Number.isFinite(seconds)
    ? Math.round(seconds * 1000)
    : undefined;
}

/** The page's audio context constructor, where it has one. */
export function audioContextClass(): typeof AudioContext | undefined {
  return "AudioContext" in globalThis ? AudioContext : undefined;
}

/**
 * The audio context's own word on its delay, after the press a context needs to run; outputLatency
 * is read after a moment's running, since a context says 0 until its output has started.
 */
export async function measureAudioLatency(settleMs = 500): Promise<AudioLatency | undefined> {
  const Context = audioContextClass();
  if (Context === undefined) {
    return undefined;
  }
  const context = new Context();
  try {
    await context.resume();
    await new Promise((resolve) => {
      setTimeout(resolve, settleMs);
    });
    return {
      baseLatencyMs: inMs(Reflect.get(context, "baseLatency")),
      outputLatencyMs: inMs(Reflect.get(context, "outputLatency")),
      sampleRate: context.sampleRate,
      state: context.state,
    };
  } finally {
    await context.close().catch(() => undefined);
  }
}

/** How many audio outputs the page can name, or why it cannot count them. */
export async function countAudioOutputs(): Promise<number | string> {
  if (!("mediaDevices" in navigator) || !hasProperty(navigator.mediaDevices, "enumerateDevices")) {
    return "no enumerateDevices";
  }
  try {
    const listed = await navigator.mediaDevices.enumerateDevices();
    return listed.filter((device) => device.kind === "audiooutput").length;
  } catch (cause) {
    return cause instanceof Error ? cause.name : "failed";
  }
}
