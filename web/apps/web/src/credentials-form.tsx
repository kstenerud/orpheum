// SPDX-License-Identifier: MPL-2.0

import "./credentials-form.css";

import type { JSX, ReactNode } from "react";

import logo from "./assets/orpheum-logo.svg";
import type { FieldMark } from "./field-marks.ts";
import { arrivalMark } from "./focus-navigation.ts";
import { submitWith } from "./form-row.tsx";
import { productName } from "./product-name.ts";

/**
 * The page every way in is: signing in, registering, a device code. The product's mark above one
 * form, headed, in the middle of the page, so the three look the same and share one set of styles.
 * after, where given, comes below the form on the same page, as what happened to copy at a dead end:
 * put after the page instead, it fell below a page a screen tall, and the owner's ThinkPad, told to
 * copy what happened, showed no way to (2026-11-07).
 */
export function CredentialsForm(props: {
  readonly heading: string;
  readonly onSubmit: () => void;
  readonly children: ReactNode;
  readonly after?: ReactNode;
}): JSX.Element {
  return (
    <main className="credentials-form">
      <img className="credentials-form__logo" src={logo} alt={productName} />
      <form
        className="credentials-form__form"
        onSubmit={submitWith(props.onSubmit)}
        {...arrivalMark}
      >
        <h1>{props.heading}</h1>
        {props.children}
      </form>
      {props.after}
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
 * The same page with nothing to fill in: a link that brought no secret says so, and offers no
 * field or button that could not work.
 */
export function CredentialsNotice(props: {
  readonly heading: string;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <main className="credentials-form">
      <img className="credentials-form__logo" src={logo} alt={productName} />
      <section className="credentials-form__form" {...arrivalMark}>
        <h1>{props.heading}</h1>
        {props.children}
      </section>
    </main>
  );
}

/**
 * What went wrong, where the person is looking: an alert for what they just did, or a quiet
 * notice for what happened before they arrived (a session the server ended). id, where given, is
 * the one the fields it is about are described by (useFieldMarks).
 */
export function CredentialsProblem(props: {
  readonly isAlert: boolean;
  readonly id?: string;
  readonly children: ReactNode;
}): JSX.Element {
  return props.isAlert ? (
    <p className="credentials-form__problem" role="alert" id={props.id}>
      {props.children}
    </p>
  ) : (
    <output className="credentials-form__problem" id={props.id}>
      {props.children}
    </output>
  );
}

/** A password field: a new password by default, or the current one when autoComplete says so. */
export function PasswordField(props: {
  readonly label: string;
  readonly value: string;
  readonly onChange: (value: string) => void;
  readonly autoComplete?: "new-password" | "current-password";
  /** Its mark while the failure shown is about it (useFieldMarks); none by default. */
  readonly mark?: FieldMark;
}): JSX.Element {
  return (
    <CredentialsField label={props.label}>
      <input
        {...props.mark}
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
