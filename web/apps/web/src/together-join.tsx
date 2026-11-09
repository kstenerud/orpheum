// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type Dispatch, type JSX, type SetStateAction } from "react";
import { useTranslation } from "react-i18next";

import {
  CredentialsField,
  CredentialsForm,
  CredentialsNotice,
  CredentialsProblem,
} from "./credentials-form.tsx";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { ApiProblem, dataOrThrow, fieldProblemText } from "./problem.ts";
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

/** The session a person joining is in already: whose, and whether their own (D372 §8). */
interface OtherSession {
  readonly hostName: string;
  readonly isHost: boolean;
}

/** The other session a join was refused for, from its refusal; undefined for any other failure. */
function otherSessionOf(cause: unknown): OtherSession | undefined {
  return cause instanceof ApiProblem ? cause.withCode("inAnotherSession")?.detail : undefined;
}

/**
 * Joining by a link: the name the others will see, and Join (watch-together.md: one Join). Someone
 * signed in here finds their own name already filled in, theirs to change (the owner, 2026-10-06).
 * A person is in one session at most (D372 §8): one in another, signed in or by the membership this
 * browser keeps, heldCredential, is asked "Leave ‹session› to join this one?" first, and leaves it
 * only by saying so.
 */
export function JoinForm(props: {
  readonly secret: string;
  /** The membership this browser keeps, of a session it may be in by link alone. */
  readonly heldCredential?: string | undefined;
  /** Called at once from the submitting, within the tap that did it. */
  readonly onSubmitted: () => void;
  readonly onJoined: (membership: Membership) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { canFollowAGroup } = useServices();
  const [name, setTypedName] = useJoinName();
  const canFollow = canFollowAGroup();
  const linkState = useLinkState(props.secret, canFollow);
  const join = useJoin(props);
  if (!canFollow) {
    // Told before joining, so no one waits for a device that could never keep up (D250 §7).
    return (
      <CredentialsNotice heading={t("together.joinHeading")}>
        <p role="alert">{t("together.cannotFollow")}</p>
      </CredentialsNotice>
    );
  }
  if (linkState !== "admits") {
    return (
      <CredentialsNotice heading={t("together.joinHeading")}>
        {linkState === "refused" ? (
          <p role="alert">{t("together.badLink")}</p>
        ) : (
          <p>{t("together.checkingLink")}</p>
        )}
      </CredentialsNotice>
    );
  }
  const { other, isBusy, problem } = join;
  return (
    <CredentialsForm
      heading={t("together.joinHeading")}
      onSubmit={() => {
        props.onSubmitted();
        join.send(name);
      }}
    >
      <p>{t("together.joinExplain")}</p>
      <JoinNameField name={name} setTypedName={setTypedName} />
      {problem === undefined ? null : <CredentialsProblem isAlert>{problem}</CredentialsProblem>}
      {other === undefined ? (
        <button type="submit" disabled={isBusy}>
          {t("together.join")}
        </button>
      ) : (
        <LeaveOtherToJoin other={other} isBusy={isBusy} onStay={join.stay} />
      )}
    </CredentialsForm>
  );
}

/**
 * Joining by props.secret as a name: the join's request, refused while its person is in another
 * session, which is then the question asked (other); sent again once asked, it says to leave it
 * (D372 §8).
 */
function useJoin(props: {
  readonly secret: string;
  readonly heldCredential?: string | undefined;
  readonly onJoined: (membership: Membership) => void;
}): {
  readonly other: OtherSession | undefined;
  readonly isBusy: boolean;
  readonly problem: string | undefined;
  readonly send: (name: string) => void;
  readonly stay: () => void;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const [other, setOther] = useState<OtherSession | undefined>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, {
      fields: { "body.name": t("together.name") },
      overrides: { notFound: t("together.badLink") },
    }),
  );
  const { secret, heldCredential, onJoined } = props;
  const send = (name: string): void => {
    const shouldLeaveOther = other !== undefined;
    runRequest(async (idempotencyKey) => {
      const answer = await api.POST("/together/join", {
        body: {
          secret,
          name,
          ...(heldCredential === undefined ? {} : { heldCredential }),
          ...(shouldLeaveOther ? { shouldLeaveOther } : {}),
        },
        ...idempotent(idempotencyKey),
      });
      try {
        onJoined(membershipOf(dataOrThrow(answer)));
      } catch (cause) {
        const asked = otherSessionOf(cause);
        if (asked === undefined) {
          throw cause;
        }
        setOther(asked);
      }
    });
  };
  return {
    other,
    isBusy,
    problem,
    send,
    stay: () => {
      setOther(undefined);
    },
  };
}

