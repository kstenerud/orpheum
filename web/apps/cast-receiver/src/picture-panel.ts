// SPDX-License-Identifier: MPL-2.0

import {
  failureText,
  type components,
  type FetchRequest,
  type PictureChoice,
  type PlaybackPicture,
  type RemotesPanel,
  type SubtitleOption,
} from "@orpheum/sdk";

import type { PanelWays } from "./sound-panel.ts";

/** A subtitle track the core offers for the film. */
type SubtitleTrack = components["schemas"]["SubtitleTrackBody"];

/** What the core decides for a film the receiver plays, as far as its subtitles go; an older core's may lack parts. */
export type PictureDecision = Partial<
  Pick<components["schemas"]["StartPlaybackOutputBody"], "preferences" | "subtitles">
>;

/** How the picture panel reads a track's text: the receiver's way to its core, signed in. */
export interface SubtitleReach {
  readonly fetch: FetchRequest;
  readonly authorization: string;
  /** The language the receiver names tracks in: the Cast device's own. */
  readonly language: string;
}

/** A track's key, as the web app's subtitles.tsx keys it, so a remote's choice names the same. */
export function subtitleKeyOf(track: {
  readonly streamId?: number | undefined;
  readonly fetchedSubtitleId?: number | undefined;
}): string | undefined {
  if (track.fetchedSubtitleId !== undefined && track.fetchedSubtitleId !== 0) {
    return `f${String(track.fetchedSubtitleId)}`;
  }
  if (track.streamId !== undefined && track.streamId !== 0) {
    return `s${String(track.streamId)}`;
  }
  return undefined;
}

/**
 * A track's name on the remote: its language in the device's own, and its container's title or the
 * release a fetched one was made for; the receiver has no words of the app's to say forced or for the
 * deaf in.
 */
export function subtitleLabelOf(track: SubtitleTrack, language: string): string {
  let name = track.language;
  try {
    name = new Intl.DisplayNames([language], { type: "language" }).of(track.language) ?? name;
  } catch {
    // An empty or unknown code: named by its title alone.
  }
  const title = track.containerTitle ?? track.releaseName ?? "";
  const label = [name, title].filter((part) => part !== "").join(" · ");
  return label === "" ? "?" : label;
}

/** A WebVTT timestamp: hours optional. */
const vttTime = /(?:(\d+):)?(\d{2}):(\d{2})\.(\d{3})/g;

/** A WebVTT timestamp, hours and all, of at ms. */
function vttTimeOf(atMs: number): string {
  const pad = (n: number, width: number): string => String(n).padStart(width, "0");
  const hours = pad(Math.floor(atMs / 3_600_000), 2);
  const minutes = pad(Math.floor(atMs / 60_000) % 60, 2);
  const seconds = pad(Math.floor(atMs / 1000) % 60, 2);
  return `${hours}:${minutes}:${seconds}.${pad(atMs % 1000, 3)}`;
}

/** text, a WebVTT track, with every cue moved later by ms (earlier when negative), none before 0. */
export function shiftedVtt(text: string, ms: number): string {
  if (ms === 0) {
    return text;
  }
  const moved = (...parts: (string | undefined)[]): string => {
    // The match, then its hours (none where the cue has none), minutes, seconds and thousandths.
    const [h, m, s, f] = parts.slice(1, 5).map((part) => Number(part ?? 0));
    const atMs = (((h ?? 0) * 60 + (m ?? 0)) * 60 + (s ?? 0)) * 1000 + (f ?? 0);
    return vttTimeOf(Math.max(0, atMs + ms));
  };
  return text
    .split("\n")
    .map((line) => (line.includes("-->") ? line.replace(vttTime, moved) : line))
    .join("\n");
}

/**
 * The Chromecast's subtitles as its cast's remote works them (D434, D555 §3), as a browser receiver's
 * Subtitles menu is worked (the web app's picture-panel.ts): the tracks the core offers, the person's
 * choice of the file to start with, and their timing, each choice kept for the person by the sender
 * (the web app's cast-remote-keep.ts), as this guest receiver is refused the person's routes. A track shows as the video's own WebVTT track, which the Cast device draws itself:
 * JASSUB's ASS layer, the Play page's, is more than a 3rd-generation Chromecast should draw over a
 * film. The original is never offered: the receiver asks for no container of a file's own
 * (receiverFormats). A subtitle found on the remote shows with the next film the receiver is handed.
 */
