// SPDX-License-Identifier: MPL-2.0

import "./system-words.ts";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

type QueueKind = components["schemas"]["QueueKindBody"];
type NodeWork = components["schemas"]["NodeWorkBody"];
type NodeDevice = components["schemas"]["NodeDeviceBody"];

/**
 * How the work queue is doing, by kind of job (D233 §8): how many wait, since when the oldest
 * has, and how many failed in the last day. Nothing while the queue is empty.
 */
export function QueueHealth(props: { readonly queue: readonly QueueKind[] }): JSX.Element | null {
  const { t } = useTranslation();
  if (props.queue.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="system-queue">
      <h2 id="system-queue">{t("system.queue.heading")}</h2>
      <ul>
        {props.queue.map((kind) => (
          <li key={kind.kind}>
            <strong>{t(`jobs.kind.${camelKey(kind.kind)}`)}</strong>{" "}
            {t("system.queue.waiting", { count: kind.queuedCount })}
            {kind.oldestQueuedAt === undefined
              ? null
              : ` ${t("system.queue.oldest", { at: dateTimeText(kind.oldestQueuedAt, i18n.language) })}`}
            {kind.failedLastDayCount === 0
              ? null
              : ` ${t("system.queue.failed", { count: kind.failedLastDayCount })}`}
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * What a machine is doing (D233 §8): its slots running work, the work waiting for one, when what
 * runs last moved, and the faults the server saw in what it holds (D234 §4).
 */
export function NodeWorkView(props: { readonly work: NodeWork | undefined }): JSX.Element | null {
  const { t } = useTranslation();
  const { work } = props;
  if (work === undefined) {
    return null;
  }
  return (
    <p>
      {t("system.work.running", { running: work.runningCount, slots: work.slotCount })}{" "}
      {t("system.work.waiting", { count: work.waitingCount })}
      {work.lastMovedAt === undefined
        ? null
        : ` ${t("system.work.lastMoved", { at: dateTimeText(work.lastMovedAt, i18n.language) })}`}
      {work.brokenInvariantCount === 0 ? null : (
        <output> {t("system.work.broken", { count: work.brokenInvariantCount })}</output>
      )}
    </p>
  );
}

/**
 * The disks a machine reads from, each with how many may read it at once, which an admin may set
 * for one disk or give back to its kind's setting (D240 §5).
 */
export function NodeDevices(props: {
  readonly nodeName: string;
  readonly devices: readonly NodeDevice[];
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.devices.length === 0) {
    return null;
  }
  return (
    <section aria-label={t("system.devices.heading")}>
      <h3>{t("system.devices.heading")}</h3>
      <ul>
        {props.devices.map((device) => (
          <li key={device.key}>
            <DeviceReaders nodeName={props.nodeName} device={device} />
          </li>
        ))}
      </ul>
    </section>
  );
}

/** One disk, its readers at once, and setting them. */
function DeviceReaders(props: {
  readonly nodeName: string;
  readonly device: NodeDevice;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { device } = props;
  const [shown, setShown] = useState({
    limit: device.readerLimit,
    isSet: device.isReaderLimitSet,
  });
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const setLimit = (readerLimit: number | undefined): void => {
    runRequest(async () => {
      const name = { name: props.nodeName };
      throwIfRefused(
        readerLimit === undefined
          ? await api.DELETE("/nodes/{name}/reader-limit", {
              params: { path: name, query: { deviceKey: device.key } },
            })
          : await api.PUT("/nodes/{name}/reader-limit", {
              params: { path: name },
              body: { deviceKey: device.key, readerLimit },
            }),
      );
      setShown({ limit: readerLimit ?? shown.limit, isSet: readerLimit !== undefined });
    });
  };
  return (
    <>
      <strong>{device.key}</strong> {t(`system.devices.kind.${device.kind}`)}{" "}
      {t(shown.isSet ? "system.devices.readersSet" : "system.devices.readersOfKind", {
        count: shown.limit,
      })}{" "}
      <ReaderLimitControl
        initial={device.readerLimit}
        isSet={shown.isSet}
        isBusy={isBusy}
        onSet={setLimit}
      />
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
}

/** How many may read a disk at once, typed and set, or given back to its kind's setting. */
function ReaderLimitControl(props: {
  readonly initial: number;
  readonly isSet: boolean;
  readonly isBusy: boolean;
  readonly onSet: (readerLimit: number | undefined) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(String(props.initial));
  return (
    <>
      <label>
        {t("system.devices.readersLabel")}{" "}
        <input
          type="number"
          min={1}
          max={64}
          value={draft}
          onChange={(event) => {
            setDraft(event.currentTarget.value);
          }}
        />
      </label>{" "}
      <button
        type="button"
        disabled={props.isBusy}
        onClick={() => {
          props.onSet(Number(draft));
        }}
      >
        {t("system.devices.set")}
      </button>
      {props.isSet ? (
        <>
          {" "}
          <button
            type="button"
            disabled={props.isBusy}
            onClick={() => {
              props.onSet(undefined);
            }}
          >
            {t("system.devices.useKinds")}
          </button>
        </>
      ) : null}
    </>
  );
}
