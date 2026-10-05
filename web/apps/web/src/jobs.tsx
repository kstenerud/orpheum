// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import type { JSX } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import { BusyGroup } from "./busy-group.tsx";
import { camelKey } from "./catalogue-key.ts";
import { dateTimeText } from "./date-time-text.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { useResource } from "./use-resource.ts";

/** One job, as the job view shows it (cut-a §5.3). */
export type Job = components["schemas"]["JobBody"];

/** The newest jobs, newest first. */
export async function listJobs(api: Api): Promise<Job[]> {
  return dataOrThrow(await api.GET("/jobs")).jobs ?? [];
}

/** Cancels a queued or running job; a node's work is stopped. */
export async function cancelJob(api: Api, jobId: number, idempotencyKey: string): Promise<void> {
  throwIfRefused(
    await api.POST("/jobs/{id}/cancel", {
      params: { path: { id: jobId } },
      ...idempotent(idempotencyKey),
    }),
  );
}

/**
 * A job as the list names it: its kind and what it works on, "Preparing a copy #12", or its kind
 * alone for a backup, which works on the whole server (D243 §11).
 */
function jobName(t: TFunction, job: Job): string {
  const kindText = t(`jobs.kind.${camelKey(job.kind)}`);
  const target = targetId(job);
  return target === undefined ? kindText : t("jobs.target", { kind: kindText, target });
}

/** The id of what a job works on: the one field its kind sets (D227). */
export function targetId(job: Job): number | undefined {
  return (
    job.itemId ??
    job.renditionId ??
    job.streamId ??
    job.fetchedSubtitleId ??
    job.fileId ??
    job.scanId ??
    job.scanBatchId ??
    job.chunkStoreId
  );
}

/** A destination that is down, which a job waits for (D231 §12). */
export type WaitingFor = NonNullable<Job["waitingFor"]>;

/**
 * What a job waits for, as the job view says it: "Waiting for TMDB: unreachable since 14:02, next
 * check 14:10", without a next check for one that only a signal brings back, such as a node's hello.
 */
export function waitingText(t: TFunction, waiting: WaitingFor, language: string): string {
  const words = {
    name: waiting.name,
    reason: waiting.reason,
    since: dateTimeText(waiting.downAt, language),
  };
  return waiting.nextCheckAt === undefined
    ? t("jobs.waitingForSignal", words)
    : t("jobs.waitingFor", { ...words, next: dateTimeText(waiting.nextCheckAt, language) });
}

/** Whether a job can still be cancelled: it has not ended. */
export function isCancellable(job: Job): boolean {
  return job.state === "queued" || job.state === "running";
}

/**
 * The job view, for the admin: every recent job, what it works on, how far it is and why, with
 * a way to cancel one that has not ended. It asks again every few seconds, keeping what it shows
 * meanwhile.
 */
export function Jobs(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [jobs, reloadJobs] = useResource("jobs", () => listJobs(api), useBackgroundChangesPoller());
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    problemText(t, cause, { conflict: t("jobs.alreadyEnded") }),
  );
  const cancel = (job: Job): void => {
    runRequest(async (idempotencyKey) => {
      await cancelJob(api, job.id, idempotencyKey);
      reloadJobs();
    });
  };
  return (
    <section aria-labelledby="jobs-heading">
      <h1 id="jobs-heading">{t("jobs.heading")}</h1>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <BusyGroup isBusy={isBusy}>
        <Loaded resource={jobs} render={(value) => <JobList jobs={value} onCancel={cancel} />} />
      </BusyGroup>
    </section>
  );
}

/** The jobs, each with its kind, target, state, progress, reason and last failure. */
export function JobList(props: {
  readonly jobs: readonly Job[];
  readonly onCancel: (job: Job) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  if (props.jobs.length === 0) {
    return <p>{t("jobs.none")}</p>;
  }
  return (
    <ul className="jobs">
      {props.jobs.map((job) => {
        const jobText = jobName(t, job);
        return (
          <li key={job.id} className="jobs__job">
            <strong>{jobText}</strong> <span>{t(`jobs.state.${job.state}`)}</span>{" "}
            {job.progressFraction === undefined ? null : (
              <progress
                value={job.progressFraction}
                max={1}
                aria-label={t("jobs.progress", { job: jobText })}
              />
            )}
            <span>{t("jobs.reason", { reason: job.reason })}</span>
            <span>{t("jobs.queuedAt", { at: dateTimeText(job.createdAt, i18n.language) })}</span>
            {job.waitingFor === undefined ? null : (
              <span className="jobs__waiting">{waitingText(t, job.waitingFor, i18n.language)}</span>
            )}
            {job.onNode === "waiting" || job.onNode === "paused" ? (
              <span className="jobs__waiting">{t(`jobs.onNode.${job.onNode}`)}</span>
            ) : null}
            {job.lastFailure === undefined ? null : (
              <span className="jobs__failure">
                {t("jobs.lastFailure", { failure: job.lastFailure })}
              </span>
            )}
            {isCancellable(job) ? (
              <button
                type="button"
                aria-label={t("jobs.cancelLabel", { job: jobText })}
                onClick={() => {
                  props.onCancel(job);
                }}
              >
                {t("jobs.cancel")}
              </button>
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}
