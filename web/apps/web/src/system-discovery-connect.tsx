// SPDX-License-Identifier: MPL-2.0

// The System page's Connect (D527 §2.3): one button that links the server to an account at the
// discovery, by the account page the core answers, and what the page says on coming back.

import "./system-words.ts";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { idempotent } from "./idempotent.ts";
import { dataOrThrow, isRefusedAt, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import {
  ConfirmIdentity,
  issuerProblem,
  typedName,
  useConnecting,
  type Asked,
  type Connecting,
} from "./system-discovery-connecting.tsx";

type Standing = components["schemas"]["DiscoveryStatusBody"];

/** The query parameter the discovery's account page comes back with once it linked the server. */
const linkedParameter = "linked";

/**
 * Whether the page came back from the discovery's account page with the server linked
 * (`?linked=1`, D527 §2.1), which it says: the parameter leaves the address at once, without a
 * reload, so a reload or the address copied never says it again.
 */
export function useJustLinked(): boolean {
  const { navigation } = useServices();
  const [isJustLinked] = useState(
    () => new URLSearchParams(navigation.query()).get(linkedParameter) === "1",
  );
  useEffect(() => {
    if (isJustLinked) {
      navigation.replace(navigation.route());
    }
  }, [isJustLinked, navigation]);
  return isJustLinked;
}

/** Connect's request, as connecting's, and whether the core asked for the admin's password. */
export interface Linking extends Connecting {
  readonly isPasswordAsked: boolean;
}

/**
 * Connect (D527 §2.1): the core enrolled if it is not, and the browser sent to the discovery's
 * account page that links it, which comes back to this page; the admin's password given only once
 * the core asked for it, holding no away-from-home key for them (§2.2). A refusal reads the core's
 * standing again, which an enrolment before it may have changed.
 */
export function useLink(reload: () => void): Linking {
  const { api, navigation } = useServices();
  const [isPasswordAsked, setIsPasswordAsked] = useState(false);
  const connecting = useConnecting(
    async (asked, identity, idempotencyKey) => {
      const body = {
        ...(asked.issuer === "" ? {} : { issuer: asked.issuer }),
        ...(identity === undefined ? {} : { identity }),
        ...(asked.password === "" ? {} : { password: asked.password }),
      };
      try {
        const { linkUrl } = dataOrThrow(
          await api.POST("/system/discovery/link", { body, ...idempotent(idempotencyKey) }),
        );
        navigation.leave(linkUrl);
      } catch (cause) {
        reload();
        throw cause;
      }
    },
    (t, cause, asked) => {
      if (isRefusedAt(cause, "body.password")) {
        setIsPasswordAsked(true);
      }
      return linkProblem(t, cause, asked);
    },
  );
  return { ...connecting, isPasswordAsked };
}

/**
 * Why Connect failed, each of its refusals in its own words (D527 §4): the password needed, or not
 * the admin's; the page's origin missing; the discovery's name or identity; only an admin; the core
 * no longer enrolled; the discovery not answering; any other failure as every request's.
 */
function linkProblem(t: TFunction, cause: unknown, asked: Asked): string {
  if (isRefusedAt(cause, "body.password")) {
    return asked.password === ""
      ? t("system.discovery.passwordNeeded")
      : t("system.discovery.wrongPasswordToLink");
  }
  if (isRefusedAt(cause, "header.Origin")) {
    return t("system.discovery.noOrigin");
  }
  return (
    issuerProblem(t, cause, asked) ??
    problemText(t, cause, {
      invalidRequest: t("system.discovery.linkRefused"),
      forbidden: t("system.discovery.onlyAdmins"),
      conflict: t("system.discovery.linkNotConnected"),
      providerError: t("system.discovery.linkUnanswered"),
    })
  );
}

/**
 * Connect, at the discovery part's top (D527 §2.1, §2.3): where the server stands with its
 * discovery, linked to an account or not, and until it is the discovery to link it at, the core's
 * suggestion to begin with, and Connect.
 */
export function ConnectSection(props: {
  readonly standing: Standing;
  readonly linking: Linking;
}): JSX.Element {
  const { t } = useTranslation();
  const { standing, linking } = props;
  return (
    <section aria-labelledby="system-discovery-connect">
      <h3 id="system-discovery-connect">{t("system.discovery.connectHeading")}</h3>
      {standing.issuer === "" ? (
        <p>{t("system.discovery.notLinked")}</p>
      ) : (
        <p>
          {standing.isSuggested
            ? t("system.discovery.atService", { issuer: standing.issuer })
            : t("system.discovery.at", { name: standing.name ?? "", issuer: standing.issuer })}
        </p>
      )}
      {standing.state === undefined ? null : <p>{t(`system.discovery.state.${standing.state}`)}</p>}
      {standing.account === undefined ? null : (
        <p>{t("system.discovery.account", { account: standing.account })}</p>
      )}
      {isLinkable(standing) ? (
        <>
          <NeedsInvitation />
          {linking.toConfirm === undefined ? (
            <LinkForm standing={standing} linking={linking} />
          ) : (
            <ConfirmIdentity
              toConfirm={linking.toConfirm}
              isBusy={linking.isBusy}
              onConfirm={linking.confirm}
              onCancel={linking.cancel}
            />
          )}
          {linking.problem === undefined ? null : <p role="alert">{linking.problem}</p>}
          <p>{t("system.discovery.connectHint")}</p>
        </>
      ) : null}
    </section>
  );
}

/** Whether the server is yet to be linked to an account: connected to no discovery, or waiting. */
function isLinkable(standing: Standing): boolean {
  return standing.issuer === "" || standing.state === undefined || standing.state === "pending";
}

/**
 * Connect's form: the discovery's name, as a person types it, the suggested one to begin with, while
 * the core is connected to none; the admin's password once the core asked for it; and Connect.
 */
function LinkForm(props: { readonly standing: Standing; readonly linking: Linking }): JSX.Element {
  const { t } = useTranslation();
  const { standing, linking } = props;
  const suggested = standing.suggestedIssuer ?? "";
  const isConnected = standing.issuer !== "";
  const [issuer, setIssuer] = useState(() => typedName(suggested));
  const [password, setPassword] = useState("");
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        linking.connect({ issuer: isConnected ? "" : issuer.trim(), password, suggested });
      }}
    >
      {isConnected ? null : (
        <>
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
        </>
      )}
      {linking.isPasswordAsked ? (
        <>
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
        </>
      ) : null}
      <button
        type="submit"
        disabled={
          linking.isBusy ||
          (!isConnected && issuer.trim() === "") ||
          (linking.isPasswordAsked && password === "")
        }
      >
        {t("system.discovery.connect")}
      </button>
    </form>
  );
}

/**
 * That remote access needs an account made from an invitation, and whom to ask for one, until the
 * server is claimed: the first beta tester never claimed his, and nothing here said a step was left
 * or that it starts with an invitation (the install report, 2026-11-19).
 */
function NeedsInvitation(): JSX.Element {
  const { t } = useTranslation();
  return <p>{t("system.discovery.invitation")}</p>;
}
