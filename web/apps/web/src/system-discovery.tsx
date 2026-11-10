// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useRef, useState, type JSX } from "react";
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
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

type Standing = components["schemas"]["DiscoveryStatusBody"];
type PortMapping = components["schemas"]["PortMappingBody"];
type WebRTCPath = components["schemas"]["WebRTCPathBody"];
type HouseNetwork = components["schemas"]["HouseNetworkBody"];

/** A discovery its issuer showed, waiting for the admin to compare its identity (D393 §4). */
interface ToConfirm {
  readonly issuer: string;
  readonly password: string;
  readonly identity: string;
  readonly name: string;
}

/**
 * The core's discovery on the System page (D273): connecting to one by its issuer, the suggested
 * discovery the core names or the owner's own, with the admin's password (D391 §3); for any but the
 * suggested one, first showing the identity its issuer shows, for the admin to compare with what its
 * operator shows (the root key audit's §3.8); the code its owner claims it by there, and once claimed
 * where clients can reach it from, as the discovery's probe found; and leaving it. The suggestion is
 * the core's, never this page's own (D391 §7).
 */
export function SystemDiscovery(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [standing, reload] = useResource(
    "discovery",
    async () => dataOrThrow(await api.GET("/system/discovery")),
    useBackgroundChangesPoller(),
  );
  const connecting = useConnecting(reload);
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
      <h2 id="system-discovery">{t("system.discovery.heading")}</h2>
      <p>{t("system.discovery.what")}</p>
      {connecting.problem === undefined ? null : <p role="alert">{connecting.problem}</p>}
      <Loaded
        resource={standing}
        render={(value) =>
          value.issuer === "" ? (
            <Unconnected connecting={connecting} suggested={value.suggestedIssuer ?? ""} />
          ) : (
            <StandingView
              standing={value}
              claiming={claiming}
              isBusy={connecting.isBusy}
              onAskAgain={askAgain}
              onLeave={leave}
            />
          )
        }
      />
    </section>
  );
}

/** Connecting to a discovery: the request under way, why it failed, and the identity to confirm. */
interface Connecting {
  readonly isBusy: boolean;
  readonly problem: string | undefined;
  readonly runRequest: ReturnType<typeof useRequest>["runRequest"];
  readonly toConfirm: ToConfirm | undefined;
  readonly connect: (issuer: string, password: string, suggested: string) => void;
  readonly confirm: () => void;
  readonly cancel: () => void;
}

/**
 * Connecting: the suggested discovery at once, any other once the identity its issuer shows is
 * confirmed (D393 §4); and the core's standing read again after.
 */
function useConnecting(reload: () => void): Connecting {
  const { t } = useTranslation();
  const { api } = useServices();
  // What the admin last asked to connect to, which says why it was refused.
  const askedRef = useRef({ issuer: "", suggested: "" });
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    connectProblem(t, cause, askedRef.current),
  );
  const [toConfirm, setToConfirm] = useState<ToConfirm | undefined>(undefined);
  const enrol = (issuer: string, password: string, identity?: string): void => {
    runRequest(async (idempotencyKey) => {
      dataOrThrow(
        await api.POST("/system/discovery", {
          body: identity === undefined ? { issuer, password } : { issuer, password, identity },
          ...idempotent(idempotencyKey),
        }),
      );
      setToConfirm(undefined);
      reload();
    });
  };
  const connect = (issuer: string, password: string, suggested: string): void => {
    askedRef.current = { issuer, suggested };
    if (suggested !== "" && typedName(issuer) === typedName(suggested)) {
      enrol(issuer, password);
      return;
    }
    runRequest(async (idempotencyKey) => {
      const shown = dataOrThrow(
        await api.POST("/system/discovery/identify", {
          body: { issuer },
          ...idempotent(idempotencyKey),
        }),
      );
      setToConfirm({ issuer, password, identity: shown.identity, name: shown.name });
    });
  };
  const confirm = (): void => {
    if (toConfirm !== undefined) {
      enrol(toConfirm.issuer, toConfirm.password, toConfirm.identity);
    }
  };
  const cancel = (): void => {
    setToConfirm(undefined);
  };
  return { isBusy, problem, runRequest, toConfirm, connect, confirm, cancel };
}

