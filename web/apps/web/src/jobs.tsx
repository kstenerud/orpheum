// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { failure, type Resource } from "./use-resource.ts";

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

/** Whether a job can still be cancelled: it has not ended. */
export function isOpen(job: Job): boolean {
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
  const [listed, setListed] = useState<Resource<Job[]>>({ state: "loading" });
  const [asked, setAsked] = useState(0);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  useEffect(() => {
    let isCurrent = true;
    const load = (): void => {
      listJobs(api).then(
        (value) => {
          if (isCurrent) {
            setListed({ state: "loaded", value });
          }
        },
        (cause: unknown) => {
          if (isCurrent) {
            setListed(failure(cause));
          }
        },
      );
    };
    load();
    const ticker = setInterval(load, refreshEveryMs);
    return () => {
      isCurrent = false;
      clearInterval(ticker);
    };
  }, [api, asked]);
  const cancel = (job: Job): void => {
    cancelJob(api, job.id).then(
      () => {
        setProblem(undefined);
        setAsked((count) => count + 1);
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
      <Loaded resource={listed} render={(jobs) => <JobList jobs={jobs} onCancel={cancel} />} />
    </section>
  );
}

/** The jobs, each with its kind, subject, state, progress, reason and last error. */
export function JobList(props: {
  readonly jobs: readonly Job[];
  readonly onCancel: (job: Job) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  if (props.jobs.length === 0) {
    return <p>{t("jobs.none")}</p>;
  }
  const when = new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" });
  return (
    <ul className="jobs">
      {props.jobs.map((job) => {
        const kind = t(`jobs.kind.${camelKey(job.kind)}`);
        return (
          <li key={job.id} className="jobs__job">
            <strong>{t("jobs.what", { kind, subject: job.subject })}</strong>{" "}
            <span>{t(`jobs.state.${job.state}`)}</span>{" "}
            {job.progress === undefined ? null : (
              <progress
                value={job.progress}
                max={1}
                aria-label={t("jobs.progress", { kind, subject: job.subject })}
              />
            )}
            <span>{t("jobs.why", { reason: job.reason })}</span>
            <span>{t("jobs.made", { at: when.format(new Date(job.createdAt)) })}</span>
            {job.lastError === undefined ? null : (
              <span className="jobs__error">{t("jobs.lastError", { error: job.lastError })}</span>
            )}
            {isOpen(job) ? (
              <button
                type="button"
                aria-label={t("jobs.cancelLabel", { kind, subject: job.subject })}
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
