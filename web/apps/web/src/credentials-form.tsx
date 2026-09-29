// SPDX-License-Identifier: MPL-2.0

import "./credentials-form.css";

import type { JSX, ReactNode, SyntheticEvent } from "react";

/**
 * The page every way in is: signing in, registering, a device code. One form, headed, alone in
 * the middle of the page, so the three look the same and share one set of styles.
 */
export function CredentialsForm(props: {
  readonly heading: string;
  readonly onSubmit: () => void;
  readonly children: ReactNode;
}): JSX.Element {
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    props.onSubmit();
  };
  return (
    <main className="credentials-form">
      <form className="credentials-form__form" onSubmit={submit}>
        <h1>{props.heading}</h1>
        {props.children}
      </form>
    </main>
  );
}

/** One labelled field of a credentials form: the label's text, then its input. */
export function CredentialsField(props: {
  readonly label: string;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <label className="credentials-form__field">
      {props.label}
      {props.children}
    </label>
  );
}

/**
 * What went wrong, where the person is looking: an alert for what they just did, or a quiet
 * notice for what happened before they arrived (a session the server ended).
 */
export function CredentialsProblem(props: {
  readonly isAlert: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  return props.isAlert ? (
    <p className="credentials-form__problem" role="alert">
      {props.children}
    </p>
  ) : (
    <output className="credentials-form__problem">{props.children}</output>
  );
}

/** A password field: a new password by default, or the current one when autoComplete says so. */
export function PasswordField(props: {
  readonly label: string;
  readonly value: string;
  readonly onChange: (value: string) => void;
  readonly autoComplete?: "new-password" | "current-password";
}): JSX.Element {
  return (
    <CredentialsField label={props.label}>
      <input
        type="password"
        autoComplete={props.autoComplete ?? "new-password"}
        required
        value={props.value}
        onChange={(event) => {
          props.onChange(event.target.value);
        }}
      />
    </CredentialsField>
  );
}
