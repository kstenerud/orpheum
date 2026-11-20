// SPDX-License-Identifier: MPL-2.0

import "./system-words.ts";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { HomeAddress } from "./home-address.tsx";

import { camelKey } from "./catalogue-key.ts";
import { ConfirmButton } from "./confirm-button.tsx";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { idempotent } from "./idempotent.ts";
import { Loaded } from "./loaded.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, isRefusedAt, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { ConnectSection, useJustLinked, useLink } from "./system-discovery-connect.tsx";
import {
  ConfirmIdentity,
  issuerProblem,
  typedName,
  useConnecting,
  type Asked,
  type Connecting,
} from "./system-discovery-connecting.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

type Standing = components["schemas"]["DiscoveryStatusBody"];
type PortMapping = components["schemas"]["PortMappingBody"];
type WebRTCPath = components["schemas"]["WebRTCPathBody"];
type HouseNetwork = components["schemas"]["HouseNetworkBody"];

/**
 * The core's discovery, the Remote access page (D273, D528 §1): at its top Connect, which links the server to an
 * account at the discovery by one press (D527 §2.1, §2.3), with where the server stands; behind
 * "Link another way", connecting by its issuer, the suggested discovery the core names or the
 * owner's own, with the admin's password (D391 §3), and the key the account made; for any but the
 * suggested one, first showing the identity its issuer shows, for the admin to compare with what its
 * operator shows (the root key audit's §3.8); once claimed where clients can reach it from, as the
 * discovery's probe found; and leaving it. The suggestion is the core's, never this page's own
 * (D391 §7).
 */
export function SystemDiscovery(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [standing, reload] = useResource(
    "discovery",
    async () => dataOrThrow(await api.GET("/system/discovery")),
    { poller: useBackgroundChangesPoller() },
  );
  const isJustLinked = useJustLinked();
  const linking = useLink(reload);
  const connecting = useEnrolling(reload);
  const claiming = useClaim(reload);
  const askAgain = (): void => {
    connecting.runRequest(async (idempotencyKey) => {
      throwIfRefused(await api.POST("/system/discovery/ask-again", idempotent(idempotencyKey)));
      reload();
    });
  };
  const leave = (): void => {
    connecting.runRequest(async (idempotencyKey) => {
      throwIfRefused(await api.DELETE("/system/discovery", idempotent(idempotencyKey)));
      reload();
    });
  };
  return (
    <section aria-labelledby="system-discovery">
      <h1 id="system-discovery">{t("system.discovery.heading")}</h1>
      <p>{t("system.discovery.what")}</p>
      {isJustLinked ? (
        <p>
          <output>{t("system.discovery.justLinked")}</output>
        </p>
      ) : null}
      {connecting.problem === undefined ? null : <p role="alert">{connecting.problem}</p>}
      <Loaded
        resource={standing}
        render={(value) => (
          <>
            <ConnectSection standing={value} linking={linking} />
            <AnotherWay standing={value} connecting={connecting} claiming={claiming} />
            {value.issuer === "" ? null : (
              <StandingView
                standing={value}
                isBusy={connecting.isBusy}
                onAskAgain={askAgain}
                onLeave={leave}
              />
            )}
          </>
        )}
      />
    </section>
  );
}

/** Connecting the core to a discovery by the admin's password alone, enrolling it unlinked (D391 §3). */
function useEnrolling(reload: () => void): Connecting {
  const { api } = useServices();
  return useConnecting(async (asked, identity, idempotencyKey) => {
    const { issuer, password } = asked;
    dataOrThrow(
      await api.POST("/system/discovery", {
        body: identity === undefined ? { issuer, password } : { issuer, password, identity },
        ...idempotent(idempotencyKey),
      }),
    );
    reload();
  }, connectProblem);
}

/**
 * Linking the way it was before Connect, behind "Link another way" (D527 §2.3), while the server is
 * yet to be linked: connecting with the admin's password, and then the operator's code and the key
 * the household's account made.
 */
