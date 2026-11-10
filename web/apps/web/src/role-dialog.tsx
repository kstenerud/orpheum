// SPDX-License-Identifier: MPL-2.0

import "./people-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { BusyGroup } from "./busy-group.tsx";
import { FormRow } from "./form-row.tsx";
import { listLibraries, type Library } from "./libraries.tsx";
import { Loaded } from "./loaded.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
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
import { Checklist, DeleteRole, NameField, holdersText, roleWords } from "./roles-section.tsx";
import { useServices } from "./services.tsx";
import { useRequest, type RequestState } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { WarningSign } from "./warning-sign.tsx";

/**
 * The one dialog for a role (D403, revising D399): with no roleId it makes a role, its name,
 * capabilities and libraries chosen, and Add the role makes it and closes; with one it shows that
 * role, each change to its capabilities and libraries saved at once as it is made (D399, revising
 * D243 §4's Save button), its name once the field is left, and Delete, offered only while no one has
 * the role and never for a built-in one, which closes it. onClose is told however it closes, for
 * the page to list the roles again.
 */
export function RoleDialog(props: {
  readonly roleId?: number | undefined;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [roles, reload] = useResource("role-dialog-roles", () => listRoles(api));
  const [libraries] = useResource("role-dialog-libraries", () => listLibraries(api));
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, roleWords(t)),
  );
  const isNew = props.roleId === undefined;
  return (
    <ModalDialog
      label={isNew ? t("roles.newTitle") : t("roles.dialogLabel")}
      shouldFocusFirstField={isNew}
      onClose={props.onClose}
    >
      <BusyGroup isBusy={isBusy}>
        <Loaded
          resource={roles}
          render={(listed) => (
            <Loaded
              resource={libraries}
              render={(libraryList) => (
                <RoleLoaded
                  roleId={props.roleId}
                  listed={listed}
                  libraries={libraryList}
                  runRequest={runRequest}
                  onSaved={reload}
                  onClose={props.onClose}
                />
              )}
            />
          )}
        />
      </BusyGroup>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </ModalDialog>
  );
}

