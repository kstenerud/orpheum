// SPDX-License-Identifier: MPL-2.0

import "./system-words.ts";

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
 * it and when. The leave is not taken back: an admin who no longer wants the machine to run stops it
 * (the owner, 2026-11-08).
 */
export function NodeUnconfinedLeave(props: {
  readonly node: SystemNode;
  /** The leave was given: read the report again. */
  readonly onChanged?: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { node } = props;
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const allow = (): void => {
    runRequest(async () => {
      throwIfRefused(
        await api.PUT("/nodes/{name}/unconfined", { params: { path: { name: node.name } } }),
      );
      props.onChanged?.();
    });
  };
  const leave = node.unconfinedLeave;
  if (leave !== undefined) {
    return (
      <p className="needs-fixing">
        <output>
          {t("system.unconfined.given", {
            by: leave.acceptedBy,
            at: dateTimeText(leave.acceptedAt, i18n.language),
          })}
        </output>
      </p>
    );
  }
  if (node.connection !== "locked") {
    return null;
  }
  return (
    <LockedNode
      node={node}
      isBusy={isBusy}
      alert={problem === undefined ? null : <p role="alert">{problem}</p>}
      onAllow={allow}
    />
  );
}

/** A locked machine: what it lacks, the risk, and the leave to run it unconfined, confirmed first. */
function LockedNode(props: {
  readonly node: SystemNode;
  readonly isBusy: boolean;
  readonly alert: JSX.Element | null;
  readonly onAllow: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div>
      <p className="needs-fixing">
        <output>{t("system.unconfined.locked")}</output>
      </p>
      <NodeConfinementView node={props.node} />
      <p>{t("system.unconfined.risk")}</p>
      <ConfirmButton
        label={t("system.unconfined.allow")}
        isDisabled={props.isBusy}
        consequence={t("system.unconfined.consequence")}
        confirmLabel={t("system.unconfined.allowConfirm")}
        onConfirm={props.onAllow}
      />
      {props.alert}
    </div>
  );
}
