// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import type { FormWords } from "./problem.ts";
import { isWithoutLibrary, roleLabel, type Role } from "./roles.ts";
import { WarningSign } from "./warning-sign.tsx";

/** How a refusal of a role's name or libraries is told, on the People page and a role's own. */
export function roleWords(t: TFunction): FormWords {
  return {
    fields: { "body.name": t("roles.name"), "body.libraries": t("roles.libraries") },
    overrides: { conflict: t("roles.refused") },
  };
}

/**
 * The roles, for the admin (D243 §4, D399): each person has one, their whole access. A row each,
 * with the role's name and how many people have it, opening the role's dialog, where its
 * permissions and libraries are changed (D403).
 */
export function RolesSection(props: {
  readonly roles: readonly Role[];
  readonly onOpen: (role: Role) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <section className="roles" aria-labelledby="roles-heading">
      <h2 id="roles-heading">{t("roles.heading")}</h2>
      <p>{t("roles.explain")}</p>
      <ul className="people__list">
        {props.roles.map((role) => (
          <li key={role.id} className="people__item">
            <button
              type="button"
              className="people__row"
              onClick={() => {
                props.onOpen(role);
              }}
            >
              <span className="people__name">{roleLabel(t, role.name)}</span>
              <span>{t("roles.holders", { count: role.holderCount })}</span>
            </button>
            {isWithoutLibrary(role) ? (
              <WarningSign
                summary={t("warning.roleWithoutLibrary")}
                forAdmin={t("roles.noLibrary")}
              />
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}

/** Deleting a role no one holds, which cannot be undone: red, and asked again (D278 §4). */
export function DeleteRole(props: { readonly onDelete: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <ConfirmButton
      label={t("roles.delete")}
      consequence={t("roles.deleteConsequence")}
      confirmLabel={t("roles.deleteConfirm")}
      onConfirm={props.onDelete}
    />
  );
}

/** A role's name, as the admin types it. */
export function NameField(props: {
  readonly value: string;
  readonly onChange: (name: string) => void;
  readonly onCommit: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <label>
      {t("roles.name")}{" "}
      <input
        required
        maxLength={100}
        value={props.value}
        onChange={(event) => {
          props.onChange(event.target.value);
        }}
        onBlur={props.onCommit}
      />
    </label>
  );
}

/** A set of choices, each ticked or not. */
export function Checklist<T extends string | number>(props: {
  readonly legend: string;
  readonly isDisabled: boolean;
  readonly choices: readonly { readonly value: T; readonly label: string }[];
  readonly chosen: readonly T[];
  readonly onChange: (chosen: readonly T[]) => void;
}): JSX.Element {
  const toggle = (value: T): void => {
    props.onChange(
      props.chosen.includes(value)
        ? props.chosen.filter((each) => each !== value)
        : [...props.chosen, value],
    );
  };
  return (
    <fieldset disabled={props.isDisabled}>
      <legend>{props.legend}</legend>
      {props.choices.map((choice) => (
        <label key={choice.value}>
          <input
            type="checkbox"
            checked={props.chosen.includes(choice.value)}
            onChange={() => {
              toggle(choice.value);
            }}
          />{" "}
          {choice.label}
        </label>
      ))}
    </fieldset>
  );
}

/**
 * Says, beside a choice of role, that the role chosen reaches no library, so whoever is given it
 * would see nothing (the owner's friend, 2026-10-11).
 */
export function NoLibraryNote(props: {
  readonly roles: readonly Role[] | undefined;
  readonly roleName: string;
}): JSX.Element | null {
  const { t } = useTranslation();
  const role = props.roles?.find((each) => each.name === props.roleName);
  if (role === undefined || !isWithoutLibrary(role)) {
    return null;
  }
  return <p>{t("roles.noLibraryChosen")}</p>;
}