function AnotherWay(props: {
  readonly standing: Standing;
  readonly connecting: Connecting;
  readonly claiming: Claiming;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { standing } = props;
  if (standing.issuer !== "" && standing.state !== "pending") {
    return null;
  }
  return (
    <details className="system__fold">
      <summary>{t("system.discovery.linkAnotherWay")}</summary>
      {standing.issuer === "" ? (
        <Unconnected connecting={props.connecting} suggested={standing.suggestedIssuer ?? ""} />
      ) : (
        <>
          {standing.claimCode === undefined ? null : (
            <p>
              {t("system.discovery.claim", {
                code: standing.claimCode,
                until: dateTimeText(standing.claimExpiresAt, i18n.language),
              })}
            </p>
          )}
          <KeyForm claiming={props.claiming} />
        </>
      )}
    </details>
  );
}

/** A core with no discovery: the form to connect one, or the identity to confirm. */
function Unconnected(props: {
  readonly connecting: Connecting;
  readonly suggested: string;
}): JSX.Element {
  const { t } = useTranslation();
  const { connecting, suggested } = props;
  if (connecting.toConfirm !== undefined) {
    return (
      <ConfirmIdentity
        toConfirm={connecting.toConfirm}
        isBusy={connecting.isBusy}
        onConfirm={connecting.confirm}
        onCancel={connecting.cancel}
      />
    );
  }
  return (
    <>
      <p>{t("system.discovery.anotherWay")}</p>
      <ConnectForm
        isBusy={connecting.isBusy}
        suggested={suggested}
        onConnect={(issuer, password) => {
          connecting.connect({ issuer, password, suggested });
        }}
      />
    </>
  );
}

/**
 * Why connecting failed: the password's and the issuer's own refusals in their words, the suggested
 * discovery's when this build knows none of its root keys, any other failure as every request's.
 */
function connectProblem(t: TFunction, cause: unknown, asked: Asked): string {
  if (isRefusedAt(cause, "body.password")) {
    return t("system.discovery.wrongPasswordToConnect");
  }
  return issuerProblem(t, cause, asked) ?? problemText(t, cause);
}

/**
 * The discovery's name, the suggested one to begin with, as a person types it, and the admin's
 * password, and connecting to it.
 */
function ConnectForm(props: {
  readonly isBusy: boolean;
  readonly suggested: string;
  readonly onConnect: (issuer: string, password: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [issuer, setIssuer] = useState(() => typedName(props.suggested));
  const [password, setPassword] = useState("");
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        props.onConnect(issuer.trim(), password);
      }}
    >
      <label>
        {t("system.discovery.issuer")}{" "}
        <input
          autoComplete="off"
          spellCheck={false}
          value={issuer}
          onChange={(event) => {
            setIssuer(event.currentTarget.value);
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
      <button type="submit" disabled={props.isBusy || issuer.trim() === "" || password === ""}>
        {t("system.discovery.connectHere")}
      </button>
    </form>
  );
}

/**
 * Where the core stands with its discovery beyond Connect's: whom it trusts it by, its household,
 * whether another server took its place, how it is reached; and leaving it.
 */
function StandingView(props: {
  readonly standing: Standing;
  readonly isBusy: boolean;
  readonly onAskAgain: () => void;
  readonly onLeave: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { standing } = props;
  return (
    <>
      <FromAnywhereLink standing={standing} />
      <AccountLink standing={standing} />
      <Trusted standing={standing} />
      {standing.realm === undefined ? null : (
        <p>{t("system.discovery.realm", { realm: standing.realm })}</p>
      )}
      <Removed standing={standing} />
      <Reach standing={standing} />
      {standing.lastHeartbeatAt === 0 ? null : (
        <p>
          {t("system.discovery.heartbeat", {
            at: dateTimeText(standing.lastHeartbeatAt, i18n.language),
          })}
        </p>
      )}
      <OneCopy standing={standing} isBusy={props.isBusy} onAskAgain={props.onAskAgain} />
      <Identity standing={standing} />
      {standing.problem === undefined ? null : (
        <p>{t("system.discovery.problem", { problem: standing.problem })}</p>
      )}
      <PortMappingView mapping={standing.portMapping} />
      <WebRTCPaths paths={standing.webrtcPaths ?? []} />
      <HouseNetworks networks={standing.houseNetworks ?? []} />
      {standing.deprecatedUntil === undefined ? null : (
        <p className="needs-fixing">
          <output>{t("system.discovery.deprecated", { date: standing.deprecatedUntil })}</output>
        </p>
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
 * The way to this server from anywhere once its discovery can lead clients to it: the discovery's
 * page, which takes a person signed in there on to the server (the owner, 2026-11-20: "a link that
 * takes them to [the discovery's page] so that it auto-forwards them to their server"), so a household
 * that claimed it need not come back by the house's plain http address. Shown while clients can
 * find the server, claimed or in its grace, and only where the discovery names its page.
 */
function FromAnywhereLink(props: { readonly standing: Standing }): JSX.Element | null {
  const { t } = useTranslation();
  const { loaderOrigin, state } = props.standing;
  if (
    loaderOrigin === undefined ||
    loaderOrigin === "" ||
    (state !== "claimed" && state !== "grace")
  ) {
    return null;
  }
  return (
    <p>
      <a href={loaderOrigin}>{t("system.discovery.fromAnywhere", { origin: loaderOrigin })}</a>
    </p>
  );
}

/**
 * The way to the household's admin account at the discovery, where its admin makes a key, as the
 * discovery's signed document names it: an admin come back months later for a new key had only the
 * install guide to find its address in (the owner, 2026-11-20). A discovery with no accounts names
 * none, and the page shows none.
 */
function AccountLink(props: { readonly standing: Standing }): JSX.Element | null {
  const { t } = useTranslation();
  const { accountOrigin } = props.standing;
  if (accountOrigin === undefined || accountOrigin === "") {
    return null;
  }
  return (
    <p>
      <a href={accountOrigin} rel="noreferrer noopener" target="_blank">
        {t("system.discovery.accountLink")}
      </a>
    </p>
  );
}

/**
 * One copy of the server serves (D437 §3, §4): this serving copy refused another that tried to start,
 * from where and when; or this copy, refused as a copy of another that serves, stopped asking, with
 * the button that has it ask again. A server older than these fields sends none: missing is neither.
 */
function OneCopy(props: {
  readonly standing: Standing;
  readonly isBusy: boolean;
  readonly onAskAgain: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { standing } = props;
  return (
    <>
      {standing.copyRefusedAt > 0 ? (
        <p className="needs-fixing">
          <output>
            {t("system.discovery.copyRefused", {
              from: standing.copyRefusedFrom ?? "",
              at: dateTimeText(standing.copyRefusedAt, i18n.language),
            })}
          </output>
        </p>
      ) : null}
      {standing.stoodAsideAt > 0 ? (
        <>
          <p className="needs-fixing">
            <output>
              {t("system.discovery.stoodAside", {
                at: dateTimeText(standing.stoodAsideAt, i18n.language),
              })}
            </output>
          </p>
          <button type="button" disabled={props.isBusy} onClick={props.onAskAgain}>
            {t("system.discovery.askAgain")}
          </button>
        </>
      ) : null}
    </>
  );
}

/** That another server was enrolled with this server's account in its place (D312 §15). */
function Removed(props: { readonly standing: Standing }): JSX.Element | null {
  const { t } = useTranslation();
  // A server older than removedAt sends none: missing is not removed.
  return props.standing.removedAt > 0 ? (
    <p className="needs-fixing">
      <output>
        {t("system.discovery.removed", {
          at: dateTimeText(props.standing.removedAt, i18n.language),
        })}
      </output>
    </p>
  ) : null;
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
        <p className="needs-fixing">
          <output>{t("system.discovery.deadEnds", { count: deadEndsLastWeek })}</output>
        </p>
      )}
      <DeadEndCauses causes={props.standing.deadEndCauses ?? []} />
      <HomeAddressOf standing={props.standing} />
    </>
  );
}

/** The words of the dead-end causes a discovery reports (D398 §3), any other word counted as other. */
type DeadEndCauseWord = "noIpv6" | "secureUnfinished" | "unknown" | "none" | "other";

/** The order the causes are told in when as common: the one the admin can act on first. */
const deadEndCauseWords: readonly DeadEndCauseWord[] = [
  "noIpv6",
  "secureUnfinished",
  "unknown",
  "none",
  "other",
];

/** The word cause is shown by: its own when the page knows it, else other. */
function deadEndCauseWordOf(cause: string): DeadEndCauseWord {
  return deadEndCauseWords.find((word) => word === cause) ?? "other";
}

/** How many dead ends had one cause, in words. */
function deadEndCauseText(t: TFunction, word: DeadEndCauseWord, count: number): string {
  switch (word) {
    case "noIpv6":
      return t("system.discovery.deadEndCause.noIpv6", { count });
    case "secureUnfinished":
      return t("system.discovery.deadEndCause.secureUnfinished", { count });
    case "unknown":
      return t("system.discovery.deadEndCause.unknown", { count });
    case "none":
      return t("system.discovery.deadEndCause.none", { count });
    case "other":
      return t("system.discovery.deadEndCause.other", { count });
  }
}

/**
 * The last week's dead ends by the cause each device reported, beside their count (D398 §7, the
 * owner's agreement of 2026-11-03): the commonest first, a word the page does not know counted as
 * another reason, so a discovery's word is never shown as it came. Nothing from a discovery that
 * keeps no causes, which leaves the count as it was.
 */
function DeadEndCauses(props: {
  readonly causes: readonly { readonly cause: string; readonly count: number }[];
}): JSX.Element | null {
  const { t } = useTranslation();
  const counts = new Map<DeadEndCauseWord, number>();
  for (const { cause, count } of props.causes) {
    if (count > 0) {
      const word = deadEndCauseWordOf(cause);
      counts.set(word, (counts.get(word) ?? 0) + count);
    }
  }
  if (counts.size === 0) {
    return null;
  }
  const told = deadEndCauseWords
    .filter((word) => counts.has(word))
    .sort((left, right) => (counts.get(right) ?? 0) - (counts.get(left) ?? 0))
    .map((word) => deadEndCauseText(t, word, counts.get(word) ?? 0));
  return <p>{t("system.discovery.deadEndCauses", { causes: told.join("; ") })}</p>;
}

/**
 * The server's home address once its certificate is held; before then, that it comes within a few
 * minutes, and that until it does admin through the discovery's page at home waits too (D297 §6:
 * the owner, after the production NAS's claim, 2026-10-19, met a person's view with nothing saying why).
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

/**
 * The discovery's root keys whose signatures count, by their fingerprints, for the admin to compare
 * with the discovery's own (D391 §2), and why no newer address document holds, when none does.
 */
function Trusted(props: { readonly standing: Standing }): JSX.Element {
  const { t } = useTranslation();
  const { identity, rootKeys, rootVersion, documentProblem, documentStaleAt } = props.standing;
  return (
    <>
      {identity === undefined ? null : <p>{t("system.discovery.identity", { identity })}</p>}
      {(rootKeys ?? []).length === 0 ? null : (
        <p>
          {t("system.discovery.rootKeys", {
            keys: (rootKeys ?? []).join(", "),
            version: rootVersion,
          })}
        </p>
      )}
      {documentStaleAt === undefined ? null : (
        <p className="needs-fixing">
          <output>
            {t("system.discovery.documentStale", {
              at: dateTimeText(documentStaleAt, i18n.language),
            })}
          </output>
        </p>
      )}
      {documentProblem === undefined ? null : (
        <p className="needs-fixing">
          <output>{t("system.discovery.documentProblem")}</output>
        </p>
      )}
    </>
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
 * through its NAT, out to a NAT both ends share and back (D354), relayed, or not at all; nothing
 * before the first.
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

/**
 * The house's public IPv6 networks the discovery recognises devices at home by (D356 §10): each
 * learnt one with the device that last reached the server directly at home and when, each configured
 * one with its warning that it goes stale; nothing while there are none.
 */
function HouseNetworks(props: { readonly networks: readonly HouseNetwork[] }): JSX.Element | null {
  const { t } = useTranslation();
  if (props.networks.length === 0) {
    return null;
  }
  return (
    <>
      <p>{t("system.discovery.houseNetworks.heading")}</p>
      <ul>
        {props.networks.map((each) => (
          <li key={each.network}>
            {each.source === "learnt"
              ? t("system.discovery.houseNetworks.learnt", {
                  network: each.network,
                  device: each.device ?? "",
                  at: dateTimeText(each.witnessedAt, i18n.language),
                })
              : t("system.discovery.houseNetworks.configured", { network: each.network })}
          </li>
        ))}
      </ul>
    </>
  );
}
