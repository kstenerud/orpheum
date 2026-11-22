// SPDX-License-Identifier: MPL-2.0

import "./cast.css";
import "./cast-words.ts";

import type { TogetherSnapshot } from "@orpheum/sdk";
import { useEffect, useMemo, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { grantAt, grantBody } from "./airplay.ts";
import type { AirPlayGrants } from "./airplay-types.ts";
import { useAirPlayHere, useIsOnAirPlay } from "./airplay-here.ts";
import { AirPlayEntry } from "./airplay-entry.tsx";
import { ChromecastEntry, useGuestCastAskingBy } from "./cast-to.tsx";
import type { CastChannel } from "./cast-sdk.ts";
import {
  askTogetherCastGrantOf,
  endQuietly,
  GuestCastFailure,
  openGuestReceiverBy,
} from "./google-cast.ts";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { dataOrThrow, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { membershipHeaders, type Membership } from "./together-storage.ts";
import { useRequest } from "./use-request.ts";
import { StatusNote } from "./status-note.tsx";

/**
 * A Watch Together guest by link, signed in to nothing, casting to a Chromecast (D518): Chromecast
 * alone is offered, as the signed-in Cast menu's last choice is (ChromecastEntry), Orpheum's own
 * devices needing a sign-in (D515 §2). The grant it hands the device is asked for by the guest's
 * membership (POST /together/cast-grants), and the device reaches the core by the membership's
 * locator, as the signed-in cast's reaches it by the session's (google-cast.ts's
 * openGuestReceiverBy). The device takes the guest's part in their session, another device of
 * theirs following the group (cast-receiver's guest-part.ts); no cast is made to it, which needs a
 * person's session. The page's cast is its Cast session: Stop casting ends it, as does the page's
 * going, and Chrome's own Cast control ends it too, as the signed-in cast's ends with its session.
 *
 * In Safari, the dialog offers AirPlay after it, as the Cast menu does (AirPlayEntry), for the
 * group's film the guest's player plays on its ladder (D519 §1): its grant is asked for and revoked
 * by the guest's membership (membershipAirPlayGrants), the player handing the film over and back as
 * the signed-in person's does. Each film the group plays is a playback of its own, whose AirPlay is a
 * grant of its own: the film's change ends AirPlay of the last one, revoking its grant.
 */

/**
 * AirPlay's grants by the guest's membership credential (POST and DELETE /together/airplay-grants,
 * D519 §1), for the playback POST /together/playback answered; each ask is one press's, under a key
 * of its own (D244 §4).
 */
export function membershipAirPlayGrants(credential: string): AirPlayGrants {
  return {
    make: async (doing, asked, origin) => {
      doing.log("asking for the guest's grant (POST /together/airplay-grants)");
      const made = dataOrThrow(
        await doing.api.POST("/together/airplay-grants", {
          headers: { ...idempotent(newIdempotencyKey()).headers, ...membershipHeaders(credential) },
          body: grantBody(asked),
        }),
      );
      return grantAt(made, origin, doing.log);
    },
    revoke: async (doing) => {
      doing.log("revoking the guest's grants (DELETE /together/airplay-grants)");
      const answered = await doing.api.DELETE("/together/airplay-grants", {
        headers: membershipHeaders(credential),
      });
      return dataOrThrow(answered).revoked;
    },
  };
}

/** The guest's AirPlay grants, where they may AirPlay as their membership (isOffered). */
function useMembershipAirPlayGrants(
  credential: string,
  isOffered: boolean,
): AirPlayGrants | undefined {
  return useMemo(
    () => (isOffered ? membershipAirPlayGrants(credential) : undefined),
    [credential, isOffered],
  );
}

/** A Watch Together guest's cast to a Chromecast, from their page. */
export interface GuestsCast {
  /** Whether this page may cast to a Chromecast: Chrome, on a page its loader serves (guestCastOf). */
  readonly isOffered: boolean;
  /** The name of the Chromecast that takes the guest's part, while one does. */
  readonly castingTo: string | undefined;
  /** Whether Cast's dialog is open, Chromecast offered: until a device takes the part. */
  readonly isChoosing: boolean;
  readonly setIsChoosing: (isChoosing: boolean) => void;
  readonly isBusy: boolean;
  readonly problem: string | undefined;
  /** Opens Chrome's picker and hands the device picked its grant; showsLog, its log on screen. */
  readonly start: (showsLog: boolean) => void;
  /** Ends the Cast session: the device lets go of the guest's part and goes home. */
  readonly stop: () => void;
  /** The membership's AirPlay grants, which the guest's player AirPlays by (D519 §1). */
  readonly airPlayGrants: AirPlayGrants | undefined;
}

/**
 * The guest of membership's cast to a Chromecast, where isOffered: on the page of a guest by link
 * alone, a signed-in guest casting from the tray's Cast as any person does (D371 §4).
 */
export function useGuestsCast(membership: Membership, isOffered: boolean): GuestsCast {
  const { t } = useTranslation();
  const { api } = useServices();
  const { credential, locator } = membership;
  const askGrant = useMemo(
    () => (isOffered ? askTogetherCastGrantOf(api, credential) : undefined),
    [api, credential, isOffered],
  );
  const ways = useGuestCastAskingBy(askGrant);
  const airPlayGrants = useMembershipAirPlayGrants(credential, isOffered);
  const [channel, setChannel] = useState<CastChannel | undefined>(undefined);
  const [isChoosing, setIsChoosing] = useState(false);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    cause instanceof GuestCastFailure ? t("cast.chromecastFailed") : problemText(t, cause),
  );
  // Each Cast session let go of, by Stop casting or the page's going, is ended.
  useEffect(
    () => () => {
      if (channel !== undefined) {
        endQuietly(channel);
      }
    },
    [channel],
  );
  const start = (showsLog: boolean): void => {
    if (ways === undefined) {
      return;
    }
    runRequest(async () => {
      const receiver = await openGuestReceiverBy(ways, { locator, showsLog });
      if (receiver === undefined) {
        return; // the guest picked no device
      }
      const opened = receiver.channel;
      // Ended from Chrome's own Cast control, or by the device as the guest's part ends.
      opened.onEnded(() => {
        setChannel((now) => (now === opened ? undefined : now));
      });
      setChannel(opened);
      setIsChoosing(false);
    });
  };
  return {
    isOffered: ways !== undefined,
    castingTo: channel?.receiverName,
    isChoosing,
    setIsChoosing,
    isBusy,
    problem,
    start,
    stop: () => {
      setChannel(undefined);
    },
    airPlayGrants,
  };
}

/**
 * The guest's Cast, on their page's panel: while no Chromecast takes their part, Cast, which opens a
 * dialog of the one choice, Chromecast, saying there why a cast failed; while one does, which it is
 * and Stop casting. Chromecast waits while the session is not connected, as the Cast menu's does
 * while the live connection is down; then AirPlay, where the guest's player offers it (Safari), as
 * the Cast menu's last choice, the dialog closing as the film goes to it.
 */
export function GuestsCastEntry(props: {
  readonly cast: GuestsCast;
  readonly snapshot: TogetherSnapshot | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { cast, snapshot } = props;
  const airPlayHere = useAirPlayHere();
  const isAirPlayOffered = cast.airPlayGrants !== undefined && airPlayHere !== undefined;
  // Open, the dialog stays as the group's film changes, AirPlay offered again for the next one.
  if (!cast.isOffered && !isAirPlayOffered && !cast.isChoosing) {
    return null;
  }
  if (cast.castingTo !== undefined) {
    return <GuestsCastOn cast={cast} name={cast.castingTo} snapshot={snapshot} />;
  }
  return (
    <>
      <button
        type="button"
        onClick={() => {
          cast.setIsChoosing(true);
        }}
      >
        {t("cast.page")}
      </button>
      {cast.isChoosing ? (
        <GuestsCastChoices cast={cast} snapshot={snapshot} isAirPlayOffered={isAirPlayOffered} />
      ) : null}
    </>
  );
}

/**
 * Cast's dialog: Chromecast where this page casts to one, then AirPlay where it is offered; while the
 * film plays on AirPlay, AirPlay alone, one cast at a time (D523 §7).
 */
function GuestsCastChoices(props: {
  readonly cast: GuestsCast;
  readonly snapshot: TogetherSnapshot | undefined;
  readonly isAirPlayOffered: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { cast } = props;
  const isOnAirPlay = useIsOnAirPlay();
  const close = (): void => {
    cast.setIsChoosing(false);
  };
  return (
    <ModalDialog label={t("cast.page")} onClose={close}>
      <section className="cast-to" aria-labelledby="guest-cast-heading">
        <h2 id="guest-cast-heading">{t("cast.heading")}</h2>
        {cast.isOffered && !isOnAirPlay ? (
          <ChromecastEntry
            isStale={props.snapshot?.status !== "connected" || cast.isBusy}
            onCast={cast.start}
          />
        ) : null}
        {props.isAirPlayOffered ? <AirPlayEntry onPressed={close} /> : null}
        {cast.problem === undefined ? null : (
          <StatusNote told={{ kind: "problem", text: cast.problem }} />
        )}
      </section>
    </ModalDialog>
  );
}

/** Which Chromecast takes the guest's part, playing the group's film or waiting for one, and Stop. */
function GuestsCastOn(props: {
  readonly cast: GuestsCast;
  readonly name: string;
  readonly snapshot: TogetherSnapshot | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { name } = props;
  const isPlaying = (props.snapshot?.group?.fileId ?? 0) !== 0;
  return (
    <section className="cast-on" aria-label={t("cast.page")}>
      <p>{isPlaying ? t("cast.playingOn", { name }) : t("cast.castingTo", { name })}</p>
      <button type="button" onClick={props.cast.stop}>
        {t("cast.stop")}
      </button>
    </section>
  );
}
