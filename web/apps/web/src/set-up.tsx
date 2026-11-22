// SPDX-License-Identifier: MPL-2.0

import "./people-words.ts";

import type { TFunction } from "i18next";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { completeSetup, signInThisDevice } from "./accounts.ts";
import { CredentialsForm } from "./credentials-form.tsx";
import {
  missingText,
  TextField,
  useFormFields,
  type FieldProblem,
  type FieldState,
} from "./field.tsx";
import { PasswordField } from "./password-field.tsx";
import { failedFields, fieldProblemText, type FormWords } from "./problem.ts";
import { setupCodeFromFragment } from "./set-up-link.ts";
import { useServices } from "./services.tsx";
import { SubmitButton } from "./submit-button.tsx";
import { useRequest } from "./use-request.ts";
import { StatusNote } from "./status-note.tsx";

/**
 * Setting up a server no one can sign in to yet: the one-time setup code it printed, and the first
 * admin's name and password, which then sign this device in (cut-a §5.2's `POST /setup`; A29, A31).
 * The sign-in screen opens it when the server answers that it needs setting up. While the server
 * knows no address in the house, it offers its guess to confirm or correct; empty leaves it to be
 * learned from the first page opened at it (D452 §4). The link the server prints carries the code after
 * a # (D527 §1.2): the page fills it in, so a person types only a name and a password, and takes the
 * # out of the address at once, so the code is not left in the address bar or the history.
 */
export function SetUp(props: {
  readonly onBack: () => void;
  readonly offeredAddressAtHome?: string;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, session, navigation } = useServices();
  const linkedCode = useLinkedCode();
  const [setupCode, setSetupCode] = useState(linkedCode ?? "");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const isAskingAddress =
    props.offeredAddressAtHome !== undefined && props.offeredAddressAtHome !== "";
  const [addressAtHome, setAddressAtHome] = useState(props.offeredAddressAtHome ?? "");
  const { isBusy, problem, failure, runRequest } = useSetUpRequest();
  const fields = useFormFields(failure, failedFields(failure, setUpWords(t)));
  const field = (key: string, value: string, onChange: (value: string) => void): Field => ({
    ...fields.fieldOf(`body.${key}`),
    value,
    onChange: fields.editing(`body.${key}`, onChange),
  });
  const submit = (): void => {
    if (!fields.check(missingOf(t, { setupCode, name, password }))) {
      return;
    }
    runRequest(async (idempotencyKey) => {
      await completeSetup(
        api,
        { setupCode, name, password, ...(isAskingAddress ? { addressAtHome } : {}) },
        idempotencyKey,
      );
      await signInThisDevice({ api, session }, { name, password }, idempotencyKey);
      // The first admin's first pages: libraries, then posters, then discovery (D527 §3).
      navigation.replace({ screen: "welcome" });
    });
  };
  return (
    <CredentialsForm heading={t("setUp.heading")} onSubmit={submit}>
      <p>
        <output>{linkedCode === undefined ? t("setUp.explain") : t("setUp.explainLinked")}</output>
      </p>
      <CodeAndAdmin
        setupCode={field("setupCode", setupCode, setSetupCode)}
        name={field("name", name, setName)}
        password={field("password", password, setPassword)}
      />
      {isAskingAddress ? (
        <AddressAtHomeField {...field("addressAtHome", addressAtHome, setAddressAtHome)} />
      ) : null}
      {problem === undefined ? null : (
        <StatusNote told={{ kind: "problem", text: problem, id: fields.problemId }} />
      )}
      <SubmitButton label={t("setUp.submit")} isBusy={isBusy} />
      <button type="button" onClick={props.onBack}>
        {t("setUp.back")}
      </button>
    </CredentialsForm>
  );
}

/** What a setup sent now would lack: each field it needs left empty, to be said at that field. */
function missingOf(
  t: TFunction,
  typed: { readonly setupCode: string; readonly name: string; readonly password: string },
): readonly FieldProblem[] {
  return [
    ["body.setupCode", missingText(t, t("setUp.code"), typed.setupCode)],
    ["body.name", missingText(t, t("signIn.name"), typed.name)],
    ["body.password", missingText(t, t("signIn.password"), typed.password)],
  ];
}

/** The code the address carried (D527 §1.2), read once; the address is rewritten without it at once. */
function useLinkedCode(): string | undefined {
  const { navigation } = useServices();
  const [linkedCode] = useState(() => setupCodeFromFragment(navigation.fragment()));
  useEffect(() => {
    if (linkedCode !== undefined) {
      navigation.replace(navigation.route());
    }
  }, [linkedCode, navigation]);
  return linkedCode;
}

/** A field's value, its change handler, and where it stands: its id, its error and its mark. */
interface Field extends FieldState {
  readonly value: string;
  readonly onChange: (value: string) => void;
}

/** The setup code the server printed, and the first admin's name and password. */
function CodeAndAdmin(props: {
  readonly setupCode: Field;
  readonly name: Field;
  readonly password: Field;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <TextField
        label={t("setUp.code")}
        name="setupCode"
        autoComplete="one-time-code"
        {...props.setupCode}
      />
      <TextField label={t("signIn.name")} name="name" autoComplete="username" {...props.name} />
      <PasswordField label={t("signIn.password")} autoComplete="new-password" {...props.password} />
    </>
  );
}

/** How a refused setup is told: its fields, and a wrong code (forbidden) about the code's. */
function setUpWords(t: TFunction): FormWords {
  return {
    fields: {
      "body.setupCode": t("setUp.code"),
      "body.name": t("signIn.name"),
      "body.password": t("signIn.password"),
      "body.addressAtHome": t("setUp.addressAtHome"),
    },
    overrides: { forbidden: t("setUp.wrongCode"), conflict: t("setUp.alreadySetUp") },
    codeFields: { forbidden: ["body.setupCode"] },
  };
}

/** The setup request, its refusals in words naming the field or saying what went wrong. */
function useSetUpRequest(): ReturnType<typeof useRequest> {
  const { t } = useTranslation();
  return useRequest((cause) => fieldProblemText(t, cause, setUpWords(t)));
}

/**
 * The server's address in the house, its guess filled in to confirm or correct (D452 §4); left
 * empty, it is learned from the first page opened at it.
 */
function AddressAtHomeField(props: Field): JSX.Element {
  const { t } = useTranslation();
  return (
    <TextField
      {...props}
      label={t("setUp.addressAtHome")}
      hint={t("setUp.addressAtHomeHint")}
      name="addressAtHome"
      inputMode="decimal"
      isOptional
    />
  );
}
