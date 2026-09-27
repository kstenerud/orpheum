// SPDX-License-Identifier: MPL-2.0

import { drawSubtitles, type SubtitleLayer } from "@orpheum/player";
import { textWhenReady, type Api, type components } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";

/** One text subtitle track a playback offers (D217 §6). */
export type SubtitleTrack = components["schemas"]["SubtitleTrackBody"];

/** How far one press of the offset buttons moves the subtitles, in milliseconds. */
export const offsetStepMs = 100;

/** A language's name in the viewer's language, or the tag itself when the browser has none. */
export function languageName(tag: string, viewerLanguage: string): string {
  try {
    return new Intl.DisplayNames([viewerLanguage], { type: "language" }).of(tag) ?? tag;
  } catch {
    return tag;
  }
}

/** A track's name in the menu: its language, its title, and whether it is forced or for the deaf. */
export function trackLabel(
  track: SubtitleTrack,
  viewerLanguage: string,
  words: { unknownLanguage: string; forced: string; hearingImpaired: string },
): string {
  const parts = [
    track.language === "" ? words.unknownLanguage : languageName(track.language, viewerLanguage),
  ];
  if (track.title !== "") {
    parts.push(track.title);
  }
  if (track.isForced) {
    parts.push(words.forced);
  }
  if (track.isHearingImpaired) {
    parts.push(words.hearingImpaired);
  }
  return parts.join(" · ");
}

/** An offset as the viewer reads it: signed seconds, to the tenth. */
export function formatOffsetSeconds(offsetMs: number): string {
  const seconds = (offsetMs / 1000).toFixed(1);
  return offsetMs > 0 ? `+${seconds}` : seconds;
}

/** Keeps a file's subtitle offset for the person, so the next playback starts with it. */
export async function keepOffset(
  api: Api,
  fileId: number,
  subtitleOffsetMs: number,
): Promise<void> {
  throwIfRefused(
    await api.PUT("/files/{id}/prefs", {
      params: { path: { id: fileId } },
      body: { subtitleOffsetMs },
    }),
  );
}

/** A person's choices for one file, as the server sends them. */
export type FilePrefs = components["schemas"]["FilePrefsBody"];

/**
 * Keeps a subtitle choice as the file's own: a track, or off when streamId is undefined. The
 * server carries it to the work's other files that have no choice of their own (D219).
 */
export async function keepSubtitleChoice(
  api: Api,
  fileId: number,
  streamId: number | undefined,
): Promise<void> {
  const subtitle = streamId === undefined ? { isOff: true } : { streamId };
  throwIfRefused(
    await api.PUT("/files/{id}/prefs", {
      params: { path: { id: fileId } },
      body: { subtitle },
    }),
  );
}

/** The track a playback starts with: the file's choice when it names one of its tracks, else none. */
export function startingTrack(
  prefs: FilePrefs,
  tracks: readonly SubtitleTrack[],
): number | undefined {
  const streamId = prefs.subtitle?.streamId;
  return tracks.some((track) => track.streamId === streamId) ? streamId : undefined;
}

/**
 * The subtitle menu and offset for a playing video: the chosen track is drawn over it by JASSUB.
 * It starts with the person's choices for this file, and keeps each change to them.
 */
