// SPDX-License-Identifier: MPL-2.0

import "./field.css";

import type { TFunction } from "i18next";
import { useCallback, useId, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { WarningIcon } from "./icons.tsx";

/**
 * What a field carries while the failure shown is about what was typed in it: marked invalid, and
 * described by the line that says why, so a screen reader on the field hears both. Nothing
 * otherwise.
 */
export interface FieldMark {
  readonly "aria-invalid"?: "true";
  readonly "aria-describedby"?: string;
}

/**
 * Where a form stands on one of its fields (useFormFields's fieldOf): the id its control takes, so a
 * press of the form's button can move focus to it; what is wrong with what is in it, said at the
 * field; and its mark while the form's last refusal is about it.
 */
export interface FieldState {
  readonly id?: string | undefined;
  readonly error?: string | undefined;
  readonly mark?: FieldMark | undefined;
}

/** What a Field's control takes, so its label names it and its hint and error describe it. */
export interface FieldControl {
  readonly id: string;
  readonly "aria-describedby"?: string;
  readonly "aria-invalid"?: "true";
}

/** What every Field shows, whatever its control. */
export interface FieldWords extends FieldState {
  /** Its name, short, with no colon. */
  readonly label: string;
  /** A line under the label saying more, its examples included; none if undefined. */
  readonly hint?: string | undefined;
  /** Whether it may be left empty: its label then ends in "(optional)" (D538, interface rule 14). */
  readonly isOptional?: boolean | undefined;
}

/**
 * One input of a form (D538, interface rule 14): its label, then its hint and its error, then its
 * control, read in that order, so the hint and the error are heard before typing and no phone
 * keyboard or autofill list covers them. The control is given what ties them to it (FieldControl):
 * the label's `for` names it, and its `aria-describedby` names the hint, then the error, then the
 * form's refusal about it. It has no placeholder: an example goes in the hint. The error is words
 * with the warning icon, never colour alone, and is not an alert: the press that shows it moves
 * focus to the control, which says it.
 */
export function Field(
  props: FieldWords & { readonly children: (control: FieldControl) => ReactNode },
): JSX.Element {
  const ownId = useId();
  const id = props.id ?? ownId;
  return (
    <div className="field">
      <FieldLabel id={id} label={props.label} isOptional={props.isOptional === true} />
      {props.hint === undefined ? null : (
        <span id={`${id}-hint`} className="field__hint">
          {props.hint}
        </span>
      )}
      {props.error === undefined ? null : (
        <span id={`${id}-error`} className="field__error">
          <WarningIcon />
          {props.error}
        </span>
      )}
      {props.children(controlOf(id, props))}
    </div>
  );
}

/** A field's label, naming its control, id, and saying when it may be left empty. */
function FieldLabel(props: {
  readonly id: string;
  readonly label: string;
  readonly isOptional: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <label className="field__label" htmlFor={props.id}>
      {props.label}
      {props.isOptional ? <span className="field__optional"> {t("field.optional")}</span> : null}
    </label>
  );
}

/**
 * What the control of the field with id carries: its id, what describes it (its hint, its error,
 * the form's refusal about it, in that order) and whether it is invalid.
 */
function controlOf(id: string, state: FieldWords): FieldControl {
  const describedBy = [
    state.hint === undefined ? undefined : `${id}-hint`,
    state.error === undefined ? undefined : `${id}-error`,
    state.mark?.["aria-describedby"],
  ].filter((each) => each !== undefined);
  const isInvalid = state.error !== undefined || state.mark?.["aria-invalid"] === "true";
  return {
    id,
    ...(describedBy.length === 0 ? {} : { "aria-describedby": describedBy.join(" ") }),
    ...(isInvalid ? { "aria-invalid": "true" } : {}),
  };
}

/** A text Field's value, and how its control is typed. */
export interface TextFieldProps extends FieldWords {
  readonly value: string;
  readonly onChange: (value: string) => void;
  readonly name?: string | undefined;
  /** What a browser may fill it with (the forms spec's table; WCAG 1.3.5). */
  readonly autoComplete?: string | undefined;
  /** The keyboard a phone shows: "numeric" for a port, "decimal" for an address. */
  readonly inputMode?: "numeric" | "decimal" | undefined;
  readonly maxLength?: number | undefined;
  /** Whether it is typed as it is, never capitalised or corrected: a household, a name to sign in by. */
  readonly isVerbatim?: boolean | undefined;
  readonly onFocus?: (() => void) | undefined;
}

/** A Field whose control is a line of text: the one way to draw a text input on a form. */
export function TextField(props: TextFieldProps): JSX.Element {
  return (
    <Field {...props}>
      {(control) => (
        <input
          {...control}
          type="text"
          name={props.name}
          autoComplete={props.autoComplete}
          inputMode={props.inputMode}
          maxLength={props.maxLength}
          autoCapitalize={props.isVerbatim === true ? "none" : undefined}
          spellCheck={props.isVerbatim === true ? false : undefined}
          required={props.isOptional !== true}
          value={props.value}
          onFocus={props.onFocus}
          onChange={(event) => {
            props.onChange(event.target.value);
          }}
        />
      )}
    </Field>
  );
}

/** One field's problem a press of the form's button found: the field, and what is wrong, if anything. */
export type FieldProblem = readonly [field: string, problem: string | undefined];

/** A form's fields: where each stands, and the press of its button that checks them. */
export interface FormFields {
  /** The id the line saying what the form's last refusal was carries, which a marked field is described by. */
  readonly problemId: string;
  /** The mark of the field named field, by where the server locates it ("body.name"). */
  readonly markOf: (field: string) => FieldMark;
  /** Where the field named field stands, for its Field: its id, its error and its mark. */
  readonly fieldOf: (field: string) => FieldState;
  /**
   * The field's change handler, onChange, which also takes its error and its mark away: what was
   * wrong is no longer what is typed.
   */
  readonly editing: <V>(field: string, onChange: (value: V) => void) => (value: V) => void;
  /**
   * The check a press of the form's button makes before it does anything (D538, interface rule 14):
   * each problem found shows at its own field and focus moves to the first, which says it; whether
   * there were none, so the press may go on. The button is never disabled for what the form holds.
   */
  readonly check: (problems: readonly FieldProblem[]) => boolean;
}

/**
 * A form's fields' states: the problems its last press of its button found (check), each until its
 * field is edited; and its fields marked by the last refusal, failed (a server's refusal's are
 * failedFields'), aria-invalid and described by the line with problemId, until each is edited or the
 * refusal goes: the form sent again, or another refusal. A failure not about what was typed, the
 * network's or a server busy, names none and marks none. failure is what failed, compared by
 * identity: each failure is a new one.
 */
export function useFormFields(failure: unknown, failed: readonly string[]): FormFields {
  const formId = useId();
  const problemId = `${formId}-problem`;
  const [seen, setSeen] = useState<unknown>(failure);
  const [edited, setEdited] = useState<readonly string[]>([]);
  const [errors, setErrors] = useState<Readonly<Record<string, string>>>({});
  if (seen !== failure) {
    // A new failure, or none: what was edited since the last one no longer counts.
    setSeen(failure);
    setEdited([]);
  }
  const markOf = (field: string): FieldMark =>
    failure !== undefined && failed.includes(field) && !edited.includes(field)
      ? { "aria-invalid": "true", "aria-describedby": problemId }
      : {};
  const idOf = useCallback((field: string) => `${formId}-${field}`, [formId]);
  const fieldOf = (field: string): FieldState => ({
    id: idOf(field),
    error: errors[field],
    mark: markOf(field),
  });
  const editing = useCallback(
    <V,>(field: string, onChange: (value: V) => void) =>
      (value: V): void => {
        setEdited((fields) => (fields.includes(field) ? fields : [...fields, field]));
        setErrors((shown) =>
          field in shown
            ? Object.fromEntries(Object.entries(shown).filter(([each]) => each !== field))
            : shown,
        );
        onChange(value);
      },
    [],
  );
  const check = useCallback(
    (problems: readonly FieldProblem[]): boolean => {
      const found = problems.filter(
        (each): each is readonly [string, string] => each[1] !== undefined,
      );
      setErrors(Object.fromEntries(found));
      const first = found[0];
      if (first !== undefined) {
        document.getElementById(idOf(first[0]))?.focus();
      }
      return first === undefined;
    },
    [idOf],
  );
  return { problemId, markOf, fieldOf, editing, check };
}

/**
 * What a field left empty says, where it has no words of its own (the forms spec): "Enter" and its
 * label, as the label reads in a sentence; undefined while it holds something.
 */
export function missingText(t: TFunction, label: string, value: string): string | undefined {
  return value === "" ? t("field.missing", { label: label.toLocaleLowerCase() }) : undefined;
}
