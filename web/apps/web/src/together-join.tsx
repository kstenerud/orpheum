// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  CredentialsField,
  CredentialsForm,
  CredentialsNotice,
  CredentialsProblem,
} from "./credentials-form.tsx";
import { idempotent } from "./idempotent.ts";
import { dataOrThrow, fieldProblemText } from "./problem.ts";
import { useServices, useToken } from "./services.tsx";
import type { Membership } from "./together-storage.ts";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** An address with no link in it, and no session this tab is in: only what to do instead. */
export function NoLink(): JSX.Element {
  const { t } = useTranslation();
  return (
    <CredentialsNotice heading={t("together.joinHeading")}>
      <p>{t("together.noLink")}</p>
    </CredentialsNotice>
  );
}

/**
 * Joining by a link: the name the others will see, and Join (watch-together.md: one Join). Someone
 * signed in here finds their own name already filled in, theirs to change (the owner, 2026-10-06).
 */
export function JoinForm(props: {
  readonly secret: string;
  /** Called at once from the submitting, within the tap that did it. */
  readonly onSubmitted: () => void;
  readonly onJoined: (membership: Membership) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, canFollowAGroup } = useServices();
  const [name, setTypedName] = useJoinName();
  const canFollow = canFollowAGroup();
  useAsksWhetherOutdated(canFollow);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, {
      fields: { "body.name": t("together.name") },
      overrides: { notFound: t("together.badLink") },
    }),
  );
  const { secret } = props;
  if (!canFollow) {
    // Told before joining, so no one waits for a device that could never keep up (D250 §7).
    return (
      <CredentialsNotice heading={t("together.joinHeading")}>
        <p role="alert">{t("together.cannotFollow")}</p>
      </CredentialsNotice>
    );
  }
  return (
    <CredentialsForm
      heading={t("together.joinHeading")}
      onSubmit={() => {
        props.onSubmitted();
        runRequest(async (idempotencyKey) => {
          const joined = dataOrThrow(
            await api.POST("/together/join", {
              body: { secret, name },
              ...idempotent(idempotencyKey),
            }),
          );
          props.onJoined({ sessionId: joined.sessionId, credential: joined.membershipCredential });
        });
      }}
    >
      <p>{t("together.joinExplain")}</p>
      <CredentialsField label={t("together.name")}>
        <input
          required
          maxLength={100}
          autoComplete="nickname"
          value={name}
          onChange={(event) => {
            setTypedName(event.target.value);
          }}
        />
      </CredentialsField>
      {problem === undefined ? null : <CredentialsProblem isAlert>{problem}</CredentialsProblem>}
      <button type="submit" disabled={isBusy}>
        {t("together.join")}
      </button>
    </CredentialsForm>
  );
}

/**
 * Asks the server something as the Join shows, so a page older than the server's client learns it
 * from the answer and reloads before its person types (ReloadWhenOutdated), not after the Join, whose
 * tap the reload loses: an iPhone that opened a new link in a tab of the last build joined, loaded
 * again and asked to Tap to continue (the owner's test, 2026-10-13, navigation=reload
 * joinedHere=false).
 */
function useAsksWhetherOutdated(isJoinShown: boolean): void {
  const { api } = useServices();
  useEffect(() => {
    if (isJoinShown) {
      void api.GET("/health").catch(() => undefined);
    }
  }, [api, isJoinShown]);
}

/**
 * The name to join by: what was typed, once anything was, else the signed-in person's own; a late
 * answer about who is signed in never replaces what was typed.
 */
function useJoinName(): [string, (typed: string) => void] {
  const { api } = useServices();
  const token = useToken();
  const [typedName, setTypedName] = useState<string | undefined>(undefined);
  const [me] = useResource(`join-name:${token ?? ""}`, async () =>
    token === undefined ? undefined : dataOrThrow(await api.GET("/me")),
  );
  return [typedName ?? (me.state === "loaded" ? me.value?.name : undefined) ?? "", setTypedName];
}
