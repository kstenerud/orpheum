// SPDX-License-Identifier: MPL-2.0

/**
 * The ended cast remote's Cast again (D553 §2), apart from the remote's page (cast-remote.tsx): the
 * film cast anew to the receiver the ended cast had, once it is back among the person's devices.
 */

import "./cast-words.ts";

import type { CastSending } from "@orpheum/sdk";
import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";

import { startCast } from "./cast.ts";
import { useLive } from "./live.tsx";
import { problemText } from "./problem.ts";
import type { ReopenAt } from "./reopen.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/** Cast again as the ended remote offers it: its press, whether it is under way, and why it failed. */
export interface CastAgain {
  readonly press: () => void;
  readonly isBusy: boolean;
  readonly problem: string | undefined;
}

/**
 * Cast again for the ended cast sent (D553 §2): offered once its receiver's device is among the
 * person's connected devices again, as the live connection says them (the remote's own reading of a
 * receiver heard, useReceiverHeard), and undefined while it is not, or for nothing on. It casts the
 * cast's film, as the title itemId, to that device from where Play here would start (from), paused
 * as it stood, the way Cast's dialog starts a cast (startCast): the Play page shows the new cast's
 * remote as its receiver takes it (cast-to.tsx's PlayOrSendOn), and where the cast had moved on from
 * the page's film, pageFileId, the page goes to its film's page, which does. A Chromecast (a guest
 * receiver, in no devices message) is offered none: Chrome's picker, which alone casts to one, names
 * no device the page could find again (D553 §2).
 */
export function useCastAgain(
  sent: CastSending,
  film: { readonly pageFileId: number; readonly itemId: number | undefined },
  from: () => ReopenAt | undefined,
): CastAgain | undefined {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const { connectedDeviceIds, sending } = useLive();
  const startedRef = useRef<number | undefined>(undefined);
  const name = sent.receiverName;
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    problemText(t, cause, {
      conflict: t("cast.notInFront", { name }),
      receivingACast: t("cast.receivingAnother", { name }),
      castingHere: t("cast.castingHere", { name }),
    }),
  );
  const { fileId } = sent;
  const isTaken =
    sending?.castId !== undefined && sending.castId === startedRef.current && sending.isAccepted;
  useEffect(() => {
    if (isTaken && fileId !== film.pageFileId) {
      navigation.replace({ screen: "play", fileId }); // its page shows the new cast's remote
    }
  }, [isTaken, fileId, film.pageFileId, navigation]);
  const isBack = !sent.isGuestReceiver && connectedDeviceIds.includes(sent.receiverDeviceId);
  if (!isBack || fileId === 0) {
    return undefined;
  }
  return {
    isBusy,
    problem,
    press: () => {
      runRequest(async () => {
        const at = from();
        const cast = await startCast(api, sent.receiverDeviceId, {
          fileId,
          itemId: film.itemId,
          positionMs: at?.positionMs,
          isPaused: at?.isPaused,
        });
        startedRef.current = cast.castId;
      });
    },
  };
}
