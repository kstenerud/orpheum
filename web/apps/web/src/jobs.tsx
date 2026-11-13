// SPDX-License-Identifier: MPL-2.0

import { allPages } from "@orpheum/sdk";
import "./jobs.css";

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import { BusyGroup } from "./busy-group.tsx";
import { camelKey } from "./catalogue-key.ts";
import { dateText, dateTimeText } from "./date-time-text.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { useResource } from "./use-resource.ts";

/** One job, as the job view shows it (cut-a §5.3). */
export type Job = components["schemas"]["JobBody"];

/** The newest jobs, newest first: every page the job view has. */
export async function listJobs(api: Api): Promise<Job[]> {
  return await allPages(async (query) => {
    const page = dataOrThrow(await api.GET("/jobs", { params: { query } }));
    return { entries: page.jobs ?? [], nextCursor: page.nextCursor };
  });
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
 * A job as the list names it: its kind and what it works on, "Making a film's chunks #12", or its kind
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
  const [jobs, reloadJobs] = useResource("jobs", () => listJobs(api), {
    poller: useBackgroundChangesPoller(),
  });
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
        <Loaded resource={jobs} render={(value) => <JobsView jobs={value} onCancel={cancel} />} />
      </BusyGroup>
    </section>
  );
}

/** Which jobs to show: every one, or those of one state or one kind; "" is every one. */
export interface JobFilter {
  readonly state: Job["state"] | "";
  readonly kind: Job["kind"] | "";
}

/** The jobs a filter keeps, in the order the server sent them. */
export function filteredJobs(jobs: readonly Job[], filter: JobFilter): readonly Job[] {
  return jobs.filter(
    (job) =>
      (filter.state === "" || job.state === filter.state) &&
      (filter.kind === "" || job.kind === filter.kind),
  );
}

/** The jobs not yet ended, running before waiting, each in the order the server sent them. */
export function activeJobs(jobs: readonly Job[]): readonly Job[] {
  return [
    ...jobs.filter((job) => job.state === "running"),
    ...jobs.filter((job) => job.state === "queued"),
  ];
}

/** The ended jobs of one day, the day as the start of it, local time. */
export interface JobDay {
  readonly day: number;
  readonly jobs: readonly Job[];
}

/**
 * The ended jobs by the local day they ended (or, without an end, were queued), the newest day
 * first, each day's in the order the server sent them.
 */
export function finishedByDay(jobs: readonly Job[]): readonly JobDay[] {
  const days = new Map<number, Job[]>();
  for (const job of jobs.filter((each) => !isCancellable(each))) {
    const at = new Date(job.finishedAt ?? job.createdAt);
    const day = new Date(at.getFullYear(), at.getMonth(), at.getDate()).getTime();
    days.set(day, [...(days.get(day) ?? []), job]);
  }
  return [...days.entries()]
    .sort(([one], [other]) => other - one)
    .map(([day, dayJobs]) => ({ day, jobs: dayJobs }));
}

/** Every state a job can be in, in the order a job passes through them. */
const jobStates = [
  "queued",
  "running",
  "done",
  "failed",
  "cancelled",
] as const satisfies readonly Job["state"][];

/**
 * The job view's list (ui-audit-2026-11-04.md §3, Jobs): a filter by state and by kind, run here on
 * the newest jobs the server sends (the owner, 2026-11-04, Option A); then the jobs running and
 * waiting; then the ended ones, folded by the day they ended.
 */
export function JobsView(props: {
  readonly jobs: readonly Job[];
  readonly onCancel: (job: Job) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const [filter, setFilter] = useState<JobFilter>({ state: "", kind: "" });
  if (props.jobs.length === 0) {
    return <p>{t("jobs.none")}</p>;
  }
  const shown = filteredJobs(props.jobs, filter);
  const active = activeJobs(shown);
  const days = finishedByDay(shown);
  return (
    <>
      <JobFilters jobs={props.jobs} filter={filter} onChange={setFilter} />
      {shown.length === 0 ? <p>{t("jobs.noneShown")}</p> : null}
      {active.length === 0 ? null : (
        <>
          <h2>{t("jobs.active")}</h2>
          <JobList jobs={active} onCancel={props.onCancel} />
        </>
      )}
      {days.length === 0 ? null : <h2>{t("jobs.finished")}</h2>}
      {days.map(({ day, jobs }) => (
        <details key={day} className="jobs__day">
          <summary>
            {t("jobs.day", { day: dateText(day, i18n.language), count: jobs.length })}
          </summary>
          <JobList jobs={jobs} onCancel={props.onCancel} />
        </details>
      ))}
    </>
  );
}

/** The filters: one state, or every one; one kind of those listed, or every one. */
function JobFilters(props: {
  readonly jobs: readonly Job[];
  readonly filter: JobFilter;
  readonly onChange: (filter: JobFilter) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { filter } = props;
  // The kinds listed, and one chosen that the newest jobs no longer hold, so it can be undone.
  const kinds = [
    ...new Set([
      ...props.jobs.map((job) => job.kind),
      ...(filter.kind === "" ? [] : [filter.kind]),
    ]),
  ];
  return (
    <div className="page-actions jobs__filters">
      <label>
        {t("jobs.filterState")}{" "}
        <select
          value={filter.state}
          onChange={(event) => {
            const state = jobStates.find((each) => each === event.target.value) ?? "";
            props.onChange({ ...filter, state });
          }}
        >
          <option value="">{t("jobs.every")}</option>
          {jobStates.map((state) => (
            <option key={state} value={state}>
              {t(`jobs.state.${state}`)}
            </option>
          ))}
        </select>
      </label>
      <label>
        {t("jobs.filterKind")}{" "}
        <select
          value={filter.kind}
          onChange={(event) => {
            const kind = kinds.find((each) => each === event.target.value) ?? "";
            props.onChange({ ...filter, kind });
          }}
        >
          <option value="">{t("jobs.every")}</option>
          {kinds.map((kind) => (
            <option key={kind} value={kind}>
              {t(`jobs.kind.${camelKey(kind)}`)}
            </option>
          ))}
        </select>
      </label>
    </div>
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
