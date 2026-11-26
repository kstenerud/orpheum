// SPDX-License-Identifier: MPL-2.0

/*
 * What the castee's Play page (play-castee.tsx) shows where the video would be (D548 §2, §3): the
 * receiver's picture and the offer at its top-right.
 */

import "./cast.css";
import "./play-words.ts";
import "./cast-words.ts";

import type { components, CastSending } from "@orpheum/sdk";
import type { JSX } from "react";

import type { UpNext } from "./around-file.ts";
import { CastFrame } from "./cast-frame.tsx";
import { isCasteeNearItsEnd } from "./castee-up-next.tsx";
import { RemoteSkipOffer, useFileSegments } from "./cast-segments.tsx";
import { moveCastOn } from "./cast.ts";
import type { PlaybackTarget } from "./playback-target.ts";
import { WideArt } from "./poster.tsx";
import { creditsStartOf } from "./segments.tsx";
import { useServices } from "./services.tsx";
import { TargetUpNext } from "./up-next.tsx";

/** A title's pictures, as its detail names them. */
type ArtBody = components["schemas"]["ArtBody"];

/**
 * The offer at the offer area's top-right, as the player's (PlayersOffers, D546): Up next over the
 * castee from the film's credits, or its fixed lead where it has none (TargetUpNext), until it is
 * dismissed here, offered again once the film is moved back from its end; else the skip of the segment the cast is
 * in, Skip credits left to Up next where an episode plays next (RemoteSkipOffer), its press seeking
 * the cast past it.
 */
export function RemoteOffers(props: {
  readonly sent: CastSending;
  readonly source: PlaybackTarget;
  readonly upNext: UpNext | undefined;
}): JSX.Element | null {
  const { sent, source, upNext } = props;
  const { api } = useServices();
  const segments = useFileSegments(sent.fileId);
  const creditsStartMs = creditsStartOf(segments);
  const isNextPlayable = upNext !== undefined && upNext.episode.unavailable === undefined;
  if (isNextPlayable && isCasteeNearItsEnd(source, creditsStartMs)) {
    return (
      <TargetUpNext
        target={source}
        upNext={upNext}
        creditsStartMs={creditsStartMs}
        onPlay={() => {
          moveCastOn(api, sent.castId).catch(() => undefined); // the cast goes on as it was
        }}
      />
    );
  }
  return (
    <RemoteSkipOffer
      segments={segments}
      atMs={source.atMs}
      isCreditsLeftToUpNext={isNextPlayable}
      onSkip={source.seekTo}
    />
  );
}

/**
 * The film's picture where its video would be, covering the offer area at its full brightness (D548
 * §2): the key frame where the receiver says it stands, no seek chosen here counted (the rework plan,
 * phase 3), renewed every 3 s while it moves (CastFrame, D545),
 * and until the first has loaded, or where the node makes none, the film's art (RemoteArt).
 */
export function RemotePicture(props: {
  readonly fileId: number;
  readonly atMs: number;
  readonly art: ArtBody | undefined;
}): JSX.Element {
  return (
    <CastFrame
      fileId={props.fileId}
      atMs={props.atMs}
      className="cast-remote__frame"
      fallback={<RemoteArt art={props.art} />}
    />
  );
}

/**
 * The film's art as its picture (D548 §2): an episode's still, else its backdrop, else the still a
 * film with no poster has; nothing where it has none, or while it is read, the area's placeholder
 * ground showing.
 */
function RemoteArt(props: { readonly art: ArtBody | undefined }): JSX.Element | null {
  const { art } = props;
  const wide = art?.stillSha256 ?? art?.backdropSha256 ?? art?.noPosterStillSha256;
  if (wide === undefined) {
    return null;
  }
  return <WideArt sourceSha256={wide} size="landscape-l" className="cast-remote__picture" />;
}
