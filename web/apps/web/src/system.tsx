// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { i18n } from "./i18n.ts";
import { NodeConfinementView } from "./node-confinement.tsx";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** Everything the server knows about itself and each machine, as the System page shows it (D225). */
export type SystemReport = components["schemas"]["SystemOutputBody"];
/** One machine that reads media and runs FFmpeg, as it last described itself. */
export type SystemNode = components["schemas"]["SystemNodeBody"];
type Disk = components["schemas"]["DiskBody"];
type Hardware = components["schemas"]["HardwareBody"];
type FinishedPrepare = components["schemas"]["FinishedPrepareBody"];
type Finding = components["schemas"]["FindingBody"];
type Build = components["schemas"]["BuildBody"];
type Host = components["schemas"]["HostBody"];

/** A size in bytes, in the unit that keeps it short: "12.3 GB". */
export function byteSize(bytes: number): string {
  const units = ["byte", "kilobyte", "megabyte", "gigabyte", "terabyte"] as const;
  let value = bytes;
  let index = 0;
  while (value >= 1000 && index < units.length - 1) {
    value /= 1000;
    index += 1;
  }
  return new Intl.NumberFormat(i18n.language, {
    style: "unit",
    unit: units[index] ?? "byte",
    unitDisplay: "short",
    maximumFractionDigits: index === 0 ? 0 : 1,
  }).format(value);
}

function when(milliseconds: number): string {
  return new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(milliseconds),
  );
}

/** The System page, loaded from the server. */
export function System(): JSX.Element {
  const { api } = useServices();
  const [report] = useResource("system", async () => dataOrThrow(await api.GET("/system")));
  return <Loaded resource={report} render={(value) => <SystemView report={value} />} />;
}

/**
 * The one place an admin sees what the server is and how it is doing (D225): its own versions,
 * disks and databases; what doctor finds; and for each machine, its software, its sandbox,
 * whether it encodes on its GPU, and what its recent prepares used.
 */
export function SystemView(props: { readonly report: SystemReport }): JSX.Element {
  const { t } = useTranslation();
  const { server } = props.report;
  return (
    <div className="system">
      <h1>{t("system.heading")}</h1>
      <section aria-labelledby="system-server">
        <h2 id="system-server">{t("system.server")}</h2>
        <dl>
          <BuildFacts build={server.build} />
          <dt>{t("system.goVersion")}</dt>
          <dd>{server.goVersion}</dd>
          <HostFacts host={server.host} />
          <dt>{t("system.startedAt")}</dt>
          <dd>{when(server.startedAt)}</dd>
          {(server.databases ?? []).map((database) => (
            <DatabaseSize key={database.name} name={database.name} bytes={database.sizeBytes} />
          ))}
        </dl>
        <Disks disks={server.disks ?? []} />
      </section>
      <Findings findings={props.report.findings ?? []} />
      {(props.report.nodes ?? []).map((node) => (
        <NodeFacts key={node.name} node={node} />
      ))}
    </div>
  );
}

/** A connected machine's build, system, FFmpeg and user, and whether its build is the server's. */
function NodeSoftware(props: { readonly node: SystemNode }): JSX.Element {
  const { t } = useTranslation();
  const { node } = props;
  return (
    <>
      {node.isServersBuild ? null : <p role="alert">{t("system.otherBuild")}</p>}
      <dl>
        {node.build === undefined ? null : <BuildFacts build={node.build} />}
        {node.host === undefined ? null : <HostFacts host={node.host} />}
        <dt>{t("system.ffmpeg")}</dt>
        <dd>
          {node.ffmpegVersion} <code>{(node.ffmpegSha256 ?? "").slice(0, 12)}</code>
        </dd>
        <dt>{t("system.runsAs")}</dt>
        <dd>{t("system.uidGid", { uid: node.uid, gid: node.gid })}</dd>
        {node.lastHeardAt === undefined ? null : (
          <>
            <dt>{t("system.lastHeard")}</dt>
            <dd>{when(node.lastHeardAt)}</dd>
          </>
        )}
      </dl>
    </>
  );
}

