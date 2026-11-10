// SPDX-License-Identifier: MPL-2.0

import "./system-words.ts";

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { WarningIcon } from "./warning-sign.tsx";
import { BusyGroup } from "./busy-group.tsx";
import { dateTimeText } from "./date-time-text.ts";
import { idempotent } from "./idempotent.ts";
import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

type Waiting = components["schemas"]["WaitingBody"];
type Unread = components["schemas"]["UnreadBody"];
type SystemNode = components["schemas"]["SystemNodeBody"];
type SystemReport = components["schemas"]["SystemOutputBody"];

/** What a report that cannot be read again does when something changed. */
const noChange = (): void => undefined;

/** What waits for the admin on the System page: removals a scan holds back, and what it could not read. */
export function WhatWaits(props: {
  readonly report: SystemReport;
  /** Reads the report again once a removal is asked for; nothing when the report cannot be. */
  readonly onChanged?: (() => void) | undefined;
}): JSX.Element {
  return (
    <>
      <WaitingRemovals
        waiting={props.report.waiting ?? []}
        onChanged={props.onChanged ?? noChange}
      />
      <UnreadPaths unread={props.report.unread ?? []} nodes={props.report.nodes ?? []} />
    </>
  );
}

/**
 * The library folders whose scans wait for the admin before removing files (D208 §3, D277 §2): for
 * each, why, since when, how many files and the first of them, and the button that removes them,
 * which scans the folder again. What the scan saw plays meanwhile.
 */
function WaitingRemovals(props: {
  readonly waiting: readonly Waiting[];
  readonly onChanged: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const removing = useRequest((cause) => problemText(t, cause));
  if (props.waiting.length === 0) {
    return null;
  }
  const remove = (root: Waiting): void => {
    removing.runRequest(async (idempotencyKey) => {
      throwIfRefused(
        await api.POST("/libraries/{id}/roots/removals", {
          params: { path: { id: root.libraryId } },
          body: { nodeName: root.nodeName, path: root.rootPath },
          ...idempotent(idempotencyKey),
        }),
      );
      props.onChanged();
    });
  };
  return (
    <section aria-labelledby="system-waiting">
      <h2 id="system-waiting">{t("system.waiting")}</h2>
      <p>{t("system.waitingExplain")}</p>
      {removing.problem === undefined ? null : <p role="alert">{removing.problem}</p>}
      <BusyGroup isBusy={removing.isBusy}>
        <ul>
          {props.waiting.map((root) => (
            <WaitingRoot
              key={`${root.nodeName}:${root.rootPath}`}
              root={root}
              onRemove={() => {
                remove(root);
              }}
            />
          ))}
        </ul>
      </BusyGroup>
    </section>
  );
}

/**
 * What scans could not read (D163 §8, D277 §3): each with its library, the platform's error, its
 * owner, group and mode, and the user and group its node reads as, so the admin can make it
 * readable to them.
 */
function UnreadPaths(props: {
  readonly unread: readonly Unread[];
  readonly nodes: readonly SystemNode[];
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.unread.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="system-unread">
      <h2 id="system-unread">{t("system.unread")}</h2>
      <ul>
        {props.unread.map((path) => {
          const node = props.nodes.find((each) => each.name === path.nodeName);
          const detail = path.errorDetail;
          return (
            <li key={`${path.nodeName}:${path.path}`}>
              <p>
                <WarningIcon />
                <code>{path.path}</code>{" "}
                {t("system.unreadWhere", { library: path.libraryName, node: path.nodeName })}{" "}
                {t("system.unreadWhy", {
                  text: detail.text,
                  owner: detail.ownerUid,
                  group: detail.groupGid,
                  mode: detail.modeText,
                })}
              </p>
              <p>
                {node === undefined
                  ? t("system.unreadFixUnknownNode")
                  : t("system.unreadFix", { uid: node.uid, gid: node.gid })}
              </p>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

/** One folder whose removals wait: why, since when, the first of the files, and removing them. */
function WaitingRoot(props: {
  readonly root: Waiting;
  readonly onRemove: () => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { root } = props;
  const named = root.firstPaths ?? [];
  return (
    <li>
      <p>
        <WarningIcon />
        {t("system.waitingRoot", {
          node: root.nodeName,
          path: root.rootPath,
          library: root.libraryName,
          count: root.fileCount,
          since: dateTimeText(root.waitingAt, i18n.language),
          reason: root.reason,
        })}
      </p>
      <ul>
        {named.map((path) => (
          <li key={path}>
            <code>{path}</code>
          </li>
        ))}
      </ul>
      {root.fileCount > named.length ? (
        <p>{t("system.waitingMore", { count: root.fileCount - named.length })}</p>
      ) : null}
      <ConfirmButton
        label={t("system.waitingRemove", { count: root.fileCount })}
        consequence={t("system.waitingRemoveConsequence", { count: root.fileCount })}
        confirmLabel={t("system.waitingRemoveConfirm")}
        onConfirm={props.onRemove}
      />
    </li>
  );
}
