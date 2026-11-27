// SPDX-License-Identifier: MPL-2.0

import "./status-tray.css";

import type { CastSending, CastToReceive } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useEffect, useRef, useState, type JSX, type Ref } from "react";
import { useTranslation } from "react-i18next";

import { useIsOnAirPlay } from "./airplay-here.ts";
import { browserStorage, tabStorage } from "./browser-storage.ts";
import { IconButton } from "./icon-button.tsx";
import { CastIcon, TogetherIcon, WarningIcon } from "./icons.tsx";
import { Link } from "./link.tsx";
import { hasCopyWarning } from "./copy-warning.ts";
import {
  useEndsTheCastSessionWithItsCast,
  useRejoinsTheCastSessionOfItsCast,
} from "./google-cast.ts";
import { useLive } from "./live.tsx";
import { readNeedsAttention, needsAttentionKey } from "./needs-attention-read.ts";
import { isAnythingNeeded } from "./attention-pages.ts";
import { routeHref } from "./router.ts";
import { useServices } from "./services.tsx";
import { useShellPart } from "./shell-read.ts";
import { closeTrayDialog, openTrayDialog, useTrayDialog } from "./tray-dialog.ts";
import { useMembership } from "./together-membership.ts";
import { TogetherDialog } from "./together-dialog-code.tsx";
import { CastDialog, useLoadsTrayDialogsAhead } from "./tray-dialog-code.tsx";
import { GrownFromOpener } from "./popover.tsx";
import { LaterScreen } from "./screen-load.tsx";
import { signMark } from "./sign-mark.tsx";
import { useResource } from "./use-resource.ts";
import { usePrefetchedHostedDetails } from "./hosted-details.ts";

/**
 * The status tray at the banner's end (D280), on every screen. Watch Together's and Cast's icons are
 * always in it, the entry points to both, each opening its dialog over the page (D384 §2), white while their function is off and blue, pulsing
 * gently, while it is on (D364 §12, §13); Watch Together's is shown to a person who may host it, and
 * to a signed-in guest in a session (§14). Cast's is on too on a device being cast to, saying from
 * which device (the owner's 11-20 batch, finding 5). For an admin, the warning sign leads to what needs them,
 * when anything does; it alone is yellow (D278).
 */
