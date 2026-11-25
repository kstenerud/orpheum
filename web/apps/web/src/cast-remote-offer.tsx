// SPDX-License-Identifier: MPL-2.0

/*
 * The cast remote's offer area (D548 §2, §3, §9), apart from the remote's page (cast-remote.tsx):
 * the receiver's picture, the offer at its top-right, the Pill at its centre, and the note once the
 * receiver has not answered.
 */

import "./cast.css";
import "./play-words.ts";
import "./cast-words.ts";

import type { components, CastSending } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import type { UpNext } from "./around-file.ts";
import { CastFrame } from "./cast-frame.tsx";
import { isCasteeNearItsEnd } from "./castee-up-next.tsx";
import type { CastAgain } from "./cast-remote-again.ts";
import { RemoteSkipOffer, useFileSegments } from "./cast-segments.tsx";
import { moveCastOn } from "./cast.ts";
import type { PlaybackTarget, PlayerSound } from "./playback-target.ts";
import { Pill } from "./pill.tsx";
import { WideArt } from "./poster.tsx";
import { creditsStartOf } from "./segments.tsx";
import { useServices } from "./services.tsx";
import { StatusNote } from "./status-note.tsx";
import { TargetUpNext } from "./up-next.tsx";

/** A title's pictures, as its detail names them. */
type ArtBody = components["schemas"]["ArtBody"];

/**
 * The remote's offer area (D548 §2, §3, §9): the film's picture where its video would be, 16:9, the
 * key frame where the receiver says it stands (RemotePicture), hidden from a screen reader, which the
 * remote's sign tells where it plays; at its top-right the offer, Up next or a skip (RemoteOffers);
 * at its centre a Pill, "Reaching ‹TV›…" while the receiver is not reached, else the film's level a
 * moment after the pad moves it (film); the receiver unheard, nothing ended by the remote, the
 * server ending a cast whose castee is silent (D555 §3), met as any such end;
 * while the remote's own connection to the server is lost (isServerLost), "Can't reach the server"
 * in Reaching's place, no time counted against the receiver and nothing ended (D553 §4, D554 §2);
 * and, the cast ended by anything but this tab's own Stop (ended), its note saying why, "‹TV› took
 * control", "‹TV› unreachable" or "Casting ended", with Play here, nothing left to try, and Cast
 * again once the receiver is back (D552 §1, §4, D553 §2).
 */
/**
 * A cast's end this tab did not ask for, as the remote shows it (D552 §4, D553 §2): its note, why it
 * ended; Play here, which plays the film here as an ordinary play, asking the server for no end; and
 * Cast again, where the receiver is back.
 */
export interface RemoteEnded {
  readonly text: string;
  readonly playHere: () => void;
  readonly castAgain: CastAgain | undefined;
}

export function RemoteOffer(props: {
  readonly sent: CastSending;
  readonly source: PlaybackTarget;
  readonly upNext: UpNext | undefined;
  readonly art: ArtBody | undefined;
  /** The film's level on the receiver, while its Pill shows. */
  readonly film: PlayerSound | undefined;
  /** The cast's end this tab did not ask, said and played here from (D552 §4); undefined while it lasts. */
  readonly ended: RemoteEnded | undefined;
  /** Whether the remote's own connection to the server is lost, reconnecting (D553 §4). */
  readonly isServerLost: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { sent, source, ended, isServerLost } = props;
  const name = sent.receiverName;
  return (
    <div className="cast-remote__offer-cell">
      <div className="cast-remote__offer">
        <div className="cast-remote__picture-area" aria-hidden="true">
          <RemotePicture fileId={sent.fileId} atMs={source.reportedAtMs} art={props.art} />
        </div>
        <div className="cast-remote__offers">
          <RemoteOffers sent={sent} source={source} upNext={props.upNext} />
        </div>
        <span className="cast-remote__centre">
          {source.isReady ? <RemoteVolumePill film={props.film} /> : null}
          {source.isReady || ended !== undefined ? null : (
            <Pill icon={<span className="cast-remote__spinner" />}>
              {isServerLost ? t("cast.serverLost") : t("cast.reaching", { name })}
            </Pill>
          )}
        </span>
        {ended === undefined ? null : <RemoteNote ended={ended} />}
      </div>
    </div>
  );
}

/**
 * The remote's note (RemoteOffer): the cast's end and why (ended); Play here, which plays the film
 * here (D549); and Cast again where the ended cast offers it (D553 §2).
 */
function RemoteNote(props: { readonly ended: RemoteEnded }): JSX.Element {
  const { t } = useTranslation();
  const { ended } = props;
  return (
    <div className="cast-remote__note">
      <StatusNote told={{ kind: "problem", text: ended.text }} />
      <button type="button" onClick={ended.playHere}>
        {t("cast.playHere")}
      </button>
      <CastAgainButton castAgain={ended.castAgain} />
    </div>
  );
}

/** The ended remote's Cast again beside Play here (D553 §2), and why it failed; none where not offered. */
function CastAgainButton(props: { readonly castAgain: CastAgain | undefined }): JSX.Element | null {
  const { t } = useTranslation();
  const { castAgain } = props;
  if (castAgain === undefined) {
    return null;
  }
  return (
    <>
      <button type="button" disabled={castAgain.isBusy} onClick={castAgain.press}>
        {t("cast.over.castAgain")}
      </button>
      {castAgain.problem === undefined ? null : (
        <StatusNote told={{ kind: "problem", text: castAgain.problem }} />
      )}
    </>
  );
}

/** The film's level on the receiver as a Pill, "Volume 70 %" or "Sound off" (D548 §6); none hidden. */
function RemoteVolumePill(props: { readonly film: PlayerSound | undefined }): JSX.Element | null {
  const { t } = useTranslation();
  const { film } = props;
  if (film === undefined) {
    return null;
  }
  return (
    <Pill voice="volume">
      {film.isMuted
        ? t("cast.pad.soundOff")
        : t("playerControls.volumeLevel", { percent: Math.round(film.volume * 100) })}
    </Pill>
  );
}

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
