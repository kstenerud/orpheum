// SPDX-License-Identifier: MPL-2.0

import "./subtitles.css";

import { drawSubtitles, type CaptionSource, type SubtitleLayer } from "@orpheum/player";
import { textWhenReady, type Api, type components } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { languageName } from "./language-name.ts";
import { Cluster } from "./layout.tsx";
import { signedTenthsText } from "./number-text.ts";
import { problemText, throwIfRefused } from "./problem.ts";
import { ActionChooser, currentLabelOf, type ActionChoice } from "./route-chooser.tsx";
import { useServices } from "./services.tsx";

/** One text subtitle track a playback offers (D217 §6). */
export type SubtitleTrack = components["schemas"]["SubtitleTrackBody"];

/** How far one press of the offset buttons moves the subtitles, in milliseconds. */
export const offsetStepMs = 100;

/** Names a track across both kinds: a stream of the file's ("s12"), or one fetched for it ("f3"). */
export type TrackKey = string;

/** The key of a track, or of a choice's track. */
export function trackKey(track: {
  readonly streamId?: number;
  readonly fetchedSubtitleId?: number;
}): TrackKey | undefined {
  if (track.fetchedSubtitleId !== undefined && track.fetchedSubtitleId !== 0) {
    return `f${String(track.fetchedSubtitleId)}`;
  }
  if (track.streamId !== undefined && track.streamId !== 0) {
    return `s${String(track.streamId)}`;
  }
  return undefined;
}

/** A track's name in the menu: its language, its container's title or the release a fetched one
 * was made for, whether it is forced or for the deaf, and for one fetched on request, that it is
 * unverified (D224). */
export function trackLabel(
  track: SubtitleTrack,
  viewerLanguage: string,
  words: { unknownLanguage: string; forced: string; hearingImpaired: string; fetched: string },
): string {
  const parts = [
    track.language === "" ? words.unknownLanguage : languageName(track.language, viewerLanguage),
  ];
  const title = track.containerTitle ?? track.releaseName;
  if (title !== undefined && title !== "") {
    parts.push(title);
  }
  if (track.isForced) {
    parts.push(words.forced);
  }
  if (track.isHearingImpaired) {
    parts.push(words.hearingImpaired);
  }
  if (track.fetchedSubtitleId !== undefined && track.fetchedSubtitleId !== 0) {
    parts.push(words.fetched);
  }
  return parts.join(" · ");
}

/** An offset as the viewer reads it: signed seconds, to the tenth, in their language. */
export function formatOffsetSeconds(offsetMs: number, viewerLanguage: string): string {
  return signedTenthsText(offsetMs / 1000, viewerLanguage);
}

/** Keeps a file's subtitle offset for the person, so the next playback starts with it. */
export async function keepOffset(
  api: Api,
  fileId: number,
  subtitleOffsetMs: number,
): Promise<void> {
  throwIfRefused(
    await api.PUT("/files/{id}/preferences", {
      params: { path: { id: fileId } },
      body: { subtitleOffsetMs },
    }),
  );
}

/** A person's choices for one file, as the server sends them. */
export type FilePreferences = components["schemas"]["PlaybackPreferencesBody"];

/**
 * Keeps a subtitle choice in a file: a track, or off when key is undefined. The file plays with it
 * again, and its language, or off, is the person's last choice in the file's work, which the work's
 * unwatched files start with (D230).
 */
export async function keepSubtitleChoice(
  api: Api,
  fileId: number,
  key: TrackKey | undefined,
): Promise<void> {
  const id = key === undefined ? 0 : Number(key.slice(1));
  let subtitle: { isOff: true } | { streamId: number } | { fetchedSubtitleId: number } = {
    isOff: true,
  };
  if (key?.startsWith("s") === true) {
    subtitle = { streamId: id };
  } else if (key?.startsWith("f") === true) {
    subtitle = { fetchedSubtitleId: id };
  }
  throwIfRefused(
    await api.PUT("/files/{id}/preferences", {
      params: { path: { id: fileId } },
      body: { subtitleChoice: subtitle },
    }),
  );
}

/** The track a playback starts with: the server's when it names one of the file's tracks, else none. */
export function startingTrackKey(
  preferences: FilePreferences,
  tracks: readonly SubtitleTrack[],
): TrackKey | undefined {
  const chosen =
    preferences.subtitleChoice === undefined ? undefined : trackKey(preferences.subtitleChoice);
  return tracks.some((track) => trackKey(track) === chosen) ? chosen : undefined;
}

