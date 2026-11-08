// SPDX-License-Identifier: MPL-2.0

import "./stop-casting-here.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { endCastRetrying } from "./cast.ts";
import { dangerousMark } from "./focus-navigation.ts";
import { useLive } from "./live.tsx";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/**
 * A cast's receiver's own end of it, Stop casting here (D446 §1, §3): it heads the ready page and the
 * player's overlay, the first row, reached by Up, and focus never starts on it. Pressed, the cast ends
 * as takenBack: its end, told to both devices, sends this one home and has the device that cast take
 * on the film as it stood here (D446 §2). Nothing on a page that receives no cast.
 */
export function StopCastingHere(): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { receiving } = useLive();
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  if (receiving === undefined) {
    return null;
  }
  const { castId } = receiving;
  return (
    <div className="stop-casting-here">
      <button
        type="button"
        className="stop-casting-here__button"
        {...dangerousMark}
        disabled={isBusy}
        onClick={() => {
          runRequest(async () => {
            await endCastRetrying(api, castId, "takenBack");
          });
        }}
      >
        {t("cast.stopHere")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </div>
  );
}
