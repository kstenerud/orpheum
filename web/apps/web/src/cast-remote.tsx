// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import {
  leaderOf,
  type CarriesSoundChoices,
  type CastSending,
  type PlaybackReport,
  type PlaybackSound,
} from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { audioTrackLabel } from "./audio-tracks.tsx";
import { browserStorage, tabStorage } from "./browser-storage.ts";
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
  readonly connection: ReturnType<typeof useTogether>["connection"];
  readonly positionMs: number | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const isGuest = useMembership(browserStorage, "viewer") !== undefined;
  const { sent } = props;
  if (isHosting || (isGuest && sent.fileId === 0)) {
    // A guest's cast with nothing of its own follows their session: its page holds the controls.
    return (
      <>
        <p>{t("cast.withTogether", { name: sent.receiverName })}</p>
        <Link to={{ screen: isHosting ? "watch-together" : "together" }}>
          {t("cast.toTogether")}
        </Link>
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
  const playback = props.snapshot === undefined ? undefined : leaderOf(props.snapshot)?.playback;
  return (
    <>
      <h2>{sent.titleName === "" ? t("cast.aFilm") : sent.titleName}</h2>
      <p>{t("cast.playingOn", { name: sent.receiverName })}</p>
      <ReceiverPlayback playback={playback} />
      <ReceiverSound sound={playback?.sound} connection={props.connection} />
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
 * The receiver's sound, as it says it, and the remote's way to change it: the dialogue boost where
 * its film has a boosted mix, levelling, and the audio track where the file has more than one, each
 * choice sent to the receiver, which carries it out as its own press and says how it sounds after.
 * A Fire TV's own controls are hidden while it is controlled, so its sound could only be changed
 * after taking control back (the owner, VQ116's run). Nothing while the receiver has said nothing.
 */
function ReceiverSound(props: {
  readonly sound: PlaybackSound | undefined;
  readonly connection: CarriesSoundChoices | undefined;
}): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { sound, connection } = props;
  if (sound === undefined || connection === undefined) {
    return null;
  }
  const words = {
    unknownLanguage: t("play.audio.unknownLanguage"),
    commentary: t("play.audio.commentary"),
    described: t("play.audio.described"),
  };
  return (
    <div className="cast-remote__sound">
      {sound.isBoostOn === undefined ? null : (
        <RemoteSwitch
          key={`boost-${String(sound.isBoostOn)}`}
          label={t("play.boost")}
          isOn={sound.isBoostOn}
          onSwitched={(isOn) => {
            connection.chooseSound({ kind: "boost", isOn });
          }}
        />
      )}
      <RemoteSwitch
        key={`levelling-${String(sound.isLevellingOn)}`}
        label={t("play.level.switch")}
        isOn={sound.isLevellingOn}
        onSwitched={(isOn) => {
          connection.chooseSound({ kind: "levelling", isOn });
        }}
      />
      {sound.audioTracks.length < 2 ? null : (
        <label className="audio-tracks">
          {t("play.audio.label")}{" "}
          <select
            key={`audio-${String(sound.audioStreamId)}`}
            defaultValue={sound.audioStreamId ?? ""}
            onChange={(event) => {
              connection.chooseSound({
                kind: "audioTrack",
                streamId: Number(event.currentTarget.value),
              });
            }}
          >
            {sound.audioTracks.map((track) => (
              <option key={track.streamId} value={track.streamId}>
                {audioTrackLabel(track, i18n.language, words)}
              </option>
            ))}
          </select>
        </label>
      )}
    </div>
  );
}

/**
 * One of the receiver's switches on the remote: as the receiver said it stands, and as pressed here
 * until the receiver says again (its key changes with what it says).
 */
function RemoteSwitch(props: {
  readonly label: string;
  readonly isOn: boolean;
  readonly onSwitched: (isOn: boolean) => void;
}): JSX.Element {
  const [isOn, setIsOn] = useState(props.isOn);
  return (
    <label className="sound__switch">
      <input
        type="checkbox"
        checked={isOn}
        onChange={(event) => {
          const isNowOn = event.currentTarget.checked;
          setIsOn(isNowOn);
          props.onSwitched(isNowOn);
        }}
      />
      {props.label}
    </label>
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
