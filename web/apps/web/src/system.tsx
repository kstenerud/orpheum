// SPDX-License-Identifier: MPL-2.0

import type { TFunction } from "i18next";
import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { BusyGroup } from "./busy-group.tsx";
import { DebugTool } from "./debug-tools.tsx";
import { SystemVideos } from "./system-videos.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import { camelKey } from "./catalogue-key.ts";
import { ConfirmButton } from "./confirm-button.tsx";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { NodeConfinementView } from "./node-confinement.tsx";
import { NodeStuckView } from "./node-stuck.tsx";
import { NodeUnconfinedLeave } from "./node-unconfined.tsx";
import { NowPlaying } from "./now-playing.tsx";
import { SystemBackups } from "./system-backups.tsx";
import { SystemContents, type ContentsEntry } from "./system-contents.tsx";
import { SystemResourceUse } from "./system-resource-use.tsx";
import { SystemUnusedSettings } from "./system-unused-settings.tsx";
import { SystemFilingReports } from "./filing-report.tsx";
import { SystemDiscovery } from "./system-discovery.tsx";
import { UpdateSystem } from "./update-page.tsx";
import { SystemReachability } from "./system-reachability.tsx";
import { NodeDevices, NodeWorkView, QueueHealth } from "./system-work.tsx";
import { DownDestinations, OutsideServices } from "./system-outside.tsx";
import { byteSizeText } from "./number-text.ts";
import { Link } from "./link.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { WhatWaits } from "./system-waiting.tsx";
import { WarningIcon } from "./warning-sign.tsx";
import { Loaded } from "./loaded.tsx";
import { useResource } from "./use-resource.ts";

/** Everything the server knows about itself and each machine, as the System page shows it (D225). */
export type SystemReport = components["schemas"]["SystemOutputBody"];
type Certificate = components["schemas"]["CertificateBody"];
/** One machine that reads media and runs FFmpeg, as it last described itself. */
export type SystemNode = components["schemas"]["SystemNodeBody"];
type Disk = components["schemas"]["DiskBody"];
type Hardware = components["schemas"]["HardwareBody"];
type Finding = components["schemas"]["FindingBody"];
type Build = components["schemas"]["BuildBody"];
type Host = components["schemas"]["HostBody"];
type Offline = components["schemas"]["OfflineBody"];

/** The System page, loaded from the server, and retiring a machine from it. */
export function System(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [report, reload] = useResource(
    "system",
    async () => dataOrThrow(await api.GET("/system")),
    useBackgroundChangesPoller(),
  );
  const retiring = useRequest((cause) => problemText(t, cause));
  const retireNode = (nodeName: string): void => {
    retiring.runRequest(async (idempotencyKey) => {
      throwIfRefused(
        await api.POST("/nodes/{name}/retire", {
          params: { path: { name: nodeName } },
          ...idempotent(idempotencyKey),
        }),
      );
      reload();
    });
  };
  return (
    <>
      {retiring.problem === undefined ? null : <p role="alert">{retiring.problem}</p>}
      <BusyGroup isBusy={retiring.isBusy}>
        <Loaded
          resource={report}
          render={(value) => (
            <SystemView report={value} onRetireNode={retireNode} onChanged={reload} />
          )}
        />
      </BusyGroup>
      <UpdateSystem />
      <NowPlaying />
      <SystemResourceUse />
      <SystemBackups />
      <SystemUnusedSettings />
      <SystemFilingReports />
      <SystemReachability />
      <SystemDiscovery />
      <DebugTool>
        <SystemVideos />
      </DebugTool>
    </>
  );
}

/**
 * The one place an admin sees what the server is and how it is doing (D225): its own versions,
 * disks and databases; what doctor finds; and for each machine, its software, its sandbox,
 * whether it encodes on its GPU, and what its recent prepares used.
 */
