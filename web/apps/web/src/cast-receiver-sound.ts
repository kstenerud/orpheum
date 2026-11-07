// SPDX-License-Identifier: MPL-2.0

import {
  PlaybackTold,
  playerSwitchedQualityEvent,
  type CarriesSoundChoices,
  type PlaybackReport,
  type TellsPlaybackAlone,
  type TogetherConnection,
} from "@orpheum/sdk";
import { heardVolumeOf, setHeardVolume } from "@orpheum/player";
import { use, useEffect, useState } from "react";

import { PicturePanelContext } from "./picture-panel.ts";
import { useSoundPart, type SoundPanel } from "./sound-panel.ts";

/**
 * Has the remote work the page's sound controls: each choice it sends in the cast's session carried
 * out by the control it is of, as that control's own press, and each change of how the controls
 * stand told at once, for the remote to show (sound-panel.ts): to the session the video leads, with
 * its state, and to the cast's alone where that is another, as while its person hosts Watch Together
 * (useToldTheCastHowItPlays). A Fire TV's own controls are hidden while it is controlled, so its sound
 * could only be changed after taking control back (the owner, VQ116's run).
 */
export function useSoundWorkedByTheRemote(sessions: {
  readonly cast: (CarriesSoundChoices & TellsPlaybackAlone) | undefined;
  readonly led: TogetherConnection | undefined;
  readonly panel: SoundPanel | undefined;
}): void {
  const { cast, led, panel } = sessions;
  useEffect(() => {
    if (cast === undefined || panel === undefined) {
      return undefined;
    }
    const unsubscribe = cast.subscribeToSoundChoices(panel.choose);
    const unwatch = panel.subscribe(() => {
      led?.reportState();
      cast.reportPlayback();
    });
    return () => {
      unsubscribe();
      unwatch();
    };
  }, [cast, led, panel]);
}

/**
 * Has the remote set the film's level and mute on the receiver (D440 §2, §5): the film's volume as
 * the player's own slider sets it, and its mute, as the video says them at each volumechange, told
 * to the remote through the page's sound panel; a choice of them carried out on the video. They are
 * the film's channel, apart from the TV's own volume; they last for the session and are not saved.
 */
export function useFilmChannelWorkedByTheRemote(video: HTMLVideoElement | null): void {
  const [heard, setHeard] = useState<{ readonly level: number; readonly isMuted: boolean }>();
  useEffect(() => {
    if (video === null) {
      return undefined;
    }
    const read = (): void => {
      // The volume the person set, not the element's, which a lowering by levelling is in (D436).
      setHeard({ level: heardVolumeOf(video), isMuted: video.muted });
    };
    read();
    video.addEventListener("volumechange", read);
    return () => {
      video.removeEventListener("volumechange", read);
    };
  }, [video]);
  useSoundPart(heard === undefined ? undefined : { kind: "channel", ...heard }, (choice) => {
    if (choice.kind !== "channel" || video === null) {
      return;
    }
    setHeardVolume(video, choice.level);
    // eslint-disable-next-line no-param-reassign -- muting the film is setting its video's muted
    video.muted = choice.isMuted;
  });
}

/**
 * Tells the cast's session how the film plays and sounds here while the video leads another, its
 * person's Watch Together session (D364 §11), where the receiver is only a panel and says no state:
 * the cast's remote works the receiver in the cast's session alone (the owner, 2026-11-06: "It
 * shouldn't be watch together's business at all which TV has what offset or playback
 * characteristics"). Nothing while the cast's session is the one it leads (cast undefined).
 */
export function useToldTheCastHowItPlays(told: {
  readonly cast: TellsPlaybackAlone | undefined;
  readonly video: HTMLVideoElement | null;
  readonly delivery: PlaybackReport["delivery"] | undefined;
  readonly panel: SoundPanel | undefined;
}): void {
  const { cast, video, delivery, panel } = told;
  // What the film shows that may be chosen (picture-panel.ts, D440 §5), told with how it plays.
  const readPicture = use(PicturePanelContext)?.read;
  useEffect(() => {
    if (cast === undefined || video === null) {
      return undefined;
    }
    const playback = new PlaybackTold(delivery, panel?.read, readPicture);
    const heardQuality = (event: Event): void => {
      if (playback.heard(event)) {
        cast.reportPlayback();
      }
    };
    video.addEventListener(playerSwitchedQualityEvent, heardQuality);
    cast.setPlayback(() => playback.report());
    return () => {
      video.removeEventListener(playerSwitchedQualityEvent, heardQuality);
      cast.setPlayback(undefined);
    };
  }, [cast, video, delivery, panel, readPicture]);
}
