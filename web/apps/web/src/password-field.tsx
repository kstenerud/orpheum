// SPDX-License-Identifier: MPL-2.0

import "./password-field.css";

import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Field, type FieldWords } from "./field.tsx";
import { rowAttribute } from "./focus-engine.ts";
import { IconButton } from "./icon-button.tsx";
import { EyeIcon } from "./icons.tsx";

/**
 * A password, in a Field (D538, interface rule 14): the one way to ask for one. Its Show password
 * button, beside the input's end edge, shows what was typed as text while pressed (aria-pressed, its
 * name the same either way), so a password being chosen is typed once, with no "same password
 * again" field (the owner, 2026-11-22). The password and the button are one row for a remote's
 * arrows (rowAttribute, which keeps a pop-up holding them open): Right at the end of what was
 * typed reaches the button, Left goes back. A press of the
 * form's button hides it again, so it never stays on screen once sent. autoComplete is what a
 * browser fills it with: the password the person has, or one being chosen (WCAG 1.3.5).
 */
export function PasswordField(
  props: FieldWords & {
    readonly value: string;
    readonly onChange: (value: string) => void;
    readonly autoComplete: "current-password" | "new-password";
    readonly name?: string | undefined;
  },
): JSX.Element {
  const { t } = useTranslation();
  const [isShown, setIsShown] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    const form = inputRef.current?.form;
    if (form === null || form === undefined) {
      return undefined;
    }
    const hide = (): void => {
      setIsShown(false);
    };
    form.addEventListener("submit", hide);
    return () => {
      form.removeEventListener("submit", hide);
    };
  }, []);
  return (
    <Field {...props}>
      {(control) => (
        <div className="password-field" {...{ [rowAttribute]: "" }}>
          <input
            {...control}
            ref={inputRef}
            className="password-field__input"
            type={isShown ? "text" : "password"}
            name={props.name}
            autoComplete={props.autoComplete}
            autoCapitalize="none"
            spellCheck={false}
            required={props.isOptional !== true}
            value={props.value}
            onChange={(event) => {
              props.onChange(event.target.value);
            }}
          />
          <IconButton
            label={t("field.showPassword")}
            icon={<EyeIcon isCrossed={isShown} />}
            isPressed={isShown}
            onClick={() => {
              setIsShown(!isShown);
            }}
          />
        </div>
      )}
    </Field>
  );
}
