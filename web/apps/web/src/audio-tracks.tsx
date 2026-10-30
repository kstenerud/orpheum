// SPDX-License-Identifier: MPL-2.0

import type { Api, components, SoundTrack } from "@orpheum/sdk";
import { useMemo, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { languageName } from "./language-name.ts";
import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useSoundPart } from "./sound-panel.ts";

/** The server's decision on how a file plays, as play.tsx names it. */
type PlaybackDecision = components["schemas"]["StartPlaybackOutputBody"];

/** One of a file's own audio tracks, which a person chooses among (D230 §5). */
export type AudioTrack = components["schemas"]["AudioTrackBody"];

/**
 * Keeps a person's choice of an audio track in a file. Its language is their last choice in the
 * file's work, which the work's unwatched files start with (D230).
 */
export async function keepAudioChoice(api: Api, fileId: number, streamId: number): Promise<void> {
  throwIfRefused(
    await api.PUT("/files/{id}/preferences", {
      params: { path: { id: fileId } },
      body: { audioChoice: { streamId } },
    }),
  );
}

/** The audio track a playback starts with, or undefined for a file with no sound. */
export function startingAudioTrack(decision: PlaybackDecision): AudioTrack | undefined {
  return (decision.audioTracks ?? []).find(
    (track) => track.streamId === decision.startingAudioStreamId,
  );
}

/**
 * Has a cast's remote show the file's tracks and the one chosen, and choose among them, a choice
 * there made by choose as one here is (sound-panel.ts).
 */
function useTracksWorkedByARemote(
  tracks: readonly AudioTrack[],
  chosenStreamId: number | undefined,
  choose: (track: AudioTrack) => void,
): void {
  const shownTracks = useMemo(() => tracks.map(soundTrackOf), [tracks]);
  useSoundPart({ kind: "audioTrack", streamId: chosenStreamId, tracks: shownTracks }, (choice) => {
    const track = tracks.find(
      (each) => choice.kind === "audioTrack" && each.streamId === choice.streamId,
    );
    if (track !== undefined && track.streamId !== chosenStreamId) {
      choose(track);
    }
  });
}

/** A track as a remote names it, its stream and what labels it (audioTrackLabel). */
function soundTrackOf(track: AudioTrack): SoundTrack {
  return {
    streamId: track.streamId,
    language: track.language,
    ...(track.containerTitle === undefined ? {} : { containerTitle: track.containerTitle }),
    kind: track.kind,
  };
}

/** A track's name in the menu: its language, its container's title, and its kind when not plain. */
export function audioTrackLabel(
  track: {
    readonly language: string;
    readonly containerTitle?: string | undefined;
    readonly kind: AudioTrack["kind"];
  },
  viewerLanguage: string,
  words: { unknownLanguage: string; commentary: string; described: string },
): string {
  const parts = [
    track.language === "" ? words.unknownLanguage : languageName(track.language, viewerLanguage),
  ];
  if (track.containerTitle !== undefined && track.containerTitle !== "") {
    parts.push(track.containerTitle);
  }
  if (track.kind === "commentary") {
    parts.push(words.commentary);
  } else if (track.kind === "described") {
    parts.push(words.described);
  }
  return parts.join(" · ");
}

/**
 * The menu of a file's audio tracks, shown when it has more than one, starting with the track the
 * playback starts with. A choice is kept for the person, and then onChosen() plays it.
 */
export function AudioMenu(props: {
  readonly fileId: number;
  readonly decision: PlaybackDecision;
  readonly onChosen: (track: AudioTrack) => void;
}): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { api } = useServices();
  const [chosenStreamId, setChosenStreamId] = useState(props.decision.startingAudioStreamId);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const tracks = useMemo(() => props.decision.audioTracks ?? [], [props.decision.audioTracks]);
  const choose = (track: AudioTrack): void => {
    setProblem(undefined);
    setChosenStreamId(track.streamId);
    keepAudioChoice(api, props.fileId, track.streamId).then(
      () => {
        props.onChosen(track);
      },
      (cause: unknown) => {
        setProblem(problemText(t, cause));
      },
    );
  };
  useTracksWorkedByARemote(tracks, chosenStreamId, choose);
  if (tracks.length < 2) {
    return null;
  }
  const words = {
    unknownLanguage: t("play.audio.unknownLanguage"),
    commentary: t("play.audio.commentary"),
    described: t("play.audio.described"),
  };
  return (
    <>
      <label className="audio-tracks">
        {t("play.audio.label")}{" "}
        <select
          value={chosenStreamId ?? ""}
          onChange={(event) => {
            const track = tracks.find(
              (candidate) => String(candidate.streamId) === event.currentTarget.value,
            );
            if (track !== undefined) {
              choose(track);
            }
          }}
        >
          {tracks.map((track) => (
            <option key={track.streamId} value={track.streamId}>
              {audioTrackLabel(track, i18n.language, words)}
            </option>
          ))}
        </select>
      </label>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
}
