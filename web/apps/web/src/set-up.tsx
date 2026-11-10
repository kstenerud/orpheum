// SPDX-License-Identifier: MPL-2.0

import "./people-words.ts";

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
 * The sign-in screen opens it when the server answers that it needs setting up. While the server
 * knows no address in the house, it offers its guess to confirm or correct; empty leaves it to be
 * learned from the first page opened at it (D452 §4).
 */
export function SetUp(props: {
  readonly onBack: () => void;
  readonly offeredAddressAtHome?: string;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const [setupCode, setSetupCode] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const isAskingAddress =
    props.offeredAddressAtHome !== undefined && props.offeredAddressAtHome !== "";
  const [addressAtHome, setAddressAtHome] = useState(props.offeredAddressAtHome ?? "");
  const { isBusy, problem, runRequest } = useSetUpRequest();
  const submit = (): void => {
    runRequest(async (idempotencyKey) => {
      await completeSetup(
        api,
        { setupCode, name, password, ...(isAskingAddress ? { addressAtHome } : {}) },
        idempotencyKey,
      );
      await signInThisDevice({ api, session }, { name, password }, idempotencyKey);
    });
  };
  return (
    <CredentialsForm heading={t("setUp.heading")} onSubmit={submit}>
      <CredentialsProblem isAlert>{t("setUp.explain")}</CredentialsProblem>
      <RequiredField
        label={t("setUp.code")}
        name="setupCode"
        autoComplete="one-time-code"
        value={setupCode}
        onChange={setSetupCode}
      />
      <RequiredField
        label={t("signIn.name")}
        name="name"
        autoComplete="username"
        value={name}
        onChange={setName}
      />
      <PasswordField label={t("signIn.password")} value={password} onChange={setPassword} />
      {isAskingAddress ? (
        <AddressAtHomeField value={addressAtHome} onChange={setAddressAtHome} />
      ) : null}
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

/** One required text field of the form. */
function RequiredField(props: {
  readonly label: string;
  readonly name: string;
  readonly autoComplete: "one-time-code" | "username";
  readonly value: string;
  readonly onChange: (value: string) => void;
}): JSX.Element {
  return (
    <CredentialsField label={props.label}>
      <input
        name={props.name}
        autoComplete={props.autoComplete}
        required
        value={props.value}
        onChange={(event) => {
          props.onChange(event.target.value);
        }}
      />
    </CredentialsField>
  );
}

/** The setup request, its refusals in words naming the field or saying what went wrong. */
function useSetUpRequest(): ReturnType<typeof useRequest> {
  const { t } = useTranslation();
  return useRequest((cause) =>
    fieldProblemText(t, cause, {
      fields: {
        "body.setupCode": t("setUp.code"),
        "body.name": t("signIn.name"),
        "body.password": t("signIn.password"),
        "body.addressAtHome": t("setUp.addressAtHome"),
      },
      overrides: { forbidden: t("setUp.wrongCode"), conflict: t("setUp.alreadySetUp") },
    }),
  );
}

/** The server's address in the house, its guess filled in to confirm or correct (D452 §4). */
function AddressAtHomeField(props: {
  readonly value: string;
  readonly onChange: (value: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <CredentialsField label={t("setUp.addressAtHome")}>
      <input
        name="addressAtHome"
        inputMode="decimal"
        value={props.value}
        onChange={(event) => {
          props.onChange(event.target.value);
        }}
      />
    </CredentialsField>
  );
}
