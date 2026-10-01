// SPDX-License-Identifier: MPL-2.0

import "./code-sign-in.css";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { signInWithCode } from "./accounts.ts";
import { CredentialsField, CredentialsForm, CredentialsProblem } from "./credentials-form.tsx";
import { fieldProblemText } from "./problem.ts";
import { useServices } from "./services.tsx";

/**
 * Signs this device in with a code the person made on another device (D218 §4): eight
 * characters, typed however a television's remote allows.
 */
export function CodeSignIn(props: { readonly onBack: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { api, session, navigation } = useServices();
  const [code, setCode] = useState("");
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const submit = (): void => {
    setProblem(undefined);
    signInWithCode(api, code, session.deviceId()).then(
      ({ token, user }) => {
        // A television signed in by code starts at home, not wherever the address last was.
        navigation.go({ screen: "films" });
        session.signIn(token, user.device?.id);
      },
      (cause: unknown) => {
        setProblem(
          fieldProblemText(t, cause, {
            fields: { "body.code": t("codeSignIn.code") },
            overrides: { authRequired: t("codeSignIn.badCode") },
          }),
        );
      },
    );
  };
  return (
    <CredentialsForm heading={t("codeSignIn.heading")} onSubmit={submit}>
      <p>{t("codeSignIn.explain")}</p>
      <CredentialsField label={t("codeSignIn.code")}>
        <input
          className="code-sign-in__code"
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
      </CredentialsField>
      {problem === undefined ? null : <CredentialsProblem isAlert>{problem}</CredentialsProblem>}
      <button type="submit">{t("codeSignIn.submit")}</button>
      <button type="button" onClick={props.onBack}>
        {t("codeSignIn.back")}
      </button>
    </CredentialsForm>
  );
}
