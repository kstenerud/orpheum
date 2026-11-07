// SPDX-License-Identifier: MPL-2.0

import type { CarriesPictureChoices, TellsPlaybackAlone, TogetherConnection } from "@orpheum/sdk";
import { use, useEffect } from "react";

import { PicturePanelContext, type PicturePanel } from "./picture-panel.ts";

/**
 * Has the remote work what the page shows that its person may choose, its subtitles and the
 * original (D440 §5): each choice sent in the cast's session (D434) carried out by the control it is
 * of, as that control's own press, and each change of how the controls stand told at once, for the
 * remote to show, as the sound's are (useSoundWorkedByTheRemote): to the session the video leads,
 * with its state, and to the cast's alone where that is another. The panel is the page's
 * (PicturePanelContext), returned for the page to tell what it shows with how it plays.
 */
export function usePictureWorkedByTheRemote(sessions: {
  readonly cast: (CarriesPictureChoices & TellsPlaybackAlone) | undefined;
  readonly led: TogetherConnection | undefined;
}): PicturePanel | undefined {
  const { cast, led } = sessions;
  const panel = use(PicturePanelContext);
  useEffect(() => {
    if (cast === undefined || panel === undefined) {
      return undefined;
    }
    const unsubscribe = cast.subscribeToPictureChoices(panel.choose);
    const unwatch = panel.subscribe(() => {
      led?.reportState();
      cast.reportPlayback();
    });
    return () => {
      unsubscribe();
      unwatch();
    };
  }, [cast, led, panel]);
  return panel;
}
