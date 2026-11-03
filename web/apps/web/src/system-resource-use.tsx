// SPDX-License-Identifier: MPL-2.0

import "./system-resource-use.css";

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { dateTimeText, timeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { Loaded } from "./loaded.tsx";
import { byteSizeText, tenthsText } from "./number-text.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

type ResourceUse = components["schemas"]["ResourceUseOutputBody"];
type Summary = components["schemas"]["ResourceUseSummaryBody"];
type Hour = components["schemas"]["ResourceUseHourBody"];

/**
 * What the core and each node used of their machines, recorded each minute (toolkit.md): each
 * one's figures over the month kept, then its hours of the last day beside what it was doing, the
 * plays on the core's and the work running on a node's, so what a server needs can be judged.
 */
export function SystemResourceUse(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [use] = useResource(
    "resource-use",
    async () => dataOrThrow(await api.GET("/system/resource-use")),
    useBackgroundChangesPoller(),
  );
  return (
    <section aria-labelledby="system-resource-use">
      <h2 id="system-resource-use">{t("system.resourceUse.heading")}</h2>
      <p>{t("system.resourceUse.what")}</p>
      <Loaded resource={use} render={(value) => <ResourceUseView use={value} />} />
    </section>
  );
}

/** Each process's summary and its hours, the core's first. */
function ResourceUseView(props: { readonly use: ResourceUse }): JSX.Element {
  const { t } = useTranslation();
  const summaries = props.use.summaries ?? [];
  const hours = props.use.hours ?? [];
  if (summaries.length === 0) {
    return <p>{t("system.resourceUse.none")}</p>;
  }
  return (
    <>
      {summaries.map((summary) => (
        <ProcessUse
          key={processKey(summary)}
          summary={summary}
          hours={hours.filter((hour) => processKey(hour) === processKey(summary))}
        />
      ))}
    </>
  );
}

/** One process: what it used over the month kept, and its hours of the last day. */
function ProcessUse(props: {
  readonly summary: Summary;
  readonly hours: readonly Hour[];
}): JSX.Element {
  const { t } = useTranslation();
  const { summary } = props;
  const name = processName(t, summary);
  return (
    <section aria-label={name}>
      <h3>{name}</h3>
      <p>
        {t("system.resourceUse.summary", {
          since: dateTimeText(summary.since, i18n.language),
          cpu: cpuText(t, summary.cpuPercent, summary.busiestCpuPercent),
          memory: memoryText(t, summary),
        })}
      </p>
      {props.hours.length === 0 ? null : (
        <div className="resource-use__scroll">
          <table>
            <thead>
              <tr>
                <th scope="col">{t("system.resourceUse.hour")}</th>
                <th scope="col">{t("system.resourceUse.cpu")}</th>
                <th scope="col">{t("system.resourceUse.memory")}</th>
                <th scope="col">{t("system.resourceUse.doing")}</th>
              </tr>
            </thead>
            <tbody>
              {props.hours.map((hour) => (
                <tr key={hour.start}>
                  <th scope="row">{timeText(hour.start, i18n.language)}</th>
                  <td>{cpuText(t, hour.cpuPercent, hour.busiestCpuPercent)}</td>
                  <td>{memoryText(t, hour)}</td>
                  <td>{doingText(t, hour)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

type Translate = ReturnType<typeof useTranslation>["t"];

/** Names a process apart from the others: the core, or a node by its name. */
function processKey(process: { readonly isNode: boolean; readonly nodeName: string }): string {
  return process.isNode ? `node:${process.nodeName}` : "core";
}

/** A process's name as the page says it. */
function processName(t: Translate, process: Summary): string {
  return process.isNode
    ? t("system.resourceUse.node", { name: process.nodeName })
    : t("system.resourceUse.core");
}

/** CPU use as a share of one core, and its busiest minute's; unknown until a minute has it. */
function cpuText(t: Translate, percent?: number, busiest?: number): string {
  if (percent === undefined || busiest === undefined) {
    return t("system.resourceUse.cpuUnknown");
  }
  return t("system.resourceUse.cpuUse", {
    percent: tenthsText(percent, i18n.language),
    busiest: tenthsText(busiest, i18n.language),
  });
}

/** The memory held, the cache charged, and the most held when the kernel said. */
function memoryText(
  t: Translate,
  use: { readonly heldBytes: number; readonly cacheBytes: number; readonly peakBytes: number },
): string {
  const held = t("system.resourceUse.memoryUse", {
    held: byteSizeText(use.heldBytes, i18n.language),
    cache: byteSizeText(use.cacheBytes, i18n.language),
  });
  return use.peakBytes > 0
    ? t("system.resourceUse.withPeak", {
        use: held,
        peak: byteSizeText(use.peakBytes, i18n.language),
      })
    : held;
}

/** What the process was doing in the hour: the core's plays, or a node's work by kind. */
function doingText(t: Translate, hour: Hour): string {
  if (!hour.isNode) {
    return t("system.resourceUse.plays", { count: hour.plays });
  }
  const running = Object.entries(hour.running).filter(([, count]) => count > 0);
  if (running.length === 0) {
    return t("system.resourceUse.idle");
  }
  return running
    .map(([kind, count]) =>
      t("system.resourceUse.running", { kind: t(`jobs.kind.${camelKey(kind)}`), count }),
    )
    .join(", ");
}