/** The dialog's content once the roles and libraries are loaded: the role asked for, or a new one. */
function RoleLoaded(props: {
  readonly roleId: number | undefined;
  readonly listed: { readonly roles: readonly Role[]; readonly capabilities: readonly string[] };
  readonly libraries: readonly Library[];
  readonly runRequest: RequestState["runRequest"];
  readonly onSaved: () => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { roleId } = props;
  const role = props.listed.roles.find((each) => each.id === roleId);
  if (roleId !== undefined && role === undefined) {
    return <p>{t("roles.notFound")}</p>;
  }
  return (
    <RoleForm
      role={role}
      capabilities={props.listed.capabilities}
      libraries={props.libraries}
      onMake={(change) => {
        props.runRequest(async (idempotencyKey) => {
          await createRole(api, change, idempotencyKey);
          props.onClose();
        });
      }}
      onSave={(change) => {
        props.runRequest(async () => {
          await updateRole(api, roleId ?? 0, change);
          props.onSaved();
        });
      }}
      onDelete={() => {
        props.runRequest(async () => {
          await deleteRole(api, roleId ?? 0);
          props.onClose();
        });
      }}
    />
  );
}

/** The role's fields, as it is now (a new one starts empty), and what the admin asks of them. */
function RoleForm(props: {
  readonly role: Role | undefined;
  readonly capabilities: readonly string[];
  readonly libraries: readonly Library[];
  readonly onMake: (change: RoleChange) => void;
  readonly onSave: (change: RoleChange) => void;
  readonly onDelete: () => void;
}): JSX.Element {
  const { role } = props;
  const draft = useDraft(role, props.onSave);
  const isBuiltin = role?.isBuiltin === true;
  return (
    <FormRow
      onSubmit={() => {
        if (role === undefined) {
          props.onMake(draft.asChange({}));
        } else {
          draft.saveName();
        }
      }}
    >
      <RoleHeading role={role} />
      {isBuiltin ? null : (
        <NameField value={draft.name} onChange={draft.setName} onCommit={draft.saveName} />
      )}
      <RoleCapabilities
        choices={props.capabilities}
        chosen={draft.capabilities}
        isDisabled={isBuiltin}
        onChange={draft.chooseCapabilities}
      />
      <RoleLibraries
        isEveryLibrary={role?.isEveryLibrary === true}
        libraries={props.libraries}
        chosen={draft.libraryIds}
        onChange={draft.chooseLibraries}
      />
      <RoleEnd role={role} onDelete={props.onDelete} />
    </FormRow>
  );
}

/**
 * What a role's form holds: the role as it is, or empty for a new one, and what the admin changes.
 * A listed role is saved as it stands, with only what the admin changed replaced, each choice at
 * once and the name once its field is left; a new one is made by Add the role, all at once.
 */
function useDraft(role: Role | undefined, onSave: (change: RoleChange) => void) {
  const [name, setName] = useState(role?.name ?? "");
  const [capabilities, setCapabilities] = useState<readonly string[]>(role?.capabilities ?? []);
  const [libraryIds, setLibraryIds] = useState<readonly number[]>(
    (role?.libraries ?? []).map((library) => library.libraryId),
  );
  const asChange = (change: Partial<RoleChange>): RoleChange => ({
    name,
    capabilities: [...capabilities],
    libraries: libraryIds.map((libraryId) => ({ libraryId })),
    ...change,
  });
  const saveNow = (change: Partial<RoleChange>): void => {
    if (role !== undefined) {
      onSave(asChange({ name: role.name, ...change }));
    }
  };
  return {
    name,
    setName,
    capabilities,
    libraryIds,
    asChange,
    saveName: (): void => {
      if (role !== undefined && name !== role.name && name !== "") {
        saveNow({ name });
      }
    },
    chooseCapabilities: (chosen: readonly string[]): void => {
      setCapabilities(chosen);
      saveNow({ capabilities: [...chosen] });
    },
    chooseLibraries: (chosen: readonly number[]): void => {
      setLibraryIds(chosen);
      saveNow({ libraries: chosen.map((libraryId) => ({ libraryId })) });
    },
  };
}

/** What a role may do, to tick; fixed for a built-in role. */
function RoleCapabilities(props: {
  readonly choices: readonly string[];
  readonly chosen: readonly string[];
  readonly isDisabled: boolean;
  readonly onChange: (chosen: readonly string[]) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <Checklist
      legend={t("roles.capabilities")}
      isDisabled={props.isDisabled}
      choices={props.choices.map((capability) => ({
        value: capability,
        label: t(capabilityKey(capability)),
      }))}
      chosen={props.chosen}
      onChange={props.onChange}
    />
  );
}

/** The libraries a role reaches, to tick, or that it reaches every one; and the note when none. */
function RoleLibraries(props: {
  readonly isEveryLibrary: boolean;
  readonly libraries: readonly Library[];
  readonly chosen: readonly number[];
  readonly onChange: (chosen: readonly number[]) => void;
}): JSX.Element {
  const { t } = useTranslation();
  if (props.isEveryLibrary) {
    return <p>{t("roles.everyLibrary")}</p>;
  }
  return (
    <>
      <Checklist
        legend={t("roles.libraries")}
        isDisabled={false}
        choices={props.libraries.map((library) => ({ value: library.id, label: library.name }))}
        chosen={props.chosen}
        onChange={props.onChange}
      />
      {props.chosen.length > 0 ? null : <p>{t("roles.noLibrary")}</p>}
    </>
  );
}

/** What ends the form: Add the role for a new one; for a listed one, Delete while it may be. */
function RoleEnd(props: {
  readonly role: Role | undefined;
  readonly onDelete: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { role } = props;
  if (role === undefined) {
    return <button type="submit">{t("roles.create")}</button>;
  }
  return role.isBuiltin || role.holderCount > 0 ? null : <DeleteRole onDelete={props.onDelete} />;
}

/** A role's name and how many hold it (a new one, only its title), with the warning sign when it reaches no library (D278). */
function RoleHeading(props: { readonly role: Role | undefined }): JSX.Element {
  const { t } = useTranslation();
  const { role } = props;
  if (role === undefined) {
    return <h2>{t("roles.newTitle")}</h2>;
  }
  return (
    <>
      <h2>{roleLabel(t, role.name)}</h2>
      {isWithoutLibrary(role) ? (
        <WarningSign summary={t("warning.roleWithoutLibrary")} forAdmin={t("roles.noLibrary")} />
      ) : null}
      <p>{holdersText(t, role.holderCount)}</p>
    </>
  );
}
