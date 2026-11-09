// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ApiProblem } from "@orpheum/sdk";

import { setupNeeded, signInThisDevice } from "./accounts.ts";
import { CodeSignIn } from "./code-sign-in.tsx";
import {
  CredentialsField,
  CredentialsForm,
  CredentialsProblem,
  PasswordField,
} from "./credentials-form.tsx";
import { fieldProblemText, signedOutText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { SetUp } from "./set-up.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** Why the last session ended, when the server ended it; nothing otherwise. */
export function SessionEndedNotice(props: {
  readonly reason: string | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.reason === undefined) {
    return null;
  }
  return <CredentialsProblem isAlert={false}>{signedOutText(t, props.reason)}</CredentialsProblem>;
}

/** The sign-in form, as it shows: what was typed, whether it is sending, and what went wrong. */
export function SignInForm(props: {
  readonly name: string;
  readonly password: string;
  readonly isSending: boolean;
  /** What went wrong with the last attempt, in the person's words. */
  readonly problem: string | undefined;
  /** Why the last session ended, when the server ended it: said first, so the person knows. */
  readonly lastEndReason?: string | undefined;
  readonly onNameChange: (name: string) => void;
  readonly onPasswordChange: (password: string) => void;
  readonly onSubmit: () => void;
  readonly onUseCode: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <CredentialsForm heading={t("signIn.heading")} onSubmit={props.onSubmit}>
      <SessionEndedNotice reason={props.lastEndReason} />
      <CredentialsField label={t("signIn.name")}>
        <input
          name="name"
          autoComplete="username"
          required
          value={props.name}
          onChange={(event) => {
            props.onNameChange(event.target.value);
          }}
        />
      </CredentialsField>
      <PasswordField
        label={t("signIn.password")}
        value={props.password}
        onChange={props.onPasswordChange}
        autoComplete="current-password"
      />
      {props.problem === undefined ? null : (
        <CredentialsProblem isAlert>{props.problem}</CredentialsProblem>
      )}
      <button type="submit" disabled={props.isSending}>
        {t("signIn.submit")}
      </button>
      <button type="button" onClick={props.onUseCode}>
        {t("signIn.useCode")}
      </button>
    </CredentialsForm>
  );
}

/** Whether a sign-in went no further because the server needs setting up; other failures are thrown. */
async function needsSetUp(signingIn: Promise<void>): Promise<boolean> {
  try {
    await signingIn;
    return false;
  } catch (cause) {
    if (cause instanceof ApiProblem && cause.code === "setupRequired") {
      return true;
    }
    throw cause;
  }
}

/**
 * Signs in, or sets up a server no one can sign in to yet: the server is asked first, so a new one
 * opens on setting up rather than on a sign-in that cannot work (A31).
 */
export function SignIn(): JSX.Element {
  const { api } = useServices();
  const [setup] = useResource("setup", () => setupNeeded(api));
  const isRequired = setup.state === "loaded" && setup.value.isRequired;
  // The address at home to offer: none when it is known, which then needs no asking (D452 §4).
  const offered =
    setup.state === "loaded" && !setup.value.isAddressAtHomeKnown ? setup.value.addressAtHome : "";
  // Keyed by the answer, so a server that turns out to need setting up opens on it.
  return (
    <SignInScreen
      key={String(isRequired)}
      showsSetUpFirst={isRequired}
      offeredAddressAtHome={offered}
    />
  );
}

/** The sign-in form, sent to the server, and the session kept on success; or setting up, or a code. */
function SignInScreen(props: {
  readonly showsSetUpFirst: boolean;
  readonly offeredAddressAtHome: string;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const {
    isBusy: isSending,
    problem,
    runRequest,
  } = useRequest((cause) =>
    fieldProblemText(t, cause, {
      fields: { "body.name": t("signIn.name"), "body.password": t("signIn.password") },
      overrides: {
        authRequired: t("signIn.wrongNameOrPassword"),
        rateLimited: t("signIn.tooManyAttempts"),
        setupRequired: t("signIn.noAdminYet"),
      },
    }),
  );
  const [isUsingCode, setIsUsingCode] = useState(false);
  // A server no one can sign in to yet is set up here, rather than by a request typed by hand (A31).
  const [isSettingUp, setIsSettingUp] = useState(props.showsSetUpFirst);
  if (isUsingCode || isSettingUp) {
    const back = (): void => {
      setIsUsingCode(false);
      setIsSettingUp(false);
    };
    return isSettingUp ? (
      <SetUp onBack={back} offeredAddressAtHome={props.offeredAddressAtHome} />
    ) : (
      <CodeSignIn onBack={back} />
    );
  }
  const submit = (): void => {
    runRequest(async (idempotencyKey) => {
      if (
        await needsSetUp(signInThisDevice({ api, session }, { name, password }, idempotencyKey))
      ) {
        setIsSettingUp(true);
      }
    });
  };
  return (
    <SignInForm
      name={name}
      password={password}
      isSending={isSending}
      problem={problem}
      onNameChange={setName}
      onPasswordChange={setPassword}
      onSubmit={submit}
      lastEndReason={session.lastEndReason()}
      onUseCode={() => {
        setIsUsingCode(true);
      }}
    />
  );
}
