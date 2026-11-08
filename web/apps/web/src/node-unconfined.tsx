// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { NodeConfinementView } from "./node-confinement.tsx";
import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

type SystemNode = components["schemas"]["SystemNodeBody"];

/**
 * A machine whose kernel lacks part of FFmpeg's sandbox, which waits locked (D450 §1): the risk in
 * plain words, and an admin's explicit leave to run it without the sandbox; and, once given, who gave
 * it and when, and withdrawing it.
 */
export function NodeUnconfinedLeave(props: {
  readonly node: SystemNode;
  /** The leave was given or withdrawn: read the report again. */
  readonly onChanged?: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { node } = props;
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const change = (isGiving: boolean): void => {
    runRequest(async () => {
      const params = { params: { path: { name: node.name } } };
      throwIfRefused(
        isGiving
          ? await api.PUT("/nodes/{name}/unconfined", params)
          : await api.DELETE("/nodes/{name}/unconfined", params),
      );
      props.onChanged?.();
    });
  };
  const alert = problem === undefined ? null : <p role="alert">{problem}</p>;
  const leave = node.unconfinedLeave;
  if (leave !== undefined) {
    return (
      <div>
        <p role="alert">
          {t("system.unconfined.given", {
            by: leave.acceptedBy,
            at: dateTimeText(leave.acceptedAt, i18n.language),
          })}
        </p>
        <button
          type="button"
          disabled={isBusy}
          onClick={() => {
            change(false);
          }}
        >
          {t("system.unconfined.withdraw")}
        </button>
        {alert}
      </div>
    );
  }
  if (node.connection !== "locked") {
    return null;
  }
  return (
    <div>
      <p role="alert">{t("system.unconfined.locked")}</p>
      <NodeConfinementView node={node} />
      <p>{t("system.unconfined.risk")}</p>
      <ConfirmButton
        label={t("system.unconfined.allow")}
        isDisabled={isBusy}
        consequence={t("system.unconfined.consequence")}
        confirmLabel={t("system.unconfined.allowConfirm")}
        onConfirm={() => {
          change(true);
        }}
      />
      {alert}
    </div>
  );
}