/** What a playing video's subtitles are chosen from, and what they are drawn over. */
interface SubtitleChoiceWays {
  readonly fileId: number;
  readonly tracks: readonly SubtitleTrack[];
  readonly preferences: FilePreferences;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}

/** A playing video's subtitles: the tracks, the one chosen, the offset, and the ways to change them. */
export interface SubtitleChoice {
  readonly tracks: readonly SubtitleTrack[];
  readonly chosenTrackKey: TrackKey | undefined;
  readonly choose: (key: TrackKey | undefined) => void;
  readonly offsetMs: number;
  /** Moves the subtitles by stepMs, later for a positive step. */
  readonly shiftOffset: (stepMs: number) => void;
  /** What failed, in the viewer's words; undefined while nothing has. */
  readonly failure: string | undefined;
}

/**
 * The subtitle menu and offset for a playing video: the chosen track is drawn over it by JASSUB.
 * It starts with the person's choices for this file, and keeps each change to them.
 */
export function Subtitles(props: SubtitleChoiceWays): JSX.Element | null {
  return <SubtitleControls choice={useSubtitleChoice(props)} />;
}

/**
 * The choice of a playing video's subtitles, the chosen track drawn over it by JASSUB whether or not
 * a menu is drawn. It starts with the person's choices for this file, and keeps each change to them.
 */
export function useSubtitleChoice(props: SubtitleChoiceWays): SubtitleChoice {
  const { t } = useTranslation();
  const { api } = useServices();
  const [chosenTrackKey, setChosenTrackKey] = useState(() =>
    startingTrackKey(props.preferences, props.tracks),
  );
  const [offsetMs, setOffsetMs] = useState(props.preferences.subtitleOffsetMs);
  // The offset a newly chosen track starts with, read when its text arrives rather than in render.
  const offsetRef = useRef(props.preferences.subtitleOffsetMs);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const track = props.tracks.find((candidate) => trackKey(candidate) === chosenTrackKey);
  const labelOf = useTrackLabel();
  const drawn = track === undefined ? undefined : drawnTrackOf(track, labelOf(track));
  const layerRef = useSubtitleLayer(drawn, props.videoRef, {
    offsetRef,
    onFailed: setFailure,
  });
  const shiftOffset = (stepMs: number): void => {
    const newOffsetMs = offsetMs + stepMs;
    setOffsetMs(newOffsetMs);
    offsetRef.current = newOffsetMs;
    layerRef.current?.setOffsetMs(newOffsetMs);
    keepOffset(api, props.fileId, newOffsetMs).catch((cause: unknown) => {
      setFailure(problemText(t, cause));
    });
  };
  const choose = (key: TrackKey | undefined): void => {
    setFailure(undefined);
    setChosenTrackKey(key);
    keepSubtitleChoice(api, props.fileId, key).catch((cause: unknown) => {
      setFailure(problemText(t, cause));
    });
  };
  return { tracks: props.tracks, chosenTrackKey, choose, offsetMs, shiftOffset, failure };
}

/** The subtitle menu and offset, held by whoever holds the choice, shown when there are tracks. */
export function SubtitleControls(props: { readonly choice: SubtitleChoice }): JSX.Element | null {
  const { t } = useTranslation();
  const { tracks, offsetMs, shiftOffset, failure } = props.choice;
  if (tracks.length === 0) {
    return null;
  }
  return (
    <div className="subtitles">
      <TrackMenu choice={props.choice} />
      <OffsetControl offsetMs={offsetMs} onMove={shiftOffset} />
      {failure === undefined ? null : (
        <p role="alert">{t("play.subtitles.failed", { reason: failure })}</p>
      )}
    </div>
  );
}

/** What draws a track: its ASS, and its WebVTT named as the menu names it, for fullscreen (D254). */
function drawnTrackOf(track: SubtitleTrack, label: string): DrawnTrack {
  return { assUrl: track.assUrl, vttUrl: track.vttUrl, language: track.language, label };
}

/** A track as useSubtitleLayer draws it. */
type DrawnTrack = CaptionSource & { readonly assUrl: string };

/** A track's name in the menu, in the viewer's language. */
export function useTrackLabel(): (track: SubtitleTrack) => string {
  const { t, i18n } = useTranslation();
  const words = {
    unknownLanguage: t("play.subtitles.unknownLanguage"),
    forced: t("play.subtitles.forced"),
    hearingImpaired: t("play.subtitles.hearingImpaired"),
    fetched: t("play.subtitles.fetched"),
  };
  return (track) => trackLabel(track, i18n.language, words);
}

