// SPDX-License-Identifier: MPL-2.0

import "./stop-casting-here.css";
import "./cast-words.ts";

import type { JSX, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { endCastRetrying, type CastEnd } from "./cast.ts";
import { useLive } from "./live.tsx";
import { ActionPill } from "./pill.tsx";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { StatusNote } from "./status-note.tsx";

/** A cast's end as a press asks for it (useCastEnding). */
export interface CastEnding {
  /** Whether its end is asked for now. */
  readonly isBusy: boolean;
  /** Why the last ask failed, twice tried; undefined where none has. */
  readonly problem: string | undefined;
  /** Asks the cast's end, tried once more where the first try fails, then onEnded. */
  readonly end: () => void;
}

/**
 * The one way a press ends a cast, castId, as reason (D446 §1): the sender's Stop casting, on the
 * remote and in Cast's dialog, and the receiver's Stop casting here. Asked once more where its first
 * try fails (endCastRetrying); failing twice, it says why, to try again (principles §28).
 */
export function useCastEnding(castId: number, reason: CastEnd, onEnded?: () => void): CastEnding {
  const { t } = useTranslation();
  const { api } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  return {
    isBusy,
    problem,
    end: () => {
      runRequest(async () => {
        await endCastRetrying(api, castId, reason);
        onEnded?.();
      });
    },
  };
}

/**
 * A cast's end as an action Pill (D538 §1), dangerous, disabled while its end is asked for, why it
 * failed under it: the receiver's Stop casting here (StopCastingHere).
 */
function StopCastingPill(props: {
  readonly castId: number;
  readonly reason: CastEnd;
  readonly children: ReactNode;
}): JSX.Element {
  const ending = useCastEnding(props.castId, props.reason);
  return (
    <div className="stop-casting-here">
      <ActionPill isDangerous isDisabled={ending.isBusy} onPress={ending.end}>
        {props.children}
      </ActionPill>
      {ending.problem === undefined ? null : (
        <StatusNote told={{ kind: "problem", text: ending.problem }} />
      )}
    </div>
  );
}

/**
 * A cast's receiver's own end of it, Stop casting here (D446 §1, §3): it heads the ready page and the
 * player's overlay, the first row, reached by Up, and focus never starts on it. Pressed, the cast ends
 * as takenBack: its end, told to both devices, sends this one home and has the device that cast take
 * on the film as it stood here (D446 §2). Nothing on a page that receives no cast.
 */
export function StopCastingHere(): JSX.Element | null {
  const { t } = useTranslation();
  const { receiving } = useLive();
  if (receiving === undefined) {
    return null;
  }
  return (
    <StopCastingPill castId={receiving.castId} reason="takenBack">
      {t("cast.stopHere")}
    </StopCastingPill>
  );
}
