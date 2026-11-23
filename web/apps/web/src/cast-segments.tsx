// SPDX-License-Identifier: MPL-2.0

/**
 * A cast's remote's segments and its skip (D546): casting behaves as playing here does, so the
 * remote offers Skip intro, recap, preview and credits as the player does (segments.tsx's
 * SkipOffer), and shows Up next from the credits, from the segments of the file the cast plays,
 * which it reads (GET /files/{id}/segments), starting no playback of its own.
 */

import "./play-words.ts";

import { useMemo, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { OfferCard } from "./offer-card.tsx";
import { dataOrThrow } from "./problem.ts";
import {
  isInside,
  isSkipOffered,
  skipWords,
  useSkipShowing,
  type MediaSegment,
  type SkippedSegment,
} from "./segments.tsx";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** No segments: while the file's are read, and where they cannot be. */
const noSegments: readonly MediaSegment[] = [];

/**
 * The segments of fileId, the file a cast plays, as its playback's decision carries them; none
 * while they are read, and where they cannot be, so the remote offers no skip and Up next keeps
 * its fixed lead then, as for a file with none.
 */
export function useFileSegments(fileId: number): readonly MediaSegment[] {
  const { api } = useServices();
  const [segments] = useResource(
    `segments/${String(fileId)}`,
    async (): Promise<readonly MediaSegment[]> =>
      dataOrThrow(await api.GET("/files/{id}/segments", { params: { path: { id: fileId } } }))
        .segments ?? noSegments,
  );
  return segments.state === "loaded" ? segments.value : noSegments;
}

/**
 * The remote's skip (D546): while the cast's position, atMs, is inside an intro, a recap, a preview
 * or credits, the same card and words as the player's (SkipOffer), whose press skips to the
 * segment's end (onSkip); pressed, it hides for that segment, as the player's does. Credits are left
 * to Up next where isCreditsLeftToUpNext. Nothing outside such a segment.
 */
export function RemoteSkipOffer(props: {
  readonly segments: readonly MediaSegment[];
  readonly atMs: number;
  /** Whether Up next takes Skip credits' place: the episode has a next one. */
  readonly isCreditsLeftToUpNext: boolean;
  readonly onSkip: (endMs: number) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { segments, atMs, isCreditsLeftToUpNext, onSkip } = props;
  const skipped = useMemo(
    () =>
      segments.filter(
        (segment): segment is SkippedSegment =>
          isSkipOffered(segment) && !(isCreditsLeftToUpNext && segment.type === "credits"),
      ),
    [segments, isCreditsLeftToUpNext],
  );
  const index = skipped.findIndex((segment) => isInside(segment, atMs));
  const at = index === -1 ? undefined : index;
  const { isPressed, press } = useSkipShowing(at);
  const segment = at === undefined || isPressed ? undefined : skipped[at];
  if (segment === undefined) {
    return null;
  }
  const skip = (): void => {
    press();
    onSkip(segment.endMs);
  };
  return <OfferCard words={t(skipWords[segment.type])} onTake={skip} />;
}
