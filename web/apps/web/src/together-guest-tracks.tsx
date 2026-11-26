// SPDX-License-Identifier: MPL-2.0

import { ownAudioTracksOf } from "@orpheum/player";
import { useMemo, useState, type JSX, type ReactNode } from "react";

import { useAudioChoice } from "./audio-tracks.tsx";
import type { FilmParts } from "./film-player.tsx";
import { playTrackChosen, useOwnTrackChoice } from "./ladder-sound.ts";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import { useThisDeviceSoundTiming } from "./sound-delay.tsx";
import { useSubtitleChoice, useSubtitlesWorkedByARemote } from "./subtitles.tsx";
import type { PlaybackDecision } from "./watching.ts";

/**
 * The guest's own sound and subtitles (D471), held around the guest's stage for the Play page's
 * Subtitles and Sound menus to read (PlayerTracksContext), as the Play page's are, none of them
 * left under the film (the owner, 2026-11-26: "The tracks and sound timing and such should also be
 * put in their relative submenus, visible only when it makes sense"): the film's subtitles, drawn
 * over it; its audio tracks where another switches in place; and this device's sound timing, played
 * as the menu leaves it (useThisDeviceSoundTiming). Each choice is played and shown on this page
 * alone. Nothing is sent to the group, nothing is kept as a person's choice (IsGuestContext), and the
 * playback is told the sound playing by the membership. The audio track is not offered where the
 * element cannot switch it in place, the film played as it is on a browser that lists no tracks: a
 * guest cannot ask for the host's film again. Neither levelling nor the dialogue boost is offered:
 * each is a person's own choice, kept by the server. A cast's remote reads and works the subtitles,
 * the audio track and the timing whether or not a menu is drawn, as a receiver playing a guest's
 * part draws none.
 */
export function GuestTracksProvider(
  props: FilmParts & {
    readonly fileId: number;
    readonly decision: PlaybackDecision;
    /** The guest's membership's authorization, by which the playback is told the sound playing. */
    readonly headers: Readonly<Record<string, string>>;
    readonly children: ReactNode;
  },
): JSX.Element {
  const { fileId, decision, headers, videoRef, ladder } = props;
  const subtitles = useSubtitleChoice({
    fileId,
    tracks: decision.subtitles ?? [],
    preferences: decision.preferences,
    videoRef,
  });
  useSubtitlesWorkedByARemote(subtitles);
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
  const hasOwnAudioTracks = useHasOwnAudioTracks();
  const canSwitch = ladder !== undefined || hasOwnAudioTracks;
  const deviceTimingMs = useThisDeviceSoundTiming(videoRef);
  const tracks = useMemo(
    (): PlayerTracks => ({
      subtitles,
      audio: canSwitch ? audio : { ...audio, tracks: [] },
      level: own.level,
      measuring: undefined,
      boost: undefined,
      deviceTimingMs,
    }),
    [subtitles, audio, canSwitch, own.level, deviceTimingMs],
  );
  return <PlayerTracksContext value={tracks}>{props.children}</PlayerTracksContext>;
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
