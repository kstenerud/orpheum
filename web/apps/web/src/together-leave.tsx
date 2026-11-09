// SPDX-License-Identifier: MPL-2.0

import type { TogetherEndReason } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { idempotent } from "./idempotent.ts";
import { ApiProblem, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { membershipHeaders, type Membership } from "./together-storage.ts";
import { useRequest } from "./use-request.ts";

/** A guest's Leave: the membership ends, and with it every device of it. */
export function LeaveButton(props: {
  readonly membership: Membership;
  readonly onLeft: (reason: TogetherEndReason) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  return (
    <>
      <button
        type="button"
        disabled={isBusy}
        onClick={() => {
          runRequest(async (idempotencyKey) => {
            const answer = await api.POST("/together/leave", {
              headers: {
                ...idempotent(idempotencyKey).headers,
                ...membershipHeaders(props.membership.credential),
              },
            });
            try {
              throwIfRefused(answer);
            } catch (cause: unknown) {
              // A session the server no longer has, as after its restart, is over, not a failure.
              if (cause instanceof ApiProblem && cause.code === "notFound") {
                props.onLeft("gone");
                return;
              }
              throw cause;
            }
            props.onLeft("left");
          });
        }}
      >
        {t("together.leave")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
}