/**
 * The choices of the subtitles: Off first, then each track by its name, the chosen one current. Under
 * the film (TrackMenu) and over it, 💬 (player-menus.tsx), alike.
 */
export function useSubtitleChoices(choice: SubtitleChoice): readonly ActionChoice[] {
  const { t } = useTranslation();
  const labelOf = useTrackLabel();
  const { tracks, chosenTrackKey, choose } = choice;
  const off: ActionChoice = {
    key: "off",
    label: t("play.subtitles.off"),
    isCurrent: chosenTrackKey === undefined,
    onChoose: () => {
      choose(undefined);
    },
  };
  const trackChoices = tracks.map((track): ActionChoice => {
    const key = trackKey(track);
    const label = labelOf(track);
    return {
      key: key ?? label,
      label,
      isCurrent: key !== undefined && key === chosenTrackKey,
      onChoose: () => {
        choose(key);
      },
    };
  });
  return [off, ...trackChoices];
}

/**
 * The menu of tracks under the film, with "Off" first: the app's chooser (route-chooser.tsx), which a
 * remote works, where a native select was, which a television's browser does not (the 2026-10-27
 * widget audit; plans/toolkit.md step 6).
 */
function TrackMenu(props: { readonly choice: SubtitleChoice }): JSX.Element {
  const { t } = useTranslation();
  const options = useSubtitleChoices(props.choice);
  return (
    <ActionChooser
      fieldLabel={t("play.subtitles.label")}
      current={currentLabelOf(options)}
      options={options}
    />
  );
}

/**
 * Moves the subtitles earlier or later by a tenth of a second, and shows where they are: in a row
 * under the film, or, isInAList, as an entry of ⁝'s list (player-menus.tsx), in a row that wraps to
 * the pop-up's width.
 */
export function OffsetControl(props: {
  readonly offsetMs: number;
  readonly onMove: (stepMs: number) => void;
  readonly isInAList?: boolean;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const parts = (
    <>
      <button
        type="button"
        onClick={() => {
          props.onMove(-offsetStepMs);
        }}
      >
        {t("play.subtitles.earlier")}
      </button>
      <output>
        {t("play.subtitles.offset", {
          seconds: formatOffsetSeconds(props.offsetMs, i18n.language),
        })}
      </output>
      <button
        type="button"
        onClick={() => {
          props.onMove(offsetStepMs);
        }}
      >
        {t("play.subtitles.later")}
      </button>
    </>
  );
  return props.isInAList === true ? (
    <Cluster space="2">{parts}</Cluster>
  ) : (
    <span className="subtitles__offset">{parts}</span>
  );
}

/**
 * Draws the track at its assUrl over the video once its text is extracted, with the same track as
 * captions for the browser's own fullscreen (D254), and removes both when another is chosen or the
 * page closes. It starts with the offset current when it is drawn; later changes go through the
 * returned layer.
 */
function useSubtitleLayer(
  drawn: DrawnTrack | undefined,
  videoRef: RefObject<HTMLVideoElement | null>,
  options: { offsetRef: RefObject<number>; onFailed: (why: string) => void },
): RefObject<SubtitleLayer | undefined> {
  const { t } = useTranslation();
  const { fetchMedia } = useServices();
  const layerRef = useRef<SubtitleLayer | undefined>(undefined);
  const { offsetRef, onFailed } = options;
  // Named by its parts, so a render that names the same track again keeps its layer.
  const { assUrl, vttUrl, language, label } = drawn ?? {};
  useEffect(() => {
    const video = videoRef.current;
    if (assUrl === undefined || vttUrl === undefined || video === null) {
      return undefined;
    }
    const captions = { vttUrl, language: language ?? "", label: label ?? "" };
    const aborter = new AbortController();
    textWhenReady(assUrl, { fetch: fetchMedia, signal: aborter.signal }).then(
      (ass) => {
        if (!aborter.signal.aborted) {
          layerRef.current = drawSubtitles(video, { assText: ass, captions }, offsetRef.current);
        }
      },
      (cause: unknown) => {
        if (!aborter.signal.aborted) {
          onFailed(problemText(t, cause));
        }
      },
    );
    return () => {
      aborter.abort();
      layerRef.current?.destroy();
      layerRef.current = undefined;
    };
  }, [assUrl, vttUrl, language, label, videoRef, offsetRef, onFailed, t, fetchMedia]);
  return layerRef;
}
