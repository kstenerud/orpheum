// SPDX-License-Identifier: MPL-2.0

import "./sign-in.css";

import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { signInWithCode } from "./accounts.ts";
import { useServices } from "./services.tsx";

/**
 * Signs this device in with a code the person made on another device (D218 §4): eight
 * characters, typed however a television's remote allows.
 */
export function CodeSignIn(props: { readonly onBack: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const [code, setCode] = useState("");
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    setProblem(undefined);
    signInWithCode(api, code).then(
      ({ token }) => {
        session.signIn(token);
      },
      (cause: unknown) => {
        setProblem(cause instanceof Error ? cause.message : String(cause));
      },
    );
  };
  return (
    <main className="sign-in">
      <form className="sign-in__form" onSubmit={submit}>
        <h1>{t("codeSignIn.heading")}</h1>
        <p>{t("codeSignIn.explain")}</p>
        <label className="sign-in__field">
          {t("codeSignIn.code")}
          <input
            className="sign-in__code"
            autoComplete="one-time-code"
            autoCapitalize="none"
            spellCheck={false}
            required
            maxLength={12}
            value={code}
            onChange={(event) => {
              setCode(event.target.value);
            }}
          />
        </label>
        {problem === undefined ? null : (
          <p className="sign-in__problem" role="alert">
            {problem}
          </p>
        )}
        <button type="submit">{t("codeSignIn.submit")}</button>
        <button type="button" onClick={props.onBack}>
          {t("codeSignIn.back")}
        </button>
      </form>
    </main>
  );
}
