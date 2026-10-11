// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { BusyGroup } from "./busy-group.tsx";
import { FormRow } from "./form-row.tsx";
import { listLibraries, type Library } from "./libraries.tsx";
import { fieldProblemText } from "./problem.ts";
import {
  capabilityKey,
  createRole,
  deleteRole,
  isWithoutLibrary,
  listRoles,
  roleLabel,
  updateRole,
  type Role,
  type RoleChange,
} from "./roles.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/**
 * The roles, for the admin (D243 §4): each person has one, their whole access. A role's libraries
 * are what its members browse and play; one taken back is gone for them at their next request.
 */
export function RolesSection(props: { readonly onChange?: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [roles, reload] = useResource("roles", () => listRoles(api));
  const [libraries] = useResource("libraries", () => listLibraries(api));
  const { actions, isBusy, problem } = useRoleActions(() => {
    reload();
    props.onChange?.();
  });
  return (
    <section className="roles" aria-labelledby="roles-heading">
      <h2 id="roles-heading">{t("roles.heading")}</h2>
      <p>{t("roles.explain")}</p>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <BusyGroup isBusy={isBusy}>
        <Loaded
          resource={roles}
          render={(listed) => (
            <Loaded
              resource={libraries}
              render={(libraryList) => (
                <>
                  {listed.roles.map((role) => (
                    <RoleEditor
                      key={role.id}
                      role={role}
                      capabilities={listed.capabilities}
                      libraries={libraryList}
                      onSave={(change) => {
                        actions.save(role.id, change);
                      }}
                      onDelete={() => {
                        actions.remove(role.id);
                      }}
                    />
                  ))}
                  <NewRole onCreate={actions.create} />
                </>
              )}
            />
          )}
        />
      </BusyGroup>
    </section>
  );
}

/** What the admin does to roles, one request at a time, and why one failed. */
function useRoleActions(changed: () => void): {
  readonly actions: {
    readonly save: (roleId: number, change: RoleChange) => void;
    readonly remove: (roleId: number) => void;
    readonly create: (name: string) => void;
  };
  readonly isBusy: boolean;
  readonly problem: string | undefined;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, {
      fields: { "body.name": t("roles.name"), "body.libraries": t("roles.libraries") },
      overrides: { conflict: t("roles.refused") },
    }),
  );
  const actions = {
    save: (roleId: number, change: RoleChange): void => {
      runRequest(async () => {
        await updateRole(api, roleId, change);
        changed();
      });
    },
    remove: (roleId: number): void => {
      runRequest(async () => {
        await deleteRole(api, roleId);
        changed();
      });
    },
    create: (name: string): void => {
      runRequest(async (idempotencyKey) => {
        await createRole(api, { name, capabilities: [], libraries: [] }, idempotencyKey);
        changed();
      });
    },
  };
  return { actions, isBusy, problem };
}

/**
 * One role: its name and capabilities, which a built-in role keeps, and the libraries it reaches,
 * which the admin role does not list: it reaches every one.
 */
function RoleEditor(props: {
  readonly role: Role;
  readonly capabilities: readonly string[];
  readonly libraries: readonly Library[];
  readonly onSave: (change: RoleChange) => void;
  readonly onDelete: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { role } = props;
  const [name, setName] = useState(role.name);
  const [capabilities, setCapabilities] = useState<readonly string[]>(role.capabilities ?? []);
  const [libraryIds, setLibraryIds] = useState<readonly number[]>(
    (role.libraries ?? []).map((library) => library.libraryId),
  );
  const save = (): void => {
    props.onSave({
      name,
      capabilities: [...capabilities],
      libraries: libraryIds.map((libraryId) => ({ libraryId })),
    });
  };
  return (
    <FormRow onSubmit={save}>
      <h3>{roleLabel(t, role.name)}</h3>
      <p>{t("roles.holders", { count: role.holderCount })}</p>
      {role.isBuiltin ? null : <NameField value={name} onChange={setName} />}
      <Checklist
        legend={t("roles.capabilities")}
        isDisabled={role.isBuiltin}
        choices={props.capabilities.map((capability) => ({
          value: capability,
          label: t(capabilityKey(capability)),
        }))}
        chosen={capabilities}
        onChange={setCapabilities}
      />
      {role.isEveryLibrary ? (
        <p>{t("roles.everyLibrary")}</p>
      ) : (
        <Checklist
          legend={t("roles.libraries")}
          isDisabled={false}
          choices={props.libraries.map((library) => ({ value: library.id, label: library.name }))}
          chosen={libraryIds}
          onChange={setLibraryIds}
        />
      )}
      {role.isEveryLibrary || libraryIds.length > 0 ? null : <p>{t("roles.noLibrary")}</p>}
      {role.isEveryLibrary && role.isBuiltin ? null : (
        <button type="submit">{t("roles.save")}</button>
      )}
      {role.isBuiltin ? null : (
        <button type="button" onClick={props.onDelete} disabled={role.holderCount > 0}>
          {t("roles.delete")}
        </button>
      )}
    </FormRow>
  );
}

/** A role's name, as the admin types it. */
function NameField(props: {
  readonly value: string;
  readonly onChange: (name: string) => void;
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
      />
    </label>
  );
}

/** A set of choices, each ticked or not. */
function Checklist<T extends string | number>(props: {
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

/** Makes a role, by name; its capabilities and libraries are chosen once it is listed. */
function NewRole(props: { readonly onCreate: (name: string) => void }): JSX.Element {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  return (
    <FormRow
      onSubmit={() => {
        props.onCreate(name);
        setName("");
      }}
    >
      <label>
        {t("roles.newName")}{" "}
        <input
          required
          maxLength={100}
          value={name}
          onChange={(event) => {
            setName(event.target.value);
          }}
        />
      </label>
      <button type="submit">{t("roles.create")}</button>
    </FormRow>
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