export function Subtitles(props: {
  readonly fileId: number;
  readonly tracks: readonly SubtitleTrack[];
  readonly prefs: FilePrefs;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const [chosen, setChosen] = useState(() => startingTrack(props.prefs, props.tracks));
  const [offsetMs, setOffsetMs] = useState(props.prefs.subtitleOffsetMs);
  // The offset a newly chosen track starts with, read when its text arrives rather than in render.
  const offsetRef = useRef(props.prefs.subtitleOffsetMs);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const track = props.tracks.find((candidate) => candidate.streamId === chosen);
  const layerRef = useSubtitleLayer(track?.assUrl, props.videoRef, {
    offsetRef,
    failed: setFailure,
  });
  if (props.tracks.length === 0) {
    return null;
  }
  const move = (stepMs: number): void => {
    const moved = offsetMs + stepMs;
    setOffsetMs(moved);
    offsetRef.current = moved;
    layerRef.current?.setOffsetMs(moved);
    keepOffset(api, props.fileId, moved).catch((cause: unknown) => {
      setFailure(problemText(t, cause));
    });
  };
  return (
    <div className="play__subtitles">
      <TrackMenu
        tracks={props.tracks}
        chosen={chosen}
        choose={(streamId) => {
          setFailure(undefined);
          setChosen(streamId);
          keepSubtitleChoice(api, props.fileId, streamId).catch((cause: unknown) => {
            setFailure(problemText(t, cause));
          });
        }}
      />
      <OffsetControl offsetMs={offsetMs} move={move} />
      {failure === undefined ? null : (
        <p role="alert">{t("play.subtitles.failed", { reason: failure })}</p>
      )}
    </div>
  );
}

/** The menu of tracks, with "Off" first. */
function TrackMenu(props: {
  readonly tracks: readonly SubtitleTrack[];
  readonly chosen: number | undefined;
  readonly choose: (streamId: number | undefined) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const words = {
    unknownLanguage: t("play.subtitles.unknownLanguage"),
    forced: t("play.subtitles.forced"),
    hearingImpaired: t("play.subtitles.hearingImpaired"),
  };
  return (
    <label>
      {t("play.subtitles.label")}{" "}
      <select
        value={props.chosen === undefined ? "" : String(props.chosen)}
        onChange={(event) => {
          const value = event.currentTarget.value;
          props.choose(value === "" ? undefined : Number(value));
        }}
      >
        <option value="">{t("play.subtitles.off")}</option>
        {props.tracks.map((track) => (
          <option key={track.streamId} value={String(track.streamId)}>
            {trackLabel(track, i18n.language, words)}
          </option>
        ))}
      </select>
    </label>
  );
}

/** Moves the subtitles earlier or later by a tenth of a second, and shows where they are. */
function OffsetControl(props: {
  readonly offsetMs: number;
  readonly move: (stepMs: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <span className="play__offset">
      <button
        type="button"
        onClick={() => {
          props.move(-offsetStepMs);
        }}
      >
        {t("play.subtitles.earlier")}
      </button>
      <output>
        {t("play.subtitles.offset", { seconds: formatOffsetSeconds(props.offsetMs) })}
      </output>
      <button
        type="button"
        onClick={() => {
          props.move(offsetStepMs);
        }}
      >
        {t("play.subtitles.later")}
      </button>
    </span>
  );
}

/**
 * Draws the track at assUrl over the video once its text is extracted, and removes it when
 * another is chosen or the page closes. It starts with the offset current when it is drawn;
 * later changes go through the returned layer.
 */
function useSubtitleLayer(
  assUrl: string | undefined,
  videoRef: RefObject<HTMLVideoElement | null>,
  options: { offsetRef: RefObject<number>; failed: (why: string) => void },
): RefObject<SubtitleLayer | undefined> {
  const { t } = useTranslation();
  const layerRef = useRef<SubtitleLayer | undefined>(undefined);
  const { offsetRef, failed } = options;
  useEffect(() => {
    const video = videoRef.current;
    if (assUrl === undefined || video === null) {
      return undefined;
    }
    const abort = new AbortController();
    textWhenReady(assUrl, { signal: abort.signal }).then(
      (ass) => {
        if (!abort.signal.aborted) {
          layerRef.current = drawSubtitles(video, ass, offsetRef.current);
        }
      },
      (cause: unknown) => {
        if (!abort.signal.aborted) {
          failed(problemText(t, cause));
        }
      },
    );
    return () => {
      abort.abort();
      layerRef.current?.destroy();
      layerRef.current = undefined;
    };
  }, [assUrl, videoRef, offsetRef, failed, t]);
  return layerRef;
}
