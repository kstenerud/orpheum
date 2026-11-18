// SPDX-License-Identifier: MPL-2.0

import "./cast.css";
import "./cast-words.ts";

import type { TogetherSnapshot } from "@orpheum/sdk";
import { useEffect, useMemo, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ChromecastEntry, useGuestCastAskingBy } from "./cast-to.tsx";
import type { CastChannel } from "./cast-sdk.ts";
import {
  askTogetherCastGrantOf,
  endQuietly,
  GuestCastFailure,
  openGuestReceiverBy,
} from "./google-cast.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import type { Membership } from "./together-storage.ts";
import { useRequest } from "./use-request.ts";

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
 */

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
  };
}

/**
 * The guest's Cast, on their page's panel: while no Chromecast takes their part, Cast, which opens a
 * dialog of the one choice, Chromecast, saying there why a cast failed; while one does, which it is
 * and Stop casting. Chromecast waits while the session is not connected, as the Cast menu's does
 * while the live connection is down.
 */
export function GuestsCastEntry(props: {
  readonly cast: GuestsCast;
  readonly snapshot: TogetherSnapshot | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { cast, snapshot } = props;
  if (!cast.isOffered) {
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
        <ModalDialog
          label={t("cast.page")}
          onClose={() => {
            cast.setIsChoosing(false);
          }}
        >
          <section className="cast-to" aria-labelledby="guest-cast-heading">
            <h2 id="guest-cast-heading">{t("cast.heading")}</h2>
            <ChromecastEntry
              isStale={snapshot?.status !== "connected" || cast.isBusy}
              onCast={cast.start}
            />
            {cast.problem === undefined ? null : <p role="alert">{cast.problem}</p>}
          </section>
        </ModalDialog>
      ) : null}
    </>
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
