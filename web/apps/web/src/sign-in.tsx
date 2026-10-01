// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { signIn } from "./accounts.ts";
import { CodeSignIn } from "./code-sign-in.tsx";
import { thisDevice } from "./device.ts";
import {
  CredentialsField,
  CredentialsForm,
  CredentialsProblem,
  PasswordField,
} from "./credentials-form.tsx";
import { fieldProblemText, signedOutText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

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

/** Signs in: the form, sent to the server, and the session kept on success. */
export function SignIn(): JSX.Element {
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
  if (isUsingCode) {
    return (
      <CodeSignIn
        onBack={() => {
          setIsUsingCode(false);
        }}
      />
    );
  }
  const submit = (): void => {
    runRequest(
      signIn(api, { name, password, device: thisDevice(session) }).then(({ token, user }) => {
        session.signIn(token, user.device?.id);
      }),
    );
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
