// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { CodeSignIn } from "./code-sign-in.tsx";
import { camelKey } from "./catalogue-key.ts";
import { thisDevice } from "./device.ts";
import {
  CredentialsField,
  CredentialsForm,
  CredentialsProblem,
  PasswordField,
} from "./credentials-form.tsx";
import { useServices } from "./services.tsx";

/** Why a sign-in failed, from the server's status, in words a person can act on. */
export type SignInProblem = "wrongNameOrPassword" | "tooManyAttempts" | "noAdminYet" | "failed";

/** The problem a failed sign-in's status names. */
export function problemForStatus(status: number): SignInProblem {
  switch (status) {
    case 401:
      return "wrongNameOrPassword";
    case 429:
      return "tooManyAttempts";
    case 409:
      return "noAdminYet";
    default:
      return "failed";
  }
}

/** Why the last session ended, when the server ended it; nothing otherwise. */
export function SessionEndedNotice(props: {
  readonly reason: string | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.reason === undefined) {
    return null;
  }
  return (
    <CredentialsProblem isAlert={false}>
      {t(`problem.signedOut.${camelKey(props.reason)}`, {
        defaultValue: t("problem.signedOut.noSession"),
      })}
    </CredentialsProblem>
  );
}

/** The sign-in form, as it shows: what was typed, whether it is sending, and what went wrong. */
export function SignInForm(props: {
  readonly name: string;
  readonly password: string;
  readonly isSending: boolean;
  readonly problem: SignInProblem | undefined;
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
        <CredentialsProblem isAlert>
          {props.problem === "failed"
            ? t("common.failed", { message: "" })
            : t(`signIn.${props.problem}`)}
        </CredentialsProblem>
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
  const { api, session } = useServices();
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [isSending, setIsSending] = useState(false);
  const [problem, setProblem] = useState<SignInProblem | undefined>(undefined);
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
    setIsSending(true);
    setProblem(undefined);
    api.POST("/session", { body: { name, password, device: thisDevice(session) } }).then(
      ({ data: signInBody, response }) => {
        setIsSending(false);
        if (signInBody === undefined) {
          setProblem(problemForStatus(response.status));
          return;
        }
        session.signIn(signInBody.token, signInBody.user.device?.id);
      },
      () => {
        setIsSending(false);
        setProblem("failed");
      },
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