export class ReceiverPicturePanel implements RemotesPanel<PictureChoice> {
  private readonly listeners = new Set<() => void>();
  private readonly tracks: readonly SubtitleTrack[];
  private readonly options: readonly SubtitleOption[];
  private key: string | undefined;
  private offsetMs: number;
  private shown: { readonly element: HTMLTrackElement; readonly url: string } | undefined;
  private text: string | undefined;
  /** Counts the tracks asked for, so a text read after another was chosen is dropped. */
  private asked = 0;

  constructor(
    decision: PictureDecision,
    private readonly ways: PanelWays,
    private readonly reach: SubtitleReach,
  ) {
    this.tracks = (decision.subtitles ?? []).filter((each) => subtitleKeyOf(each) !== undefined);
    this.options = this.tracks.map((each) => ({
      key: subtitleKeyOf(each) ?? "",
      label: subtitleLabelOf(each, reach.language),
    }));
    const choice = decision.preferences?.subtitleChoice;
    const chosen = choice === undefined ? undefined : subtitleKeyOf(choice);
    this.key = this.options.some((each) => each.key === chosen) ? chosen : undefined;
    this.offsetMs = decision.preferences?.subtitleOffsetMs ?? 0;
    this.show();
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  /** What the film shows that may be chosen, as its remote shows it; undefined with no track. */
  readonly read = (): PlaybackPicture | undefined =>
    this.options.length === 0
      ? undefined
      : {
          ...(this.key === undefined ? {} : { subtitleKey: this.key }),
          ...(this.offsetMs === 0 ? {} : { subtitleOffsetMs: this.offsetMs }),
          subtitles: this.options,
          canPlayOriginal: false,
        };

  readonly choose = (choice: PictureChoice): void => {
    if (choice.kind === "subtitles") {
      if (choice.key !== undefined && !this.options.some((each) => each.key === choice.key)) {
        return;
      }
      this.key = choice.key;
      this.text = undefined;
      this.show();
      this.told();
    } else if (choice.kind === "subtitleOffset") {
      this.offsetMs = choice.ms;
      this.draw();
      this.told();
    }
  };

  /** Takes the track shown off the video, the film ended or another started. */
  destroy(): void {
    this.asked += 1;
    this.takeOff();
  }

  /** Reads the chosen track's text and draws it, or takes the track off for none. */
  private show(): void {
    this.asked += 1;
    const asked = this.asked;
    const track = this.tracks.find((each) => subtitleKeyOf(each) === this.key);
    if (track === undefined) {
      this.takeOff();
      return;
    }
    const headers = new Headers();
    headers.set("Authorization", this.reach.authorization);
    const request = new Request(new URL(track.vttUrl, globalThis.location.origin).href, {
      headers,
    });
    void this.reach
      .fetch(request)
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`HTTP ${String(response.status)}`);
        }
        return await response.text();
      })
      .then((text) => {
        if (asked === this.asked) {
          this.text = text;
          this.draw();
        }
      })
      .catch((cause: unknown) => {
        this.ways.note(`could not read the subtitles: ${failureText(cause)}`);
      });
  }

  /** Shows the track read, at its timing, as the video's own text track. */
  private draw(): void {
    if (this.text === undefined) {
      return;
    }
    this.takeOff();
    const url = URL.createObjectURL(
      new Blob([shiftedVtt(this.text, this.offsetMs)], { type: "text/vtt" }),
    );
    const element = document.createElement("track");
    element.kind = "subtitles";
    element.src = url;
    element.default = true;
    this.ways.video.append(element);
    element.track.mode = "showing";
    this.shown = { element, url };
  }

  private takeOff(): void {
    if (this.shown === undefined) {
      return;
    }
    this.shown.element.remove();
    URL.revokeObjectURL(this.shown.url);
    this.shown = undefined;
  }

  private told(): void {
    for (const listener of this.listeners) {
      listener();
    }
  }
}