/** A core with no discovery: the form to connect one, or the identity to confirm. */
function Unconnected(props: {
  readonly connecting: Connecting;
  readonly suggested: string;
}): JSX.Element {
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
    <ConnectForm
      isBusy={connecting.isBusy}
      suggested={suggested}
      onConnect={(issuer, password) => {
        connecting.connect(issuer, password, suggested);
      }}
    />
  );
}

/**
 * Why connecting failed: the password's and the issuer's own refusals in their words, the suggested
 * discovery's when this build knows none of its root keys, any other failure as every request's.
 */
function connectProblem(
  t: TFunction,
  cause: unknown,
  asked: { readonly issuer: string; readonly suggested: string },
): string {
  if (isRefusedAt(cause, "body.password")) {
    return t("system.discovery.wrongPasswordToConnect");
  }
  if (isRefusedAt(cause, "body.identity")) {
    return t("system.discovery.notTheIdentity");
  }
  if (isRefusedAt(cause, "body.issuer")) {
    return asked.suggested !== "" && typedName(asked.issuer) === typedName(asked.suggested)
      ? t("system.discovery.suggestedUnknown", { issuer: typedName(asked.suggested) })
      : t("system.discovery.notAnIssuer");
  }
  return problemText(t, cause);
}

/** An issuer as a person types it: its host, without https:// or a closing slash. */
function typedName(issuer: string): string {
  return issuer
    .trim()
    .replace(/^https:\/\//iu, "")
    .replace(/\/$/u, "")
    .toLowerCase();
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
        {t("system.discovery.connect")}
      </button>
    </form>
  );
}

/**
 * The identity a discovery's issuer showed, for the admin to compare with what its operator shows,
 * and connecting to it only when they confirm.
 */
function ConfirmIdentity(props: {
  readonly toConfirm: ToConfirm;
  readonly isBusy: boolean;
  readonly onConfirm: () => void;
  readonly onCancel: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { toConfirm } = props;
  return (
    <>
      <p>
        {t("system.discovery.compareIdentity", {
          name: toConfirm.name,
          issuer: typedName(toConfirm.issuer),
        })}
      </p>
      <p>
        <code>{toConfirm.identity}</code>
      </p>
      <button type="button" disabled={props.isBusy} onClick={props.onConfirm}>
        {t("system.discovery.identityMatches")}
      </button>{" "}
      <button type="button" disabled={props.isBusy} onClick={props.onCancel}>
        {t("system.discovery.identityCancel")}
      </button>
    </>
  );
}

/** Where the core stands with its discovery, claiming it by the account's key, and leaving it. */
function StandingView(props: {
  readonly standing: Standing;
  readonly claiming: Claiming;
  readonly isBusy: boolean;
  readonly onAskAgain: () => void;
  readonly onLeave: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { standing } = props;
  return (
    <>
      <p>
        {standing.isSuggested
          ? t("system.discovery.atService", { issuer: standing.issuer })
          : t("system.discovery.at", { name: standing.name ?? "", issuer: standing.issuer })}
      </p>
      <Trusted standing={standing} />
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
      <OneCopy standing={standing} isBusy={props.isBusy} onAskAgain={props.onAskAgain} />
      <Identity standing={standing} />
      {standing.problem === undefined ? null : (
        <p>{t("system.discovery.problem", { problem: standing.problem })}</p>
      )}
      <PortMappingView mapping={standing.portMapping} />
      <WebRTCPaths paths={standing.webrtcPaths ?? []} />
      <HouseNetworks networks={standing.houseNetworks ?? []} />
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
        <p role="alert">
          {t("system.discovery.copyRefused", {
            from: standing.copyRefusedFrom ?? "",
            at: dateTimeText(standing.copyRefusedAt, i18n.language),
          })}
        </p>
      ) : null}
      {standing.stoodAsideAt > 0 ? (
        <>
          <p role="alert">
            {t("system.discovery.stoodAside", {
              at: dateTimeText(standing.stoodAsideAt, i18n.language),
            })}
          </p>
          <button type="button" disabled={props.isBusy} onClick={props.onAskAgain}>
            {t("system.discovery.askAgain")}
          </button>
        </>
      ) : null}
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
        <p role="alert">
          {t("system.discovery.documentStale", {
            at: dateTimeText(documentStaleAt, i18n.language),
          })}
        </p>
      )}
      {documentProblem === undefined ? null : (
        <p role="alert">{t("system.discovery.documentProblem")}</p>
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