export function SystemView(props: {
  readonly report: SystemReport;
  readonly onRetireNode: (nodeName: string) => void;
  /** Something the admin did here changed the report: read it again. */
  readonly onChanged?: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { server } = props.report;
  const nodes = props.report.nodes ?? [];
  return (
    <div className="system">
      <h1>{t("system.heading")}</h1>
      <SystemContents entries={contentsEntries(t, nodes)} />
      <section aria-labelledby="system-server">
        <h2 id="system-server">{t("system.server")}</h2>
        <dl>
          <BuildFacts build={server.build} />
          <dt>{t("system.goVersion")}</dt>
          <dd>{server.goVersion}</dd>
          <HostFacts host={server.host} />
          <dt>{t("system.startedAt")}</dt>
          <dd>{dateTimeText(server.startedAt, i18n.language)}</dd>
          {server.certificate === undefined ? null : (
            <CertificateFacts certificate={server.certificate} />
          )}
          {(server.databases ?? []).map((database) => (
            <DatabaseSize key={database.name} name={database.name} bytes={database.sizeBytes} />
          ))}
        </dl>
        <Disks disks={server.disks ?? []} />
        <MultipathTcpNote isOn={server.isMultipathTcpOn} />
      </section>
      <Findings findings={props.report.findings ?? []} />
      <QueueHealth queue={props.report.queue ?? []} />
      <DownDestinations destinations={props.report.destinations ?? []} />
      <OutsideServices providers={props.report.providers ?? []} />
      <OfflineStorage offline={props.report.offline ?? []} />
      <WhatWaits report={props.report} onChanged={props.onChanged} />
      {nodes.map((node) => (
        <NodeFacts
          key={node.name}
          node={node}
          onChanged={props.onChanged}
          onRetire={() => {
            props.onRetireNode(node.name);
          }}
        />
      ))}
    </div>
  );
}

/** A note that the machine's kernel has MPTCP on, which the server's own sockets never use (D454 §4). */
function MultipathTcpNote(props: { readonly isOn: boolean }): JSX.Element | null {
  const { t } = useTranslation();
  return props.isOn ? <p>{t("system.multipathTcpOn")}</p> : null;
}

/**
 * The sections the page always has, and each machine's, in the page's order: those shown only when
 * there is something to say (work waiting, offline storage, unused settings…) are left out.
 */
function contentsEntries(t: TFunction, nodes: readonly SystemNode[]): readonly ContentsEntry[] {
  return [
    { headingId: "system-server", name: t("system.server") },
    { headingId: "system-findings", name: t("system.findings") },
    { headingId: "system-outside", name: t("system.outside") },
    ...nodes.map((node) => ({
      headingId: nodeHeadingId(node.name),
      name: t("system.node", { node: node.name }),
    })),
    { headingId: "now-playing", name: t("nowPlaying.heading") },
    { headingId: "system-resource-use", name: t("system.resourceUse.heading") },
    { headingId: "system-backups", name: t("system.backups.heading") },
    { headingId: "system-reachability", name: t("system.reachability.heading") },
    { headingId: "system-discovery", name: t("system.discovery.heading") },
  ];
}

/** The id of a machine's section heading, which the contents row leads to. */
function nodeHeadingId(nodeName: string): string {
  return `system-node-${nodeName}`;
}

/** Why a node that is not connected is not, in the viewer's words. */
function notConnectedText(t: TFunction, node: SystemNode, language: string): string {
  if (node.connection === "needs_update") {
    return t("system.needsUpdate");
  }
  if (node.connection === "silent" && node.lastHeardAt !== undefined) {
    return t("system.silentSince", { at: dateTimeText(node.lastHeardAt, language) });
  }
  return t("system.awaitingHello");
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
            <dd>{dateTimeText(node.lastHeardAt, i18n.language)}</dd>
          </>
        )}
      </dl>
    </>
  );
}

