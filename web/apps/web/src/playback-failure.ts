// SPDX-License-Identifier: MPL-2.0

import { reportFailure, type Api, type components } from "@orpheum/sdk";
import { useCallback, useEffect, useEffectEvent, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow, unavailableText } from "./problem.ts";
import type { ReopenAt } from "./reopen.ts";
import { useServices } from "./services.tsx";

/**
 * What the viewer is told when playback fails: what the player saw, at once, and, when the server
 * says the file cannot be read now, why instead: its machine away, or its folder or disk missing
 * (D241 §7). A file that can be read leaves the player's words. The server is told what the player saw,
 * and what failed where the player says (whatFailed), for its log alone.
 * While the file cannot be read, the server is asked again, each ask held until it can
 * (useMediaBack), and once it can, onMediaBack is told where the video stood, to open the film
 * again from there (the owner, 2026-10-21: a player an outage stopped stayed stopped once the
 * films' NAS was back).
 */
export function usePlaybackFailure(
  fileId: number | undefined,
  back?: MediaBack,
): [string | undefined, (seen: string, whatFailed?: string) => void] {
  const { api } = useServices();
  const { t } = useTranslation();
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [isUnreachable, setIsUnreachable] = useState(false);
  const fail = useCallback(
    (seen: string, whatFailed?: string) => {
      setFailure(seen);
      // The server is told, beside the page's own words: a film that stopped for good said so
      // only on the person's screen (the owner's test, 2026-10-18); with what failed where the player
      // says, which the page's words alone never did ("the browser gave no reason", staging,
      // 2026-10-29).
      reportFailure(
        new Error(
          `the player failed: ${seen}${whatFailed === undefined ? "" : ` (${whatFailed})`}`,
        ),
      );
      if (fileId === undefined) {
        return;
      }
      fetchUnavailability(api, fileId).then(
        (unavailable) => {
          if (unavailable !== undefined) {
            setFailure(unavailableText(t, unavailable));
            setIsUnreachable(true);
          }
        },
        () => undefined, // the player's own words stand
      );
    },
    [api, fileId, t],
  );
  useMediaBack(fileId, isUnreachable, back);
  return [failure, fail];
}

/** What a player tells once its film can be read again, and the video it reads where it stood from. */
export interface MediaBack {
  readonly onMediaBack?: ((at: ReopenAt) => void) | undefined;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}

/**
 * How long each ask of a player its film's being unreachable stopped asks the server to hold its
 * answer while the film cannot be read: the server asks the film's machine meanwhile, and answers as
 * soon as it can be read (D327 §3).
 */
export const mediaBackWaitSeconds = 20;

/** How long after an answer that it still cannot be read the player asks again. */
export const mediaBackAskAgainMs = 2_000;

/** How long after an ask that failed the player asks again. */
export const mediaBackAfterFailureMs = 10_000;

/**
 * Asks whether the file can be read again while isUnreachable, each ask held by the server until it
 * can or for mediaBackWaitSeconds, and once it can tells onMediaBack, once, where the video stood: its
 * position once it had loaded enough to say, else the start, and whether it was paused. Asked every
 * ten seconds, a film resumed up to ten seconds after its share was back (staging, 2026-10-21).
 */
function useMediaBack(
  fileId: number | undefined,
  isUnreachable: boolean,
  back: MediaBack | undefined,
): void {
  const { api } = useServices();
  const onMediaBack = back?.onMediaBack;
  const canTellBack = onMediaBack !== undefined;
  // Told through an effect event, not a dependency: the page draws itself again as it opens the
  // film again, with a new onMediaBack, and a loop begun again for it asked again, was told at once
  // that the film could be read, and opened it a second time (staging, 2026-10-21).
  const tellBack = useEffectEvent((at: ReopenAt) => {
    onMediaBack?.(at);
  });
  const videoRef = back?.videoRef;
  useEffect(() => {
    if (!isUnreachable || fileId === undefined || !canTellBack) {
      return undefined;
    }
    let isWaiting = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const askAfter = (ms: number): void => {
      timer = setTimeout(ask, ms);
    };
    const ask = (): void => {
      fetchUnavailability(api, fileId, mediaBackWaitSeconds).then(
        (unavailable) => {
          if (!isWaiting) {
            return;
          }
          if (unavailable !== undefined) {
            askAfter(mediaBackAskAgainMs);
            return;
          }
          isWaiting = false;
          const video = videoRef?.current;
          const haveMetadata = 1; // HTMLMediaElement.HAVE_METADATA
          const isLoaded =
            video !== undefined && video !== null && video.readyState >= haveMetadata;
          tellBack({
            positionMs: isLoaded ? video.currentTime * 1000 : 0,
            isPaused: video?.paused ?? false,
          });
        },
        () => {
          if (isWaiting) {
            askAfter(mediaBackAfterFailureMs);
          }
        },
      );
    };
    ask();
    return () => {
      isWaiting = false;
      clearTimeout(timer);
    };
  }, [api, fileId, isUnreachable, canTellBack, videoRef]);
}

/** Why a file cannot be read now, as the server says, or undefined when it can. */
async function fetchUnavailability(
  api: Api,
  fileId: number,
  waitSeconds = 0,
): Promise<components["schemas"]["UnavailableBody"] | undefined> {
  const answer = dataOrThrow(
    await api.GET("/files/{id}/availability", {
      params: { path: { id: fileId }, query: { waitSeconds } },
    }),
  );
  return answer.unavailable;
}
