// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import { leaderOf, type CastSending, type PlaybackReport } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { CastTargets } from "./cast-to.tsx";
import { endCast, listMyCasts, moveCastOn, playInCast } from "./cast.ts";
import { placeCode } from "./episode-place.ts";
import { Link } from "./link.tsx";
import { useLive } from "./live.tsx";
import { positionPlayedHere } from "./playing-here.ts";
import { rateText } from "./rate-text.ts";
import { useServices, useToken } from "./services.tsx";
import { SessionRemote, usePositionNow } from "./session-remote.tsx";
import { clockText } from "./timeline.tsx";
import { useMembership } from "./together-membership.ts";
import { useUpNext } from "./up-next.tsx";
import { useResource } from "./use-resource.ts";
import { useTogether } from "./use-together.ts";

/**
 * The casting page, where the tray's Cast leads (D364 §8, §13): with no cast, the devices to cast to,
 * a cast started there moving fileId, the film this device was playing as the person came, to the
 * receiver at its position (§1); with one, its remote, which the person returns to from anywhere, the
 * video playing on as they browse. Reached from anywhere, it has no Up; Back returns to where the
 * person was (D367). A cast that ends while shown stays shown, ended, until another starts.
 */
export function Casting(props: { readonly fileId?: number | undefined }): JSX.Element {
  const { t } = useTranslation();
  const { sending, isSendingKnown } = useLive();
  const shown = useShownCast(sending);
  const { fileId } = props;
  const film = (): { fileId: number; positionMs: number | undefined } | undefined =>
    fileId === undefined ? undefined : { fileId, positionMs: positionPlayedHere(fileId) };
  return (
    <section className="cast-remote" aria-labelledby="casting-heading">
      <h1 id="casting-heading">{t("cast.page")}</h1>
      {!isSendingKnown && shown === undefined ? <p>{t("common.loading")}</p> : null}
      {isSendingKnown && shown === undefined ? <CastTargets film={film} /> : null}
      {shown === undefined ? null : <Remote key={shown.castId} sent={shown} />}
    </section>
  );
}

/**
 * The cast the page shows: the one this device sends, once its receiver said Use, and after it
 * ends, the last one, so its end is said, until another is taken.
 */
function useShownCast(sending: CastSending | undefined): CastSending | undefined {
  const taken = sending?.isAccepted === true ? sending : undefined;
  const [held, setHeld] = useState<CastSending | undefined>(taken);
  if (taken !== undefined && taken !== held) {
    setHeld(taken); // kept as it renders, so it is there once the cast ends
  }
  return taken ?? held;
}

/**
 * The remote of one cast (D364 §8): it holds no player and loads no film; what it shows is what the
 * live connection says plays on the receiver, and where, as the cast's session says, moved on by the
 * clock between. With a video playing: its title, the controls, Up next, "Play on this device
 * instead" (§4) and Stop casting; with none, "Ready on ‹Device›" and Stop casting; while Watch
 * Together is on too, the controls are its page's, which this one leads to (§11, §13). Once the cast
 * ends it says why, offers to play here from where it stood (§10), and the devices to cast to again.
 */
function Remote(props: { readonly sent: CastSending }): JSX.Element {
  const { t } = useTranslation();
  const { sent } = props;
  const { connection, snapshot, positionMs, over, stop } = useCastSession(sent.castId);
  if (over !== undefined) {
    return (
      <>
        <CastOver sent={sent} reason={over.reason} positionMs={over.positionMs} />
        <CastTargets film={() => undefined} />
      </>
    );
  }
  return (
    <>
      <RemoteOf sent={sent} snapshot={snapshot} connection={connection} positionMs={positionMs} />
      <button type="button" onClick={stop}>
        {t("cast.stop")}
      </button>
    </>
  );
}

/** How a cast ended, as its remote says it: why, and where its video stood. */
interface CastEnd {
  readonly reason: string;
  readonly positionMs: number;
}

/**
 * The cast castId's session, joined as this device's remote by a credential read from the server for
 * it: what it says, where its video stands now, how it ended once it has (stopped here, ended
 * elsewhere, or no longer the server's), and Stop casting, which ends it.
 */
function useCastSession(castId: number): {
  readonly connection: ReturnType<typeof useTogether>["connection"];
  readonly snapshot: ReturnType<typeof useTogether>["snapshot"];
  readonly positionMs: number | undefined;
  readonly over: CastEnd | undefined;
  readonly stop: () => void;
} {
  const { api } = useServices();
  const token = useToken();
  const [casts] = useResource(`my-casts/${String(castId)}`, async () => await listMyCasts(api));
  const own =
    casts.state === "loaded"
      ? casts.value.find((each) => each.castId === castId && each.isSender)
      : undefined;
  const { connection, snapshot } = useTogether({
    credential: own?.membershipCredential,
    sessionToken: token,
    role: "panel",
  });
  const positionMs = usePositionNow(snapshot);
  const [stoppedAtMs, setStoppedAtMs] = useState<number | undefined>(undefined);
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  const isGone = casts.state === "loaded" && own === undefined;
  const stop = (): void => {
    const atMs = positionMs ?? 0;
    endCast(api, castId, "stopped").then(
      (ended) => {
        setStoppedAtMs(ended.positionMs);
      },
      () => {
        setStoppedAtMs(atMs);
      },
    );
  };
  return {
    connection,
    snapshot,
    positionMs,
    over: endOf({ stoppedAtMs, endReason, isGone, positionMs }),
    stop,
  };
}

