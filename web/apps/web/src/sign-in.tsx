// SPDX-License-Identifier: MPL-2.0

import "./sign-in.css";

import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { CodeSignIn } from "./code-sign-in.tsx";
import { camelKey } from "./catalogue-key.ts";
import { thisDevice } from "./device.ts";
import { PasswordField } from "./register.tsx";
import { useServices } from "./services.tsx";

/** Why a sign-in failed, from the server's status, in words a person can act on. */
export type SignInProblem = "wrongNameOrPassword" | "tooManyAttempts" | "noAdminYet" | "failed";

/** The problem a failed sign-in's status names. */
export function signInProblem(status: number): SignInProblem {
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
export function EndedNotice(props: { readonly reason: string | undefined }): JSX.Element | null {
  const { t } = useTranslation();
  if (props.reason === undefined) {
    return null;
  }
  return (
    <output className="sign-in__problem">
      {t(`problem.signedOut.${camelKey(props.reason)}`, {
        defaultValue: t("problem.signedOut.noSession"),
      })}
    </output>
  );
}

/** The sign-in form, as it shows: what was typed, whether it is sending, and what went wrong. */
export function SignInForm(props: {
  readonly name: string;
  readonly password: string;
  readonly isSending: boolean;
  readonly problem: SignInProblem | undefined;
  /** Why the last session ended, when the server ended it: said first, so the person knows. */
  readonly endedBecause?: string | undefined;
  readonly onName: (name: string) => void;
  readonly onPassword: (password: string) => void;
  readonly onSubmit: () => void;
  readonly onUseCode: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    props.onSubmit();
  };
  return (
    <main className="sign-in">
      <form className="sign-in__form" onSubmit={submit}>
        <h1>{t("signIn.heading")}</h1>
        <EndedNotice reason={props.endedBecause} />
        <label className="sign-in__field">
          {t("signIn.name")}
          <input
            name="name"
            autoComplete="username"
            required
            value={props.name}
            onChange={(event) => {
              props.onName(event.target.value);
            }}
          />
        </label>
        <PasswordField
          label={t("signIn.password")}
          value={props.password}
          onChange={props.onPassword}
          autoComplete="current-password"
        />
        {props.problem === undefined ? null : (
          <p className="sign-in__problem" role="alert">
            {props.problem === "failed"
              ? t("common.failed", { message: "" })
              : t(`signIn.${props.problem}`)}
          </p>
        )}
        <button type="submit" disabled={props.isSending}>
          {t("signIn.submit")}
        </button>
        <button type="button" onClick={props.onUseCode}>
          {t("signIn.useCode")}
        </button>
      </form>
    </main>
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
      ({ data, response }) => {
        setIsSending(false);
        if (data === undefined) {
          setProblem(signInProblem(response.status));
          return;
        }
        session.signIn(data.token, data.user.device?.id);
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
      onName={setName}
      onPassword={setPassword}
      onSubmit={submit}
      endedBecause={session.endedBecause()}
      onUseCode={() => {
        setIsUsingCode(true);
      }}
    />
  );
}
