// SPDX-License-Identifier: MPL-2.0

// Connecting the core to a discovery, as Connect and the other way share it (D273, D393 §4): a
// discovery not the suggested one shows its identity first, and is sent only once it is confirmed.

import "./system-words.ts";

import type { TFunction } from "i18next";
import { useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { idempotent } from "./idempotent.ts";
import { dataOrThrow, isRefusedAt } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/**
 * What an admin asked to connect to: the discovery typed, "" for the one the core is enrolled with
 * or suggests; their password, "" where none was asked; and the core's suggestion.
 */
export interface Asked {
  readonly issuer: string;
  readonly password: string;
  readonly suggested: string;
}

/** A discovery its issuer showed, waiting for the admin to compare its identity (D393 §4). */
interface ToConfirm {
  readonly asked: Asked;
  readonly identity: string;
  readonly name: string;
}

/** Sends what the admin asked, once the discovery's identity is known: undefined for one needing none. */
type SendAsked = (
  asked: Asked,
  identity: string | undefined,
  idempotencyKey: string,
) => Promise<void>;

/** Connecting to a discovery: the request under way, why it failed, and the identity to confirm. */
export interface Connecting {
  readonly isBusy: boolean;
  readonly problem: string | undefined;
  readonly runRequest: ReturnType<typeof useRequest>["runRequest"];
  readonly toConfirm: ToConfirm | undefined;
  readonly connect: (asked: Asked) => void;
  readonly confirm: () => void;
  readonly cancel: () => void;
}

/**
 * Connecting, by send, which enrols or links: with the suggested discovery, or the one the core is
 * enrolled with, at once; with any other once the identity its issuer shows is confirmed (D393 §4).
 * Why it failed is describe's words for it.
 */
export function useConnecting(
  send: SendAsked,
  describe: (t: TFunction, cause: unknown, asked: Asked) => string,
): Connecting {
  const { t } = useTranslation();
  const { api } = useServices();
  // What the admin last asked to connect to, which says why it was refused.
  const askedRef = useRef<Asked>({ issuer: "", password: "", suggested: "" });
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    describe(t, cause, askedRef.current),
  );
  const [toConfirm, setToConfirm] = useState<ToConfirm | undefined>(undefined);
  const sendAsked = (asked: Asked, identity?: string): void => {
    runRequest(async (idempotencyKey) => {
      await send(asked, identity, idempotencyKey);
      setToConfirm(undefined);
    });
  };
  const connect = (asked: Asked): void => {
    askedRef.current = asked;
    if (isTheSuggested(asked)) {
      sendAsked(asked);
      return;
    }
    runRequest(async (idempotencyKey) => {
      const shown = dataOrThrow(
        await api.POST("/system/discovery/identify", {
          body: { issuer: asked.issuer },
          ...idempotent(idempotencyKey),
        }),
      );
      setToConfirm({ asked, identity: shown.identity, name: shown.name });
    });
  };
  const confirm = (): void => {
    if (toConfirm !== undefined) {
      sendAsked(toConfirm.asked, toConfirm.identity);
    }
  };
  const cancel = (): void => {
    setToConfirm(undefined);
  };
  return { isBusy, problem, runRequest, toConfirm, connect, confirm, cancel };
}

/**
 * Whether what was asked needs no identity confirmed: the discovery the core is enrolled with or
 * suggests ("" asked), or the suggested one typed.
 */
export function isTheSuggested(asked: Asked): boolean {
  return (
    asked.issuer === "" ||
    (asked.suggested !== "" && typedName(asked.issuer) === typedName(asked.suggested))
  );
}

/**
 * The discovery's own refusals, connecting or linking: its identity not the one confirmed, its name
 * no discovery's, or the suggested one's keys unknown to this build; undefined for any other.
 */
export function issuerProblem(t: TFunction, cause: unknown, asked: Asked): string | undefined {
  if (isRefusedAt(cause, "body.identity")) {
    return t("system.discovery.notTheIdentity");
  }
  if (isRefusedAt(cause, "body.issuer")) {
    return isTheSuggested(asked)
      ? t("system.discovery.suggestedUnknown", { issuer: typedName(asked.suggested) })
      : t("system.discovery.notAnIssuer");
  }
  return undefined;
}

/** An issuer as a person types it: its host, without https:// or a closing slash. */
export function typedName(issuer: string): string {
  return issuer
    .trim()
    .replace(/^https:\/\//iu, "")
    .replace(/\/$/u, "")
    .toLowerCase();
}

/**
 * The identity a discovery's issuer showed, for the admin to compare with what its operator shows,
 * and connecting to it only when they confirm.
 */
export function ConfirmIdentity(props: {
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
          issuer: typedName(toConfirm.asked.issuer),
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