/** How a cast ended, from Stop here, its session's end, or its absence; undefined while it lasts. */
function endOf(cast: {
  readonly stoppedAtMs: number | undefined;
  readonly endReason: string | undefined;
  readonly isGone: boolean;
  readonly positionMs: number | undefined;
}): CastEnd | undefined {
  const { stoppedAtMs, endReason, isGone, positionMs } = cast;
  if (stoppedAtMs !== undefined) {
    return { reason: "stopped", positionMs: stoppedAtMs };
  }
  if (endReason === undefined && !isGone) {
    return undefined;
  }
  return { reason: endReason ?? "gone", positionMs: positionMs ?? 0 };
}

/**
 * What the remote shows of what plays on the receiver: Watch Together's page holding the controls
 * while it is on too (D364 §11), the receiver ready with nothing playing (§5), or the film and its
 * controls.
 */
function RemoteOf(props: {
  readonly sent: CastSending;
  readonly snapshot: Parameters<typeof SessionRemote>[0]["snapshot"];
  readonly connection: Parameters<typeof SessionRemote>[0]["connection"];
  readonly positionMs: number | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const { sent } = props;
  if (isHosting) {
    return (
      <>
        <p>{t("cast.withTogether", { name: sent.receiverName })}</p>
        <Link to={{ screen: "watch-together" }}>{t("cast.toTogether")}</Link>
      </>
    );
  }
  if (sent.fileId === 0) {
    return <p>{t("cast.readyOn", { name: sent.receiverName })}</p>;
  }
  const playHereInstead = (): void => {
    const atMs = props.positionMs ?? 0;
    playInCast(api, sent.castId, undefined).catch(() => undefined); // the receiver waits ready
    navigation.go({ screen: "play", fileId: sent.fileId, atMs, shouldPlayHere: true });
  };
  return (
    <>
      <h2>{sent.titleName === "" ? t("cast.aFilm") : sent.titleName}</h2>
      <p>{t("cast.playingOn", { name: sent.receiverName })}</p>
      <ReceiverPlayback
        playback={props.snapshot === undefined ? undefined : leaderOf(props.snapshot)?.playback}
      />
      <SessionRemote
        snapshot={props.snapshot}
        connection={props.connection}
        durationMs={sent.durationMs}
      />
      <RemoteUpNext sent={sent} />
      <button type="button" onClick={playHereInstead}>
        {t("cast.playHereInstead")}
      </button>
    </>
  );
}

/**
 * How the receiver plays the film, as its state comes back to the remote (D371 §4): the file as it
 * is or converted, the quality it plays and the link it chose it on. The owner: "The controller side
 * should get information about playback, such as the transcoding and bandwidth choices". Nothing
 * while the receiver has said nothing.
 */
function ReceiverPlayback(props: {
  readonly playback: PlaybackReport | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { playback } = props;
  if (playback === undefined) {
    return null;
  }
  const { qualityKbps, linkKbps } = playback;
  return (
    <p className="cast-remote__playback">
      {[
        t(`cast.playback.${playback.delivery}`),
        qualityKbps === undefined
          ? undefined
          : t("cast.playback.quality", { rate: rateText(qualityKbps) }),
        linkKbps === undefined ? undefined : t("cast.playback.link", { rate: rateText(linkKbps) }),
      ]
        .filter((each) => each !== undefined)
        .join(" · ")}
    </p>
  );
}

/**
 * The remote's Up next: the cast's film's next episode, "Up next: S1E3 · ‹name›", which moves the
 * cast on to it, the receiver handed it (plans/up-next.md); nothing where none can play next.
 */
function RemoteUpNext(props: { readonly sent: CastSending }): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { sent } = props;
  const upNext = useUpNext(sent.fileId);
  if (upNext === undefined || upNext.episode.unavailable !== undefined) {
    return null;
  }
  const { episode } = upNext;
  const place = placeCode(t, upNext.ordering, { ...episode, isOnlyGroup: upNext.isOnlyGroup });
  return (
    <button
      type="button"
      className="cast-remote__up-next"
      onClick={() => {
        moveCastOn(api, sent.castId).catch(() => undefined); // the cast goes on as it was
      }}
    >
      {t("upNext.offer", { place, name: episode.name ?? "" })}
    </button>
  );
}

/**
 * The remote once its cast is over: why, in the person's words, and, for a video that was playing,
 * Play here from where it stood (D364 §10), which the Play page starts from.
 */
function CastOver(props: {
  readonly sent: CastSending;
  readonly reason: string;
  readonly positionMs: number;
}): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const { sent, reason } = props;
  return (
    <>
      <output>{overText(t, reason, sent.receiverName)}</output>
      {sent.fileId === 0 ? null : (
        <button
          type="button"
          onClick={() => {
            navigation.go({ screen: "play", fileId: sent.fileId, atMs: props.positionMs });
          }}
        >
          {t("cast.playHere", { at: clockText(props.positionMs) })}
        </button>
      )}
    </>
  );
}

/** Why a cast is over, in the person's words; name is its receiver's. */
function overText(t: TFunction, reason: string, name: string): string {
  switch (reason) {
    case "takenBack":
      return t("cast.over.takenBack", { name });
    case "replaced":
      return t("cast.over.replaced", { name });
    case "stopped":
      return t("cast.over.stopped", { name });
    case "receiverGone":
      return t("cast.over.receiverGone", { name });
    default:
      return t("cast.over.gone", { name });
  }
}
