// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { completeSetup, signInThisDevice } from "./accounts.ts";
import {
  CredentialsField,
  CredentialsForm,
  CredentialsProblem,
  PasswordField,
} from "./credentials-form.tsx";
import { fieldProblemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/**
 * Setting up a server no one can sign in to yet: the one-time setup code it printed, and the first
 * admin's name and password, which then sign this device in (cut-a §5.2's `POST /setup`; A29, A31).
 * The sign-in screen opens it when the server answers that it needs setting up.
 */
export function SetUp(props: { readonly onBack: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const [setupCode, setSetupCode] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, {
      fields: {
        "body.setupCode": t("setUp.code"),
        "body.name": t("signIn.name"),
        "body.password": t("signIn.password"),
      },
      overrides: { forbidden: t("setUp.wrongCode"), conflict: t("setUp.alreadySetUp") },
    }),
  );
  const submit = (): void => {
    runRequest(async (idempotencyKey) => {
      await completeSetup(api, { setupCode, name, password }, idempotencyKey);
      await signInThisDevice({ api, session }, { name, password }, idempotencyKey);
    });
  };
  return (
    <CredentialsForm heading={t("setUp.heading")} onSubmit={submit}>
      <CredentialsProblem isAlert>{t("setUp.explain")}</CredentialsProblem>
      <CredentialsField label={t("setUp.code")}>
        <input
          name="setupCode"
          autoComplete="one-time-code"
          required
          value={setupCode}
          onChange={(event) => {
            setSetupCode(event.target.value);
          }}
        />
      </CredentialsField>
      <CredentialsField label={t("signIn.name")}>
        <input
          name="name"
          autoComplete="username"
          required
          value={name}
          onChange={(event) => {
            setName(event.target.value);
          }}
        />
      </CredentialsField>
      <PasswordField label={t("signIn.password")} value={password} onChange={setPassword} />
      {problem === undefined ? null : <CredentialsProblem isAlert>{problem}</CredentialsProblem>}
      <button type="submit" disabled={isBusy}>
        {t("setUp.submit")}
      </button>
      <button type="button" onClick={props.onBack}>
        {t("setUp.back")}
      </button>
    </CredentialsForm>
  );
}