/** What a binary was built from: its tag, or dev and its commit, and whether the tree was modified. */
function BuildFacts(props: { readonly build: Build }): JSX.Element {
  const { t } = useTranslation();
  const { build } = props;
  return (
    <>
      <dt>{t("system.build")}</dt>
      <dd>
        <code>{build.name}</code>
        {build.commit === undefined ? null : (
          <>
            {" "}
            {t("system.commit", { commit: build.commit.slice(0, 12) })}
            {build.committedAt === undefined
              ? null
              : ` ${t("system.committedAt", { when: when(build.committedAt) })}`}
          </>
        )}
        {build.isModified ? ` ${t("system.uncommittedChanges")}` : null}
      </dd>
    </>
  );
}

/** A machine's operating system, as lsb_release -a shows it, with its kernel and platform. */
function HostFacts(props: { readonly host: Host }): JSX.Element {
  const { t } = useTranslation();
  const { host } = props;
  const rows: readonly (readonly [string, string | undefined])[] = [
    [t("system.distribution"), host.distribution],
    [t("system.description"), host.description],
    [t("system.release"), host.release],
    [t("system.codename"), host.codename],
    [t("system.kernel"), host.kernel],
    [t("system.platform"), host.platform],
  ];
  return (
    <>
      {rows
        .filter((row) => row[1] !== undefined && row[1] !== "")
        .map(([label, value]) => (
          <HostFact key={label} label={label} value={value ?? ""} />
        ))}
    </>
  );
}

function HostFact(props: { readonly label: string; readonly value: string }): JSX.Element {
  return (
    <>
      <dt>{props.label}</dt>
      <dd>{props.value}</dd>
    </>
  );
}

function DatabaseSize(props: { readonly name: string; readonly bytes: number }): JSX.Element {
  return (
    <>
      <dt>
        <code>{props.name}</code>
      </dt>
      <dd>{byteSize(props.bytes)}</dd>
    </>
  );
}

function Disks(props: { readonly disks: readonly Disk[] }): JSX.Element | null {
  const { t } = useTranslation();
  if (props.disks.length === 0) {
    return null;
  }
  return (
    <ul className="system__disks" aria-label={t("system.disks")}>
      {props.disks.map((disk) => (
        <li key={disk.path}>
          <code>{disk.path}</code>{" "}
          {disk.failure === undefined
            ? t("system.disk", {
                free: byteSize(disk.availableBytes),
                total: byteSize(disk.totalBytes),
              })
            : t("system.diskUnmeasured", { why: disk.failure })}
        </li>
      ))}
    </ul>
  );
}

