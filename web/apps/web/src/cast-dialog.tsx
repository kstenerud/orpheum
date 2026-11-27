// SPDX-License-Identifier: MPL-2.0

import "./cast.css";
import "./cast-words.ts";

import type { CastSending } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { CastTargets } from "./cast-to.tsx";
import { arrivalMark } from "./focus-navigation.ts";
import type { CastFilm } from "./cast.ts";
import { useLatest } from "./latest.ts";
import { useLive } from "./live.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { isPausedHere, isTheGroupsFilmShown, positionPlayedHere } from "./playing-here.ts";
import { useRoute, useServices } from "./services.tsx";
import { StatusNote } from "./status-note.tsx";
import { useCastEnding } from "./stop-casting-here.tsx";

/**
 * Cast's dialog, which the tray's Cast opens over whatever page is shown and which never moves it
 * (D384 §2): with no cast, the devices to cast to, a cast started there moving the film of the Play
 * page it opened over to the receiver at its position (D364 §1), but a guest's group's film, which
 * goes with no film, the receiver following the group in the guest's part (D371 §4); with one, what
 * plays where, the way back to its film's page, whose video's area is the cast's remote, and Stop
 * casting.
 */
export function CastDialog(props: { readonly onClose: () => void }): JSX.Element {
  const { t } = useTranslation();
  const route = useRoute();
  const { sending, isSendingKnown } = useLive();
  const fileId = route.screen === "play" ? route.fileId : undefined;
  const itemId = route.screen === "play" ? route.itemId : undefined; // what it plays as (B0 question 20)
  const film = (): CastFilm | undefined =>
    fileId === undefined || isTheGroupsFilmShown()
      ? undefined
      : { fileId, itemId, positionMs: positionPlayedHere(fileId), isPaused: isPausedHere(fileId) };
  const taken = sending?.isAccepted === true ? sending : undefined;
  useClosesOnceTaken(taken?.castId, props.onClose);
  if (taken !== undefined) {
    return <CastOn sent={taken} isOnItsPage={fileId === taken.fileId} onClose={props.onClose} />;
  }
  return (
    <ModalDialog label={t("cast.page")} heading={t("cast.page")} onClose={props.onClose}>
      {isSendingKnown ? (
        <CastTargets
          film={film}
          onAirPlayPressed={props.onClose}
          // Over a film's page, which says the connect from the pick, with its Cancel (D570 §2).
          onChromecastPicked={fileId === undefined ? undefined : props.onClose}
        />
      ) : (
        <p>{t("common.loading")}</p>
      )}
    </ModalDialog>
  );
}

/**
 * Closes the dialog once a cast started from it is taken by its receiver (D468 §1): the page
 * under it is the cast's remote then, which the dialog would cover. A cast on as it opened leaves it
 * open, to be stopped or gone back to.
 */
function useClosesOnceTaken(takenCastId: number | undefined, onClose: () => void): void {
  const [openedWith] = useState(takenCastId);
  const closeRef = useLatest(onClose);
  useEffect(() => {
    if (takenCastId !== undefined && takenCastId !== openedWith) {
      closeRef.current();
    }
  }, [takenCastId, openedWith, closeRef]);
}

/**
 * The cast this device sends, in Cast's dialog: on which device it plays, never what plays (the
 * owner, 2026-11-23: "only where we're casting to"), and the way to its film's page where that is
 * not the page shown; at its foot Stop casting alone, its main action and where focus arrives
 * (dialogs-spec), with no Cancel (the owner: "We don't need a cancel button here"; Escape and a
 * press outside still close it), which brings the film back to that page where it is shown
 * (play.tsx's PlayOrSendOn). Stop is tried once more where its first try fails, and closes the
 * dialog once the cast is over; failing twice, it says why and the dialog stays, to try again
 * (principles §28: it hid the failure, the cast going on as the person took it for stopped).
 */
function CastOn(props: {
  readonly sent: CastSending;
  readonly isOnItsPage: boolean;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const { sent, onClose } = props;
  const name = sent.receiverName;
  const { isBusy, problem, end } = useCastEnding(sent.castId, "stopped", onClose);
  return (
    <ModalDialog
      label={t("cast.page")}
      heading={t("cast.page")}
      onClose={onClose}
      footer={
        <button type="button" disabled={isBusy} {...arrivalMark} onClick={end}>
          {t("cast.stop")}
        </button>
      }
    >
      <section className="cast-on" aria-labelledby="cast-on-heading">
        <h3 id="cast-on-heading">
          {sent.fileId === 0 ? t("cast.readyOn", { name }) : t("cast.playingOn", { name })}
        </h3>
        {sent.fileId === 0 || props.isOnItsPage ? null : (
          <div className="cast-on__buttons">
            <button
              type="button"
              className="title__play"
              onClick={() => {
                onClose();
                navigation.go({ screen: "play", fileId: sent.fileId });
              }}
            >
              {t("cast.backToTheFilm")}
            </button>
          </div>
        )}
        {problem === undefined ? null : <StatusNote told={{ kind: "problem", text: problem }} />}
      </section>
    </ModalDialog>
  );
}