export function StatusTray(props: {
  readonly isAdmin: boolean;
  /** Whether the person's role may host Watch Together, whose icon then shows while it is off. */
  readonly canHostWatchTogether: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const isViewing = useMembership(browserStorage, "viewer") !== undefined;
  const { sending, receiving } = useLive();
  useEndsTheCastSessionWithItsCast(); // a Google Cast device's, its Cast menu closed once it took it
  useRejoinsTheCastSessionOfItsCast(); // Chrome's own Stop ends it again, the page loaded again or resumed
  useClosesTheDialogAsThePageMoves();
  useLoadsTrayDialogsAhead();
  return (
    <section className="status-tray icon-tray" aria-label={t("status.tray")}>
      {props.isAdmin ? <NeedsAttentionSign /> : null}
      {props.canHostWatchTogether || isHosting || isViewing ? (
        <TogetherSign isHosting={isHosting} isViewing={isViewing} />
      ) : null}
      <CastSign sending={sending} receiving={receiving} />
    </section>
  );
}

/**
 * Closes the tray's dialog as the page moves to another address, as by its own way back to a film:
 * the dialog belongs to the page it opened over. A step of the history that leaves the address as it
 * was, as a dialog opened from it closing, leaves it open.
 */
function useClosesTheDialogAsThePageMoves(): void {
  const { navigation } = useServices();
  useEffect(() => {
    let shown = routeHref(navigation.route());
    return navigation.subscribe(() => {
      const now = routeHref(navigation.route());
      if (now !== shown) {
        shown = now;
        closeTrayDialog();
      }
    });
  }, [navigation]);
}

/**
 * Watch Together's sign (D364 §13, D384 §2): it opens Watch Together's dialog over the page, to start
 * a session or run it, or, for a signed-in guest, to go back to the group's film, and never moves the
 * page; blue while a session is on. The hosted session's details are fetched as it shows, so its
 * dialog shows them at once (hosted-details.ts).
 */
function TogetherSign(props: {
  readonly isHosting: boolean;
  readonly isViewing: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  usePrefetchedHostedDetails();
  const isOn = props.isHosting || props.isViewing;
  const openerRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <IconButton
        ref={openerRef}
        label={t(isOn ? "status.togetherOn" : "status.together")}
        icon={<TogetherIcon />}
        mark={signMark(isOn)}
        onClick={() => {
          openTrayDialog("together");
        }}
      />
      {useTrayDialog() === "together" ? (
        <GrownFromOpener openerRef={openerRef} tab={<TogetherIcon />}>
          <LaterScreen resetKey="together-dialog">
            <TogetherDialog onClose={closeTrayDialog} />
          </LaterScreen>
        </GrownFromOpener>
      ) : null}
    </>
  );
}

/**
 * Cast's sign (D364 §13, D384 §2): it opens Cast's dialog over the page, to choose a device or stop,
 * and never moves the page; blue while a cast is on, named by its receiver and what plays, or, on the
 * device cast to, by the device casting. While a film plays on the receiver it goes to that film's
 * Play page instead, the cast's remote, whose Cast sign over the film opens the dialog (the owner,
 * 2026-11-23: "the cast button in the top bar no longer shows a submenu, but rather brings us back to
 * the casting page"); with nothing on there, the dialog, to stop or choose.
 */
function CastSign(props: {
  readonly sending: CastSending | undefined;
  readonly receiving: CastToReceive | undefined;
}): JSX.Element {
  const openerRef = useRef<HTMLButtonElement>(null);
  const toTheRemote = useToTheRemote(props.sending);
  return (
    <>
      <CastSignButton
        openerRef={openerRef}
        sending={props.sending}
        receiving={props.receiving}
        isOverVideo={false}
        onOpen={() => {
          if (toTheRemote?.() !== true) {
            openTrayDialog("cast");
          }
        }}
      />
      {useTrayDialog() === "cast" ? (
        <GrownFromOpener openerRef={openerRef} tab={<CastIcon />}>
          <CastDialogLoaded onClose={closeTrayDialog} />
        </GrownFromOpener>
      ) : null}
    </>
  );
}

/**
 * The way to the remote of the cast this device sends, the Play page of the film on its receiver,
 * which is the cast's remote (cast-to.tsx's PlayOrSendOn), true where it went there; false where the
 * page shown is it already, whose Cast is the dialog, Stop casting in it. Undefined while nothing
 * plays there, or this device sends no cast.
 */
function useToTheRemote(sending: CastSending | undefined): (() => boolean) | undefined {
  const { navigation } = useServices();
  if (sending === undefined || sending.fileId === 0) {
    return undefined;
  }
  const { fileId } = sending;
  return () => {
    const shown = navigation.route();
    if (shown.screen === "play" && shown.fileId === fileId) {
      return false;
    }
    navigation.go({ screen: "play", fileId });
    return true;
  };
}

/**
 * Cast's sign over the playing film, at the end of the player's top row (D422): the
 * film fills the window and hides the page, the tray with it (D418 §1), so a cast is started from
 * here. Its dialog is drawn inside the player, where the hidden page's would not show; while it is
 * open, onOpenChange holds the overlay shown and gives the dialog its presses, as the menus do.
 */
export function CastSignOverTheFilm(props: {
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { sending, receiving } = useLive();
  const [isOpen, setIsOpen] = useState(false);
  const openerRef = useRef<HTMLButtonElement>(null);
  const { onOpenChange } = props;
  const openOrClose = (isNowOpen: boolean): void => {
    setIsOpen(isNowOpen);
    onOpenChange(isNowOpen);
  };
  return (
    <span className="player-overlay__cast">
      <CastSignButton
        openerRef={openerRef}
        sending={sending}
        receiving={receiving}
        isOverVideo
        onOpen={() => {
          openOrClose(true);
        }}
      />
      {isOpen ? (
        <GrownFromOpener openerRef={openerRef} tab={<CastIcon />}>
          <CastDialogLoaded
            onClose={() => {
              openOrClose(false);
            }}
          />
        </GrownFromOpener>
      ) : null}
    </span>
  );
}

/** Cast's sign over the film, as the signed-in app gives it to the overlay (CastSignOverTheFilmContext). */
export function castSignOverTheFilm(onOpenChange: (isOpen: boolean) => void): JSX.Element {
  return <CastSignOverTheFilm onOpenChange={onOpenChange} />;
}

/**
 * Cast's sign itself: Cast to a device, or, blue, what plays on which device while this one casts, or
 * which device casts to this one while it receives (a device that does both says what it casts); blue
 * too while this tab's film plays on AirPlay, saying so (D523 §6).
 */
function CastSignButton(props: {
  /** The button, which the dialog over the film grows out of (GrownFromContext). */
  readonly openerRef?: Ref<HTMLButtonElement> | undefined;
  readonly sending: CastSending | undefined;
  readonly receiving: CastToReceive | undefined;
  readonly isOverVideo: boolean;
  readonly onOpen: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { openerRef, sending, receiving, isOverVideo, onOpen } = props;
  const isOnAirPlay = useIsOnAirPlay();
  return (
    <IconButton
      ref={openerRef}
      label={isOnAirPlay ? t("status.onAirPlay") : castSignLabel(t, sending, receiving)}
      icon={<CastIcon />}
      mark={signMark(isOnAirPlay || sending !== undefined || receiving !== undefined)}
      isOverVideo={isOverVideo}
      onClick={onOpen}
    />
  );
}

/** What Cast's sign says: what this device casts, else which device casts to it, else Cast to a device. */
function castSignLabel(
  t: TFunction,
  sending: CastSending | undefined,
  receiving: CastToReceive | undefined,
): string {
  if (sending !== undefined) {
    return t(sending.fileId === 0 ? "status.castingReady" : "status.castingFilm", {
      name: sending.receiverName,
      film: sending.itemName,
    });
  }
  if (receiving === undefined) {
    return t("status.cast");
  }
  return receiving.fromDeviceName === ""
    ? t("status.castFromAnother")
    : t("status.castFrom", { name: receiving.fromDeviceName });
}

/** Cast's dialog, its code loaded apart (tray-dialog-code.tsx). */
function CastDialogLoaded(props: { readonly onClose: () => void }): JSX.Element {
  return (
    <LaterScreen resetKey="cast-dialog">
      <CastDialog onClose={props.onClose} />
    </LaterScreen>
  );
}

/**
 * For an admin, the warning sign when anything needs them, leading to the page that lists it: the
 * shell's read says it first (D476). Failures of unknown cause piling up light it too, with no banner
 * of their own (D517 §2).
 */
function NeedsAttentionSign(): JSX.Element | null {
  const { api } = useServices();
  const [counts] = useResource(needsAttentionKey, async () => await readNeedsAttention(api), {
    given: useShellPart((shell) => shell.needsAttention),
  });
  if (counts.state !== "loaded") {
    return null;
  }
  // Said by page (D538 §12), the sign counts from the pages; a server before them, from its counts.
  if (counts.value.pages !== undefined) {
    return isAnythingNeeded(counts.value.pages) ? <NeedsAttentionLink /> : null;
  }
  const {
    rootCount,
    diskCount,
    waitingRootCount,
    unreadCount,
    matchCount,
    seriesReadWronglyCount,
    filmsWithoutSoundCount,
    rescansGivenUp,
    unknownFailureAlertCount,
  } = counts.value;
  if (
    rootCount +
      diskCount +
      waitingRootCount +
      unreadCount +
      matchCount +
      seriesReadWronglyCount +
      filmsWithoutSoundCount +
      (rescansGivenUp?.length ?? 0) +
      unknownFailureAlertCount ===
      0 &&
    !hasCopyWarning(counts.value)
  ) {
    return null;
  }
  return <NeedsAttentionLink />;
}

/** The warning sign itself, leading to the page that lists what needs the admin. */
function NeedsAttentionLink(): JSX.Element {
  const { t } = useTranslation();
  return (
    <Link
      to={{ screen: "needs-attention" }}
      className="icon-button status-tray__button status-tray__button--warning"
      label={t("status.needsAttention")}
    >
      <WarningIcon />
    </Link>
  );
}