/**
 * The question a person in another session is asked before joining this one, "Leave ‹session› to
 * join this one?", their own ending for everyone in it (D372 §8): leaving submits the join again,
 * saying so; staying asks nothing more.
 */
function LeaveOtherToJoin(props: {
  readonly other: OtherSession;
  readonly isBusy: boolean;
  readonly onStay: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { other } = props;
  return (
    <>
      <CredentialsProblem isAlert>
        {other.isHost
          ? t("together.leaveOwnToJoin")
          : t("together.leaveOtherToJoin", { host: other.hostName })}
      </CredentialsProblem>
      <button type="submit" disabled={props.isBusy}>
        {t("together.leaveAndJoin")}
      </button>
      <button type="button" disabled={props.isBusy} onClick={props.onStay}>
        {t("together.stayInOther")}
      </button>
    </>
  );
}

/** The name to join by, its field. */
function JoinNameField(props: {
  readonly name: string;
  readonly setTypedName: Dispatch<SetStateAction<string | undefined>>;
}): JSX.Element {
  const { t } = useTranslation();
  const { name, setTypedName } = props;
  return (
    <CredentialsField label={t("together.name")}>
      <input
        required
        maxLength={100}
        autoComplete="nickname"
        value={name}
        onFocus={() => {
          // What it shows as its person starts on it is theirs: a name loaded later replaces nothing,
          // where it went in before what they typed ("viewer-firefoxAnna", check-pr 2026-10-15).
          setTypedName((typed) => typed ?? name);
        }}
        onChange={(event) => {
          setTypedName(event.target.value);
        }}
      />
    </CredentialsField>
  );
}

/** The membership a join's answer gives: its session, its credential, and the locator a discovery finds it by. */
function membershipOf(joined: {
  readonly sessionId: number;
  readonly membershipCredential: string;
  readonly locator: string;
}): Membership {
  return {
    sessionId: joined.sessionId,
    credential: joined.membershipCredential,
    locator: joined.locator,
  };
}

/** What the server says of a link before its form shows: whether it admits, or "asking" meanwhile. */
type LinkState = "asking" | "admits" | "refused";

/**
 * Whether the link of secret would admit its holder now, asked before its form shows where it may
 * (isJoinShown): one refused, as a link used already in another browser, says so in the form's place
 * rather than once its person filled the form in (the owner, 2026-10-30: "ideally it should tell
 * them right off the bat"; VQ172(3)). An asking that fails otherwise shows the form, whose Join says
 * what is wrong. Its answer also tells a page older than the server's client so as the form shows, and
 * it reloads before its person types (ReloadWhenOutdated), not after the Join, whose tap the reload
 * loses: an iPhone that opened a new link in a tab of the last build joined, loaded again and asked
 * to Tap to continue (the owner's test, 2026-10-13, navigation=reload joinedHere=false).
 */
function useLinkState(secret: string, isJoinShown: boolean): LinkState {
  const { api } = useServices();
  const [state, setState] = useState<{ readonly secret: string; readonly state: LinkState }>({
    secret,
    state: "asking",
  });
  useEffect(() => {
    if (!isJoinShown) {
      return undefined;
    }
    let isCurrent = true;
    const tell = (told: LinkState): void => {
      if (isCurrent) {
        setState({ secret, state: told });
      }
    };
    // Keyed as every POST is (D244 §4), though asking twice changes nothing.
    api.POST("/together/join/check", { body: { secret }, ...idempotent(newIdempotencyKey()) }).then(
      ({ response }) => {
        tell(response.status === 404 ? "refused" : "admits");
      },
      () => {
        tell("admits");
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [api, secret, isJoinShown]);
  return state.secret === secret ? state.state : "asking";
}

/**
 * The name to join by: what was typed, or shown as the field was first focused, once either was,
 * else the signed-in person's own; a late answer about who is signed in never replaces either.
 */
function useJoinName(): [string, Dispatch<SetStateAction<string | undefined>>] {
  const { api } = useServices();
  const token = useToken();
  const [typedName, setTypedName] = useState<string | undefined>(undefined);
  const [me] = useResource(`join-name:${token ?? ""}`, async () =>
    token === undefined ? undefined : dataOrThrow(await api.GET("/me")),
  );
  return [typedName ?? (me.state === "loaded" ? me.value?.name : undefined) ?? "", setTypedName];
}
