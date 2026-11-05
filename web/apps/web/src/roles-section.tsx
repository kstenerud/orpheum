// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { GridList } from "./grid-list.tsx";
import type { FormWords } from "./problem.ts";
import { isWithoutLibrary, roleLabel, type Role } from "./roles.ts";
import { WarningSign } from "./warning-sign.tsx";

/** How many people hold a role, "1 person" for one (the catalogue has no plural forms: en.ts). */
export function holdersText(t: TFunction, count: number): string {
  return count === 1 ? t("roles.holdersOne") : t("roles.holders", { count });
}

/** How a refusal of a role's name or libraries is told, on the Users page and a role's own. */
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
      <GridList
        rows={props.roles}
        rowKey={(role) => String(role.id)}
        columns={[
          { key: "name", text: "wrap", cell: (role) => roleLabel(t, role.name) },
          { key: "holders", text: "fit", cell: (role) => holdersText(t, role.holderCount) },
        ]}
        target={(role, drawn) => (
          <button
            type="button"
            {...drawn}
            onClick={() => {
              props.onOpen(role);
            }}
          />
        )}
        aside={(role) =>
          isWithoutLibrary(role) ? (
            <WarningSign
              summary={t("warning.roleWithoutLibrary")}
              forAdmin={t("roles.noLibrary")}
            />
          ) : null
        }
      />
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
