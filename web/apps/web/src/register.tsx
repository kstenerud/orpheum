// SPDX-License-Identifier: MPL-2.0

import type { TFunction } from "i18next";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { linkKind, register, secretFromHash, type LinkKind } from "./accounts.ts";
import {
  CredentialsForm,
  CredentialsNotice,
  CredentialsProblem,
  PasswordField,
} from "./credentials-form.tsx";
import { useFieldMarks, type FieldMark } from "./field-marks.ts";
import { failedFields, fieldProblemText, type FormWords } from "./problem.ts";
import { thisDevice } from "./device.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** Why a registration did not go through. */
export type RegisterProblem = "mismatch" | "refused";

/**
 * Registration from a one-time link: an invitation, from which the person chooses their own name
 * and password (D246 §2), or a registration link, from which they choose a new password (D218 §5);
 * either signs this device in as theirs. The link's secret is after #, so no request carried it
 * here.
 */
export function Register(): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const [secret] = useState(() => secretFromHash(navigation.fragment()));
  if (secret === undefined) {
    return (
      <CredentialsNotice heading={t("register.heading")}>
        <p>{t("register.noLink")}</p>
      </CredentialsNotice>
    );
  }
  return <RegisterForm secret={secret} />;
}

/** The registration form for the link whose secret is given. */
function RegisterForm(props: { readonly secret: string }): JSX.Element {
  const { t } = useTranslation();
  const { secret } = props;
  const kind = useLinkKind(secret);
  const isInvitation = kind?.isInvitation === true;
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [passwordAgain, setPasswordAgain] = useState("");
  const registration = useRegistration(secret);
  const marks = useFieldMarks(registration.failure, registration.failure?.fields ?? []);
  return (
    <CredentialsForm
      heading={headingText(t, kind)}
      onSubmit={() => {
        registration.submit({ name: isInvitation ? name : undefined, password, passwordAgain });
      }}
    >
      <p>{explainText(t, kind)}</p>
      {isInvitation ? (
        <NameField
          value={name}
          onChange={marks.editing("body.name", setName)}
          mark={marks.markOf("body.name")}
        />
      ) : null}
      <PasswordField
        label={t("register.password")}
        value={password}
        onChange={marks.editing("body.password", setPassword)}
        mark={marks.markOf("body.password")}
      />
      <PasswordField
        label={t("register.again")}
        value={passwordAgain}
        onChange={marks.editing(passwordAgainField, setPasswordAgain)}
        mark={marks.markOf(passwordAgainField)}
      />
      {registration.problem === undefined ? null : (
        <CredentialsProblem isAlert id={marks.problemId}>
          {t(`register.${registration.problem}`, { refusalText: registration.refusalText })}
        </CredentialsProblem>
      )}
      <button type="submit" disabled={registration.isBusy}>
        {t("register.submit")}
      </button>
    </CredentialsForm>
  );
}

/** What the link is for, once the server has said; undefined until then. */
function useLinkKind(secret: string): LinkKind | undefined {
  const { api } = useServices();
  const [kind] = useResource("link-kind", async () => await linkKind(api, secret));
  return kind.state === "loaded" ? kind.value : undefined;
}

/**
 * The page's heading by what the link is for: an invitation makes an account, a reset link a new
 * password for a person who has one (D466), and a first registration's link their first.
 */
function headingText(t: TFunction, kind: LinkKind | undefined): string {
  if (kind?.isInvitation === true) {
    return t("register.invitedHeading");
  }
  return t(kind?.isReset === true ? "register.resetHeading" : "register.heading");
}

/** What the page says the link is for: an invitation, a person's new password, or a link. */
function explainText(t: TFunction, kind: LinkKind | undefined): string {
  if (kind?.isInvitation === true) {
    return t("register.invitedExplain");
  }
  return kind?.name === undefined
    ? t("register.explain")
    : t("register.explainFor", { name: kind.name });
}

/** The name an invitation's person chooses, which the others see and they sign in with. */
function NameField(props: {
  readonly value: string;
  readonly onChange: (name: string) => void;
  readonly mark: FieldMark;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <label>
      {t("register.name")}{" "}
      <input
        {...props.mark}
        required
        maxLength={100}
        autoComplete="username"
        value={props.value}
        onChange={(event) => {
          props.onChange(event.target.value);
        }}
      />
    </label>
  );
}

/**
 * The field the password is typed again in, which no request carries: a mismatch is about it, and
 * the server never names it.
 */
const passwordAgainField = "passwordAgain";

/**
 * How a refused registration is told: its fields, a bad link (authRequired), which is about no
 * field, and a name taken (conflict), which is about the name.
 */
function registerWords(t: TFunction): FormWords {
  return {
    fields: { "body.password": t("register.password"), "body.name": t("register.name") },
    overrides: { authRequired: t("register.badLink"), conflict: t("register.nameTaken") },
    codeFields: { conflict: ["body.name"] },
  };
}

/** Sending the registration, and why it did not go through. */
function useRegistration(secret: string): {
  readonly submit: (entered: {
    readonly name: string | undefined;
    readonly password: string;
    readonly passwordAgain: string;
  }) => void;
  readonly problem: RegisterProblem | undefined;
  readonly refusalText: string;
  /** The last failure, each a new one, and the fields it is about (useFieldMarks). */
  readonly failure: { readonly fields: readonly string[] } | undefined;
  readonly isBusy: boolean;
} {
  const { t } = useTranslation();
  const { api, session, navigation } = useServices();
  const [problem, setProblem] = useState<RegisterProblem | undefined>(undefined);
  const [failure, setFailure] = useState<{ readonly fields: readonly string[] } | undefined>(
    undefined,
  );
  const [refusalText, setRefusalText] = useState("");
  // The refusal is told by the form's own words; the request only makes the button wait.
  const registering = useRequest(() => "");
  const submit = ({
    name,
    password,
    passwordAgain,
  }: {
    readonly name: string | undefined;
    readonly password: string;
    readonly passwordAgain: string;
  }): void => {
    if (password !== passwordAgain) {
      setProblem("mismatch");
      setFailure({ fields: [passwordAgainField] });
      return;
    }
    setProblem(undefined);
    setFailure(undefined);
    registering.runRequest(async (idempotencyKey) => {
      try {
        const device = thisDevice(session);
        const { token, deviceSecret, locator, user } = await register(
          api,
          name === undefined ? { secret, password, device } : { secret, name, password, device },
          idempotencyKey,
        );
        // The secret rode in the fragment so no request carried it (D218 §1, the one exception
        // to D154's rule). It is spent now, and Films takes its page's place, so it stays in
        // neither the address nor the history, and Back leaves the app.
        session.signIn(token, { deviceId: user.device?.id, deviceSecret, locator });
        navigation.replace({ screen: "home" });
      } catch (cause: unknown) {
        setRefusalText(fieldProblemText(t, cause, registerWords(t)));
        setProblem("refused");
        setFailure({ fields: failedFields(cause, registerWords(t)) });
      }
    });
  };
  return { submit, problem, refusalText, failure, isBusy: registering.isBusy };
}
