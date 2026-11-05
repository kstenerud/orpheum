// SPDX-License-Identifier: MPL-2.0

import "./cast.css";

import type { CastSending } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { CastTargets } from "./cast-to.tsx";
import { endCastRetrying } from "./cast.ts";
import { useLive } from "./live.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { isTheGroupsFilmShown, positionPlayedHere } from "./playing-here.ts";
import { problemText } from "./problem.ts";
import { useRoute, useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

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
  const film = (): { fileId: number; positionMs: number | undefined } | undefined =>
    fileId === undefined || isTheGroupsFilmShown()
      ? undefined
      : { fileId, positionMs: positionPlayedHere(fileId) };
  const taken = sending?.isAccepted === true ? sending : undefined;
  useClosesOnceTaken(taken?.castId, props.onClose);
  return (
    <ModalDialog label={t("cast.page")} onClose={props.onClose}>
      {!isSendingKnown ? <p>{t("common.loading")}</p> : null}
      {isSendingKnown && taken === undefined ? <CastTargets film={film} /> : null}
      {taken === undefined ? null : (
        <CastOn sent={taken} isOnItsPage={fileId === taken.fileId} onClose={props.onClose} />
      )}
    </ModalDialog>
  );
}

/**
 * Closes the dialog once a cast started from it is taken, its receiver having said Use: the page
 * under it is the cast's remote then, which the dialog would cover. A cast on as it opened leaves it
 * open, to be stopped or gone back to.
 */
function useClosesOnceTaken(takenCastId: number | undefined, onClose: () => void): void {
  const [openedWith] = useState(takenCastId);
  const closeRef = useRef(onClose);
  useEffect(() => {
    closeRef.current = onClose;
  });
  useEffect(() => {
    if (takenCastId !== undefined && takenCastId !== openedWith) {
      closeRef.current();
    }
  }, [takenCastId, openedWith]);
}

/**
 * The cast this device sends, in Cast's dialog: what plays on which device, the way to its film's
 * page where that is not the page shown, and Stop casting, which brings the film back to that page
 * where it is shown (play.tsx's PlayOrSendOn). Stop is tried once more where its first try fails, and
 * closes the dialog once the cast is over; failing twice, it says why and the dialog stays, to try
 * again (principles §28: it hid the failure, the cast going on as the person took it for stopped).
 */
function CastOn(props: {
  readonly sent: CastSending;
  readonly isOnItsPage: boolean;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const { sent, onClose } = props;
  const name = sent.receiverName;
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  return (
    <section className="cast-on" aria-labelledby="cast-on-heading">
      <h2 id="cast-on-heading">
        {sent.fileId === 0
          ? t("cast.readyOn", { name })
          : t("cast.playingFilmOn", {
              film: sent.itemName === "" ? t("cast.aFilm") : sent.itemName,
              name,
            })}
      </h2>
      <div className="cast-on__buttons">
        {sent.fileId === 0 || props.isOnItsPage ? null : (
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
        )}
        <button
          type="button"
          disabled={isBusy}
          onClick={() => {
            runRequest(async () => {
              await endCastRetrying(api, sent.castId, "stopped");
              onClose();
            });
          }}
        >
          {t("cast.stop")}
        </button>
      </div>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </section>
  );
}
