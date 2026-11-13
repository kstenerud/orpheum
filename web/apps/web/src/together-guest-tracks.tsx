// SPDX-License-Identifier: MPL-2.0

import { ownAudioTracksOf } from "@orpheum/player";
import { useState, type JSX } from "react";

import { AudioChoiceMenu, useAudioChoice } from "./audio-tracks.tsx";
import type { FilmParts } from "./film-player.tsx";
import { playTrackChosen, useOwnTrackChoice } from "./ladder-sound.ts";
import { SubtitleControls, useSubtitleChoice } from "./subtitles.tsx";
import type { PlaybackDecision } from "./watching.ts";

/**
 * The guest's own sound and subtitles (D471): the film's subtitles, drawn over it, and its audio
 * tracks where another switches in place, each choice played and shown on this page alone. Nothing
 * is sent to the group, nothing is kept as a person's choice (IsGuestContext), and the playback
 * is told the sound playing by the membership. The audio track is not offered where the element
 * cannot switch it in place, the film played as it is on a browser that lists no tracks: a guest
 * cannot ask for the host's film again. Neither levelling nor the dialogue boost is offered: each is
 * a person's own choice, kept by the server.
 */
export function GuestTracks(
  props: FilmParts & {
    readonly fileId: number;
    readonly decision: PlaybackDecision;
    /** The guest's membership's authorization, by which the playback is told the sound playing. */
    readonly headers: Readonly<Record<string, string>>;
  },
): JSX.Element {
  const { fileId, decision, headers, videoRef, ladder } = props;
  const subtitles = useSubtitleChoice({
    fileId,
    tracks: decision.subtitles ?? [],
    preferences: decision.preferences,
    videoRef,
  });
  const own = useOwnTrackChoice(decision, videoRef, { askAgain: staysAsItPlays, headers });
  const audio = useAudioChoice({
    fileId,
    decision,
    onChosen: (track) => {
      if (ladder === undefined) {
        own.onAudioChosen(track);
        return;
      }
      const isBoostOn = ladder.sound?.isBoosted ?? false;
      playTrackChosen(
        { audioTrackIndex: track.audioTrackIndex, isBoostOn, tracks: decision.tracks },
        { playSound: ladder.playSound, askAgain: staysAsItPlays },
      );
    },
  });
  const hasOwnTracks = useHasOwnAudioTracks();
  return (
    <div className="together-viewer__tracks">
      <SubtitleControls choice={subtitles} />
      {ladder !== undefined || hasOwnTracks ? <AudioChoiceMenu choice={audio} /> : null}
    </div>
  );
}

/** What a guest's choice that cannot play in place does: nothing, the film playing on as it is. */
function staysAsItPlays(): void {
  // A guest cannot ask for the host's film again (D471).
}

/**
 * Whether this browser's video lists a file's own audio tracks, as Safari's does, so another switches
 * in place (own-tracks.ts). A page drawn with no document, as a test draws it to markup, is taken not
 * to.
 */
function useHasOwnAudioTracks(): boolean {
  const [hasOwn] = useState(
    () =>
      typeof document !== "undefined" &&
      ownAudioTracksOf(document.createElement("video")) !== undefined,
  );
  return hasOwn;
}
