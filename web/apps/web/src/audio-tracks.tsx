// SPDX-License-Identifier: MPL-2.0

import type { Api, components, SoundTrack } from "@orpheum/sdk";
import { useMemo, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { languageName } from "./language-name.ts";
import { useIsGuestReceiver } from "./guest-receiver.ts";
import { problemText, throwIfRefused } from "./problem.ts";
import { ActionChooser, currentLabelOf, type ActionChoice } from "./route-chooser.tsx";
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

/** A file's audio tracks, the one chosen, the way to choose another, and what failed. */
export interface AudioChoice {
  readonly tracks: readonly AudioTrack[];
  readonly chosenStreamId: number | undefined;
  readonly choose: (track: AudioTrack) => void;
  /** Why keeping the last choice failed, in the viewer's words; undefined while nothing has. */
  readonly problem: string | undefined;
}

/** What the choice of a file's audio track is of, and what plays a track chosen. */
interface AudioChoiceWays {
  readonly fileId: number;
  readonly decision: PlaybackDecision;
  readonly onChosen: (track: AudioTrack) => void;
}

/**
 * The choice of a file's audio track, starting with the track the playback starts with. A choice is
 * kept for the person, and then onChosen() plays it. A cast's remote shows the tracks and chooses
 * among them whether or not a menu is drawn (sound-panel.ts).
 */
export function useAudioChoice(props: AudioChoiceWays): AudioChoice {
  const { t } = useTranslation();
  const { api } = useServices();
  const [chosenStreamId, setChosenStreamId] = useState(props.decision.startingAudioStreamId);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const tracks = useMemo(() => props.decision.audioTracks ?? [], [props.decision.audioTracks]);
  // A guest receiver keeps no choice, its person's preferences refused it (D469 §4): it plays one.
  const isGuest = useIsGuestReceiver();
  const choose = (track: AudioTrack): void => {
    setProblem(undefined);
    setChosenStreamId(track.streamId);
    (isGuest ? Promise.resolve() : keepAudioChoice(api, props.fileId, track.streamId)).then(
      () => {
        props.onChosen(track);
      },
      (cause: unknown) => {
        setProblem(problemText(t, cause));
      },
    );
  };
  useTracksWorkedByARemote(tracks, chosenStreamId, choose);
  return { tracks, chosenStreamId, choose, problem };
}

/**
 * The menu of a file's audio tracks, shown when it has more than one, starting with the track the
 * playback starts with. A choice is kept for the person, and then onChosen() plays it.
 */
export function AudioMenu(props: AudioChoiceWays): JSX.Element | null {
  return <AudioChoiceMenu choice={useAudioChoice(props)} />;
}

/**
 * The choices of a file's audio tracks, the one playing current: the chosen, else the first, which
 * the playback plays with no choice marked. Under the film (AudioChoiceMenu) and over it, 🎧
 * (player-menus.tsx), alike.
 */
export function useAudioChoices(choice: AudioChoice): readonly ActionChoice[] {
  const { t, i18n } = useTranslation();
  const { tracks, chosenStreamId, choose } = choice;
  const words = {
    unknownLanguage: t("play.audio.unknownLanguage"),
    commentary: t("play.audio.commentary"),
    described: t("play.audio.described"),
  };
  const playingId = chosenStreamId ?? tracks[0]?.streamId;
  return tracks.map((track): ActionChoice => ({
    key: String(track.streamId),
    label: audioTrackLabel(track, i18n.language, words),
    isCurrent: track.streamId === playingId,
    onChoose: () => {
      choose(track);
    },
  }));
}

/**
 * The menu of a file's audio tracks under the film, held by whoever holds the choice, shown when it
 * has two or more: the app's chooser (route-chooser.tsx), which a remote works, where a native select
 * was, which a television's browser does not (the 2026-10-27 widget audit; plans/toolkit.md step 6).
 */
export function AudioChoiceMenu(props: { readonly choice: AudioChoice }): JSX.Element | null {
  const { t } = useTranslation();
  const { tracks, problem } = props.choice;
  const options = useAudioChoices(props.choice);
  if (tracks.length < 2) {
    return null;
  }
  return (
    <>
      <ActionChooser
        fieldLabel={t("play.audio.label")}
        current={currentLabelOf(options)}
        options={options}
      />
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
}
