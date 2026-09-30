// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { dateTimeText } from "./date-time-text.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** One job, as the job view shows it (cut-a §5.3). */
export type Job = components["schemas"]["JobBody"];

/** The newest jobs, newest first. */
export async function listJobs(api: Api): Promise<Job[]> {
  return dataOrThrow(await api.GET("/jobs")).jobs ?? [];
}

/** Cancels a queued or running job; a node's work is stopped. */
export async function cancelJob(api: Api, jobId: number): Promise<void> {
  throwIfRefused(await api.POST("/jobs/{id}/cancel", { params: { path: { id: jobId } } }));
}

/** How often the view asks again, so a running job's progress moves on the screen. */
const refreshEveryMs = 3000;

/** The id of what a job works on: the one field its kind sets (D227). */
export function targetId(job: Job): number | undefined {
  return job.itemId ?? job.renditionId ?? job.streamId ?? job.fetchedSubtitleId ?? job.fileId;
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
    since: dateTimeText(waiting.since, language),
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
  const [jobs, reloadJobs] = useResource("jobs", () => listJobs(api), refreshEveryMs);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const cancel = (job: Job): void => {
    cancelJob(api, job.id).then(
      () => {
        setProblem(undefined);
        reloadJobs();
      },
      (cause: unknown) => {
        setProblem(problemText(t, cause, { conflict: t("jobs.alreadyEnded") }));
      },
    );
  };
  return (
    <section aria-labelledby="jobs-heading">
      <h1 id="jobs-heading">{t("jobs.heading")}</h1>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <Loaded resource={jobs} render={(value) => <JobList jobs={value} onCancel={cancel} />} />
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
        const kindText = t(`jobs.kind.${camelKey(job.kind)}`);
        return (
          <li key={job.id} className="jobs__job">
            <strong>{t("jobs.target", { kind: kindText, target: targetId(job) })}</strong>{" "}
            <span>{t(`jobs.state.${job.state}`)}</span>{" "}
            {job.progressFraction === undefined ? null : (
              <progress
                value={job.progressFraction}
                max={1}
                aria-label={t("jobs.progress", { kind: kindText, target: targetId(job) })}
              />
            )}
            <span>{t("jobs.reason", { reason: job.reason })}</span>
            <span>{t("jobs.queuedAt", { at: dateTimeText(job.createdAt, i18n.language) })}</span>
            {job.waitingFor === undefined ? null : (
              <span className="jobs__waiting">{waitingText(t, job.waitingFor, i18n.language)}</span>
            )}
            {job.lastFailure === undefined ? null : (
              <span className="jobs__failure">
                {t("jobs.lastFailure", { failure: job.lastFailure })}
              </span>
            )}
            {isCancellable(job) ? (
              <button
                type="button"
                aria-label={t("jobs.cancelLabel", { kind: kindText, target: targetId(job) })}
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
