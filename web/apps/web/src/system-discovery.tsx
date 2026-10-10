// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { ConfirmButton } from "./confirm-button.tsx";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { idempotent } from "./idempotent.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

type Standing = components["schemas"]["DiscoveryStatusBody"];
type PortMapping = components["schemas"]["PortMappingBody"];
type WebRTCPath = components["schemas"]["WebRTCPathBody"];

/** The paid discovery's address, the one an admin is offered first (D273 §2). */
export const serviceDiscoveryUrl = "https://api.orpheum-app.com/core/v1";

/**
 * The core's discovery on the System page (D273): connecting to one, the paid service or the
 * owner's own, the code its owner claims it by there, and once claimed where clients can reach it
 * from, as the discovery's probe found; and leaving it.
 */
export function SystemDiscovery(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [standing, reload] = useResource(
    "discovery",
    async () => dataOrThrow(await api.GET("/system/discovery")),
    useBackgroundChangesPoller(),
  );
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const connect = (baseUrl: string): void => {
    runRequest(async (idempotencyKey) => {
      dataOrThrow(
        await api.POST("/system/discovery", { body: { baseUrl }, ...idempotent(idempotencyKey) }),
      );
      reload();
    });
  };
  const leave = (): void => {
    runRequest(async (idempotencyKey) => {
      throwIfRefused(await api.DELETE("/system/discovery", idempotent(idempotencyKey)));
      reload();
    });
  };
  return (
    <section aria-labelledby="system-discovery">
      <h2 id="system-discovery">{t("system.discovery.heading")}</h2>
      <p>{t("system.discovery.what")}</p>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <Loaded
        resource={standing}
        render={(value) =>
          value.baseUrl === "" ? (
            <ConnectForm isBusy={isBusy} onConnect={connect} />
          ) : (
            <StandingView standing={value} onLeave={leave} />
          )
        }
      />
    </section>
  );
}

/** The discovery's address, the service's to begin with, and connecting to it. */
function ConnectForm(props: {
  readonly isBusy: boolean;
  readonly onConnect: (baseUrl: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [baseUrl, setBaseUrl] = useState(serviceDiscoveryUrl);
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        props.onConnect(baseUrl.trim());
      }}
    >
      <label>
        {t("system.discovery.address")}{" "}
        <input
          type="url"
          value={baseUrl}
          onChange={(event) => {
            setBaseUrl(event.currentTarget.value);
          }}
        />
      </label>{" "}
      <button type="submit" disabled={props.isBusy || baseUrl.trim() === ""}>
        {t("system.discovery.connect")}
      </button>
    </form>
  );
}

/** Where the core stands with its discovery, and leaving it. */
function StandingView(props: {
  readonly standing: Standing;
  readonly onLeave: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { standing } = props;
  const published = standing.published ?? [];
  return (
    <>
      <p>{t("system.discovery.at", { address: standing.baseUrl })}</p>
      {standing.state === undefined ? null : <p>{t(`system.discovery.state.${standing.state}`)}</p>}
      {standing.claimCode === undefined ? null : (
        <p>
          {t("system.discovery.claim", {
            code: standing.claimCode,
            until: dateTimeText(standing.claimExpiresAt, i18n.language),
          })}
        </p>
      )}
      {standing.reachableFrom === undefined ? null : (
        <p>{t(`system.discovery.reachable.${camelKey(standing.reachableFrom)}`)}</p>
      )}
      {published.length === 0 ? null : (
        <p>{t("system.discovery.published", { names: published.join(", ") })}</p>
      )}
      {standing.lastHeartbeatAt === 0 ? null : (
        <p>
          {t("system.discovery.heartbeat", {
            at: dateTimeText(standing.lastHeartbeatAt, i18n.language),
          })}
        </p>
      )}
      {standing.problem === undefined ? null : (
        <p>{t("system.discovery.problem", { problem: standing.problem })}</p>
      )}
      <PortMappingView mapping={standing.portMapping} />
      <WebRTCPaths paths={standing.webrtcPaths ?? []} />
      {standing.deprecatedUntil === undefined ? null : (
        <p role="alert">{t("system.discovery.deprecated", { date: standing.deprecatedUntil })}</p>
      )}
      <ConfirmButton
        label={t("system.discovery.leave")}
        consequence={t("system.discovery.leaveConsequence")}
        confirmLabel={t("system.discovery.leaveConfirm")}
        onConfirm={props.onLeave}
      />
    </>
  );
}

/**
 * What the home router opened for the core (D273 §7): each opening, and why each way that failed
 * failed; nothing while the core is not asking.
 */
function PortMappingView(props: { readonly mapping: PortMapping }): JSX.Element | null {
  const { t } = useTranslation();
  const { mapping } = props;
  if (!mapping.isWanted) {
    return null;
  }
  const mappings = mapping.mappings ?? [];
  const problems = mapping.problems ?? [];
  return (
    <>
      <p>
        {mappings.length === 0
          ? t("system.discovery.portMapping.none")
          : t("system.discovery.portMapping.opened")}
      </p>
      <ul>
        {mappings.map((each) => (
          <li key={`${each.method} ${each.internal}`}>
            {t("system.discovery.portMapping.mapping", {
              method: t(`system.discovery.portMapping.method.${each.method}`),
              external: each.external,
              internal: each.internal,
            })}
          </li>
        ))}
      </ul>
      {problems.length === 0 ? null : (
        <details>
          <summary>{t("system.discovery.portMapping.problems")}</summary>
          <ul>
            {problems.map((problem) => (
              <li key={problem}>{problem}</li>
            ))}
          </ul>
        </details>
      )}
    </>
  );
}

/**
 * How the WebRTC connections since the core started reached it (D274 §3): directly, punched
 * through its NAT, relayed, or not at all; nothing before the first.
 */
function WebRTCPaths(props: { readonly paths: readonly WebRTCPath[] }): JSX.Element | null {
  const { t } = useTranslation();
  const taken = props.paths.filter((each) => each.count > 0);
  if (taken.length === 0) {
    return null;
  }
  return (
    <>
      <p>{t("system.discovery.paths.heading")}</p>
      <ul>
        {taken.map((each) => (
          <li key={each.path}>
            {t(`system.discovery.paths.path.${camelKey(each.path)}`, { count: each.count })}
          </li>
        ))}
      </ul>
    </>
  );
}