/** The public name's HTTPS certificate: when it expires, or that there is none yet (D243 §6). */
function CertificateFacts(props: { readonly certificate: Certificate }): JSX.Element {
  const { t } = useTranslation();
  const { certificate } = props;
  return (
    <>
      <dt>{t("system.certificate.heading", { name: certificate.name })}</dt>
      <dd>
        {certificate.expiresAt === undefined
          ? t("system.certificate.none")
          : t("system.certificate.expires", {
              at: dateTimeText(certificate.expiresAt, i18n.language),
            })}
      </dd>
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
              : ` ${t("system.committedAt", { when: dateTimeText(build.committedAt, i18n.language) })}`}
          </>
        )}
        {build.hasUncommittedChanges ? ` ${t("system.uncommittedChanges")}` : null}
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
      <dd>{byteSizeText(props.bytes, i18n.language)}</dd>
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
                available: byteSizeText(disk.availableBytes, i18n.language),
                total: byteSizeText(disk.totalBytes, i18n.language),
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
          <li key={finding.check} className={`system__finding system__finding--${finding.verdict}`}>
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

/**
 * The folders and disks scans have found missing, each since the first scan that did: nothing
 * of theirs is removed until an admin retires them or a scan finds them (D41, D231 §7).
 */
function OfflineStorage(props: { readonly offline: readonly Offline[] }): JSX.Element | null {
  const { t } = useTranslation();
  if (props.offline.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="system-offline">
      <h2 id="system-offline">{t("system.offline")}</h2>
      <ul>
        {props.offline.map((missing) => {
          const since = dateTimeText(missing.offlineAt, i18n.language);
          const place = { library: missing.libraryName, node: missing.nodeName, since };
          return (
            <li key={`${missing.nodeName}:${missing.rootPath}:${missing.mountPath ?? ""}`}>
              <WarningIcon />
              {missing.mountPath === undefined
                ? t("system.offlineRoot", { ...place, path: missing.rootPath })
                : t("system.offlineDisk", { ...place, path: missing.mountPath })}
            </li>
          );
        })}
      </ul>
      <p>
        {t("system.offlineHelp")}{" "}
        <Link to={{ screen: "libraries" }}>{t("system.offlineWhere")}</Link>
      </p>
    </section>
  );
}

/** A machine's facts, and retiring it for good, confirmed first (D231 §7). */
function NodeFacts(props: {
  readonly node: SystemNode;
  readonly onRetire: () => void;
  readonly onChanged?: (() => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { node } = props;
  const headingId = nodeHeadingId(node.name);
  return (
    <section aria-labelledby={headingId}>
      <h2 id={headingId}>{t("system.node", { node: node.name })}</h2>
      {node.isConnected ? (
        <>
          <NodeSoftware node={node} />
          <NodeWorkView work={node.work} />
          <NodeConfinementView node={node} />
          {node.hardware === undefined ? null : <HardwareEncoding hardware={node.hardware} />}
          {node.preparedUsage === undefined ? null : (
            <p>
              {t("system.preparedUsage", {
                used: byteSizeText(node.preparedUsage.usedBytes, i18n.language),
                budget: byteSizeText(node.preparedUsage.budgetBytes, i18n.language),
                percent: node.preparedUsage.diskUsedPercent,
                limit: node.preparedUsage.diskUsedLimitPercent,
              })}
            </p>
          )}
          <Disks disks={node.disks ?? []} />
          <NodeDevices nodeName={node.name} devices={node.devices ?? []} />
          <NodeStuckView stuck={node.stuck ?? []} />
        </>
      ) : null}
      {node.isConnected || node.connection === "locked" ? null : (
        <>
          <p>{notConnectedText(t, node, i18n.language)}</p>
          <p>{t("system.notConnectedHint")}</p>
        </>
      )}
      <NodeUnconfinedLeave node={node} onChanged={props.onChanged} />
      <ConfirmButton
        label={t("system.retireNode")}
        consequence={t("system.retireNodeConsequence")}
        confirmLabel={t("system.retireNodeConfirm")}
        onConfirm={props.onRetire}
      />
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
