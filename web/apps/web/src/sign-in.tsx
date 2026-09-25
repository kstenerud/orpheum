// SPDX-License-Identifier: MPL-2.0

import "./sign-in.css";

import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { useServices } from "./services.tsx";

/** Why a sign-in failed, from the server's status, in words a person can act on. */
export type SignInProblem = "wrong" | "wait" | "setup" | "failed";

/** The problem a failed sign-in's status names. */
export function signInProblem(status: number): SignInProblem {
  switch (status) {
    case 401:
      return "wrong";
    case 429:
      return "wait";
    case 409:
      return "setup";
    default:
      return "failed";
  }
}

/** The sign-in form, as it shows: what was typed, whether it is sending, and what went wrong. */
export function SignInForm(props: {
  readonly name: string;
  readonly password: string;
  readonly isSending: boolean;
  readonly problem: SignInProblem | undefined;
  readonly onName: (name: string) => void;
  readonly onPassword: (password: string) => void;
  readonly onSubmit: () => void;
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
        <label className="sign-in__field">
          {t("signIn.password")}
          <input
            name="password"
            type="password"
            autoComplete="current-password"
            required
            value={props.password}
            onChange={(event) => {
              props.onPassword(event.target.value);
            }}
          />
        </label>
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
  const submit = (): void => {
    setIsSending(true);
    setProblem(undefined);
    api.POST("/session", { body: { name, password } }).then(
      ({ data, response }) => {
        setIsSending(false);
        if (data === undefined) {
          setProblem(signInProblem(response.status));
          return;
        }
        session.signIn(data.token);
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
    />
  );
}