function Findings(props: { readonly findings: readonly Finding[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="system-findings">
      <h2 id="system-findings">{t("system.findings")}</h2>
      <ul className="system__findings">
        {props.findings.map((finding) => (
          <li key={finding.check} className={`system__finding--${finding.verdict}`}>
            <strong>{t(`system.verdict.${finding.verdict}`)}</strong> {finding.check}:{" "}
            {finding.detail}
            {finding.action === undefined ? null : (
              <p>{t("system.action", { action: finding.action })}</p>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

function NodeFacts(props: { readonly node: SystemNode }): JSX.Element {
  const { t } = useTranslation();
  const { node } = props;
  const headingId = `system-node-${node.name}`;
  return (
    <section aria-labelledby={headingId}>
      <h2 id={headingId}>{t("system.node", { node: node.name })}</h2>
      {node.isConnected ? (
        <>
          <NodeSoftware node={node} />
          <NodeConfinementView node={node} />
          {node.hardware === undefined ? null : <HardwareEncoding hardware={node.hardware} />}
          {node.preparedUsage === undefined ? null : (
            <p>
              {t("system.preparedUsage", {
                used: byteSize(node.preparedUsage.usedBytes),
                budget: byteSize(node.preparedUsage.budgetBytes),
                percent: node.preparedUsage.diskUsedPercent,
                limit: node.preparedUsage.diskUsedLimitPercent,
              })}
            </p>
          )}
          <Disks disks={node.disks ?? []} />
          <RecentPrepares prepares={node.recentPrepares ?? []} />
        </>
      ) : (
        <p>{t("system.notConnected")}</p>
      )}
    </section>
  );
}

/** Does it see the GPU, may it open it, what worked, and what it chose (D225 §3). */
function HardwareEncoding(props: { readonly hardware: Hardware }): JSX.Element {
  const { t } = useTranslation();
  const { hardware } = props;
  const gpus = hardware.gpus ?? [];
  const isOnGPU = hardware.chosenEncoder !== "x264";
  let verdict = t("system.gpuUnused");
  if (isOnGPU) {
    verdict = t("system.encodesWith", {
      encoder: t(`system.encoder.${camelKey(hardware.chosenEncoder)}`),
    });
  } else if (gpus.length === 0) {
    verdict = t("system.noGPU");
  }
  return (
    <section aria-label={t("system.hardware")}>
      <h3>{t("system.hardware")}</h3>
      <p role={!isOnGPU && gpus.length > 0 ? "alert" : undefined}>{verdict}</p>
      <h4>{t("system.gpuDevices")}</h4>
      {gpus.length === 0 ? (
        <p>{t("system.seesNothing")}</p>
      ) : (
        <ul>
          {gpus.map((gpu) => (
            <li key={gpu.devicePath}>
              <code>{gpu.devicePath}</code>{" "}
              {gpu.driver === undefined ? null : t("system.driver", { driver: gpu.driver })}{" "}
              {gpu.noAccessReason === undefined
                ? t("system.canOpen")
                : t("system.cannotOpen", { why: gpu.noAccessReason })}
            </li>
          ))}
        </ul>
      )}
      <h4>{t("system.trials")}</h4>
      <ul>
        {(hardware.trials ?? []).map((trial) => (
          <li key={trial.encoder}>
            {t(`system.encoder.${camelKey(trial.encoder)}`)}:{" "}
            {trial.failure === undefined
              ? t("system.trialWorked")
              : t("system.trialFailed", { why: trial.failure })}
          </li>
        ))}
      </ul>
    </section>
  );
}

/** What the machine's recent prepares used, and how fast they ran (D225 §3). */
function RecentPrepares(props: { readonly prepares: readonly FinishedPrepare[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-label={t("system.recentPrepares")}>
      <h3>{t("system.recentPrepares")}</h3>
      {props.prepares.length === 0 ? (
        <p>{t("system.noPrepares")}</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th scope="col">{t("system.preparedFile")}</th>
              <th scope="col">{t("system.preparedWith")}</th>
              <th scope="col">{t("system.speed")}</th>
              <th scope="col">{t("system.finishedAt")}</th>
            </tr>
          </thead>
          <tbody>
            {props.prepares.map((prepare) => (
              <tr
                key={`${String(prepare.sourceFileId)}-${prepare.profile}-${String(prepare.finishedAt)}`}
              >
                <td>
                  <code>
                    {prepare.path === "" ? `#${String(prepare.sourceFileId)}` : prepare.path}
                  </code>
                </td>
                <td>
                  {prepare.state === "failed"
                    ? t("system.prepareFailed", { why: prepare.failure ?? "" })
                    : t(`system.encoder.${camelKey(prepare.videoEncoder ?? "unknown")}`)}
                </td>
                <td>
                  {prepare.realtimeFactor === undefined
                    ? ""
                    : t("system.speedTimes", {
                        speed: prepare.realtimeFactor.toLocaleString(i18n.language, {
                          maximumFractionDigits: 1,
                        }),
                      })}
                </td>
                <td>{when(prepare.finishedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
