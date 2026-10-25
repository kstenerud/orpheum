// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { HomeAddress } from "./home-address.tsx";

import { camelKey } from "./catalogue-key.ts";
import { ConfirmButton } from "./confirm-button.tsx";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { idempotent } from "./idempotent.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { ApiProblem, dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
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
  const claiming = useClaim(reload);
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
            <StandingView standing={value} claiming={claiming} onLeave={leave} />
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

/** Where the core stands with its discovery, claiming it by the account's key, and leaving it. */
function StandingView(props: {
  readonly standing: Standing;
  readonly claiming: Claiming;
  readonly onLeave: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { standing } = props;
  return (
    <>
      <p>{t("system.discovery.at", { address: standing.baseUrl })}</p>
      {standing.realm === undefined ? null : (
        <p>{t("system.discovery.realm", { realm: standing.realm })}</p>
      )}
      <Enrolment standing={standing} claiming={props.claiming} />
      <Reach standing={standing} />
      {standing.lastHeartbeatAt === 0 ? null : (
        <p>
          {t("system.discovery.heartbeat", {
            at: dateTimeText(standing.lastHeartbeatAt, i18n.language),
          })}
        </p>
      )}
      <Identity standing={standing} />
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
 * Where the server stands with its account: taken off it, its state, the account, and while it waits
 * the key form and the operator's code.
 */
function Enrolment(props: {
  readonly standing: Standing;
  readonly claiming: Claiming;
}): JSX.Element {
  const { t } = useTranslation();
  const { standing } = props;
  return (
    <>
      {/* A server older than removedAt sends none: missing is not removed. */}
      {standing.removedAt > 0 ? (
        <p role="alert">
          {t("system.discovery.removed", {
            at: dateTimeText(standing.removedAt, i18n.language),
          })}
        </p>
      ) : null}
      {standing.state === undefined ? null : <p>{t(`system.discovery.state.${standing.state}`)}</p>}
      {standing.account === undefined ? null : (
        <p>{t("system.discovery.account", { account: standing.account })}</p>
      )}
      {standing.state === "pending" ? <KeyForm claiming={props.claiming} /> : null}
      {standing.claimCode === undefined ? null : (
        <p>
          {t("system.discovery.claim", {
            code: standing.claimCode,
            until: dateTimeText(standing.claimExpiresAt, i18n.language),
          })}
        </p>
      )}
    </>
  );
}

/**
 * Claiming by the account's key and the admin's password: the key's and the password's own refusals
 * in their words, any other failure as every request's, and the standing read again after.
 */
function useClaim(reload: () => void): Claiming {
  const { t } = useTranslation();
  const { api } = useServices();
  const claiming = useRequest((cause) =>
    isRefusedAt(cause, "body.password")
      ? t("system.discovery.wrongPassword")
      : problemText(t, cause, {
          invalidRequest: t("system.discovery.wrongKey"),
          conflict: t("system.discovery.notPending"),
        }),
  );
  const claim = (key: string, password: string): void => {
    claiming.runRequest(async (idempotencyKey) => {
      dataOrThrow(
        await api.POST("/system/discovery/claim", {
          body: { key, password },
          ...idempotent(idempotencyKey),
        }),
      );
      reload();
    });
  };
  return { isBusy: claiming.isBusy, problem: claiming.problem, onClaim: claim };
}

/** Whether the server refused what was entered at location ("body.password"). */
function isRefusedAt(cause: unknown, location: string): boolean {
  const problems =
    cause instanceof ApiProblem ? (cause.withCode("invalid_request")?.detail?.problems ?? []) : [];
  return problems.some((problem) => problem.location === location);
}

/** Claiming by the account's key: whether a claim is under way, why the last failed, and making one. */
interface Claiming {
  readonly isBusy: boolean;
  readonly problem: string | undefined;
  readonly onClaim: (key: string, password: string) => void;
}

/**
 * The key the household's account made, which claims the server for it (the protocol's §2.2): typed
 * as the account page shows it, spaces and dashes and all, for the discovery reads it loosely.
 */
function KeyForm(props: { readonly claiming: Claiming }): JSX.Element {
  const { t } = useTranslation();
  const { isBusy, problem, onClaim } = props.claiming;
  const [key, setKey] = useState("");
  const [password, setPassword] = useState("");
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        onClaim(key.trim(), password);
      }}
    >
      <label>
        {t("system.discovery.key")}{" "}
        <input
          autoComplete="off"
          spellCheck={false}
          value={key}
          onChange={(event) => {
            setKey(event.currentTarget.value);
          }}
        />
      </label>{" "}
      <label>
        {t("system.discovery.yourPassword")}{" "}
        <input
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(event) => {
            setPassword(event.currentTarget.value);
          }}
        />
      </label>{" "}
      <button type="submit" disabled={isBusy || key.trim() === "" || password === ""}>
        {t("system.discovery.claimByKey")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <p>{t("system.discovery.keyHint")}</p>
    </form>
  );
}

/** How the server is reached: from where, by which names, and how often a client found no way in. */
function Reach(props: { readonly standing: Standing }): JSX.Element {
  const { t } = useTranslation();
  const { reachableFrom, deadEndsLastWeek } = props.standing;
  const published = props.standing.published ?? [];
  return (
    <>
      {reachableFrom === undefined ? null : (
        <p>{t(`system.discovery.reachable.${camelKey(reachableFrom)}`)}</p>
      )}
      {published.length === 0 ? null : (
        <p>{t("system.discovery.published", { names: published.join(", ") })}</p>
      )}
      {deadEndsLastWeek === 0 ? null : (
        <p role="alert">{t("system.discovery.deadEnds", { count: deadEndsLastWeek })}</p>
      )}
      <HomeAddressOf standing={props.standing} />
    </>
  );
}

/**
 * The server's home address once its certificate is held; before then, that it comes within a few
 * minutes, and that until it does admin through the discovery's page at home waits too (D297 §6:
 * the owner, after nas.example's claim, 2026-10-19, met a person's view with nothing saying why).
 */
function HomeAddressOf(props: { readonly standing: Standing }): JSX.Element | null {
  const { t } = useTranslation();
  const addresses = props.standing.homeAddresses ?? [];
  if (addresses.length === 0) {
    return null;
  }
  return props.standing.isHomeAddressReady ? (
    <HomeAddress addresses={addresses} />
  ) : (
    <p>{t("system.discovery.homeAddressWaiting")}</p>
  );
}

/** Where the server's links open, and the fingerprint they carry (D293 §4; the protocol's §3.1). */
function Identity(props: { readonly standing: Standing }): JSX.Element {
  const { t } = useTranslation();
  const { clientOrigin, fingerprint } = props.standing;
  return (
    <>
      {clientOrigin === undefined ? null : (
        <p>{t("system.discovery.clientOrigin", { origin: clientOrigin })}</p>
      )}
      <p>{t("system.discovery.fingerprint", { fingerprint })}</p>
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
