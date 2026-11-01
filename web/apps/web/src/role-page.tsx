// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { BusyGroup } from "./busy-group.tsx";
import { FormRow } from "./form-row.tsx";
import { listLibraries, type Library } from "./libraries.tsx";
import { Loaded } from "./loaded.tsx";
import { fieldProblemText } from "./problem.ts";
import {
  capabilityKey,
  deleteRole,
  isWithoutLibrary,
  listRoles,
  roleLabel,
  updateRole,
  type Role,
  type RoleChange,
} from "./roles.ts";
import { Checklist, DeleteRole, NameField, roleWords } from "./roles-section.tsx";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { WarningSign } from "./warning-sign.tsx";

/**
 * One role, for the admin (D399, revising D243 §4's Save button): its capabilities and the libraries
 * it reaches, each change saved at once as it is made; and Delete, offered only while no one has the
 * role, and never for a built-in one.
 */
export function RolePage(props: { readonly roleId: number }): JSX.Element {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const [roles, reload] = useResource("roles", () => listRoles(api));
  const [libraries] = useResource("libraries", () => listLibraries(api));
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, roleWords(t)),
  );
  return (
    <section className="role" aria-labelledby="role-heading">
      <BusyGroup isBusy={isBusy}>
        <Loaded
          resource={roles}
          render={(listed) => (
            <Loaded
              resource={libraries}
              render={(libraryList) => {
                const role = listed.roles.find((each) => each.id === props.roleId);
                return role === undefined ? (
                  <p>{t("roles.notFound")}</p>
                ) : (
                  <RoleControls
                    role={role}
                    capabilities={listed.capabilities}
                    libraries={libraryList}
                    onSave={(change) => {
                      runRequest(async () => {
                        await updateRole(api, role.id, change);
                        reload();
                      });
                    }}
                    onDelete={() => {
                      runRequest(async () => {
                        await deleteRole(api, role.id);
                        navigation.go({ screen: "people" });
                      });
                    }}
                  />
                );
              }}
            />
          )}
        />
      </BusyGroup>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </section>
  );
}

/** What a role's page changes: the role as it is now, and what the admin asks of it. */
function RoleControls(props: {
  readonly role: Role;
  readonly capabilities: readonly string[];
  readonly libraries: readonly Library[];
  readonly onSave: (change: RoleChange) => void;
  readonly onDelete: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { role } = props;
  const [name, setName] = useState(role.name);
  const capabilities = role.capabilities ?? [];
  const libraryIds = (role.libraries ?? []).map((library) => library.libraryId);
  // The role as it stands, with only what the admin changed replaced.
  const save = (change: Partial<RoleChange>): void => {
    props.onSave({
      name: role.name,
      capabilities: [...capabilities],
      libraries: libraryIds.map((libraryId) => ({ libraryId })),
      ...change,
    });
  };
  const saveName = (): void => {
    if (name !== role.name && name !== "") {
      save({ name });
    }
  };
  return (
    <FormRow onSubmit={saveName}>
      <RoleHeading role={role} />
      {role.isBuiltin ? null : <NameField value={name} onChange={setName} onCommit={saveName} />}
      <Checklist
        legend={t("roles.capabilities")}
        isDisabled={role.isBuiltin}
        choices={props.capabilities.map((capability) => ({
          value: capability,
          label: t(capabilityKey(capability)),
        }))}
        chosen={capabilities}
        onChange={(chosen) => {
          save({ capabilities: [...chosen] });
        }}
      />
      {role.isEveryLibrary ? (
        <p>{t("roles.everyLibrary")}</p>
      ) : (
        <Checklist
          legend={t("roles.libraries")}
          isDisabled={false}
          choices={props.libraries.map((library) => ({ value: library.id, label: library.name }))}
          chosen={libraryIds}
          onChange={(chosen) => {
            save({ libraries: chosen.map((libraryId) => ({ libraryId })) });
          }}
        />
      )}
      {role.isEveryLibrary || libraryIds.length > 0 ? null : <p>{t("roles.noLibrary")}</p>}
      {role.isBuiltin || role.holderCount > 0 ? null : <DeleteRole onDelete={props.onDelete} />}
    </FormRow>
  );
}

/** A role's name and how many hold it, with the warning sign when it reaches no library (D278). */
function RoleHeading(props: { readonly role: Role }): JSX.Element {
  const { t } = useTranslation();
  const { role } = props;
  return (
    <>
      <h1 id="role-heading">{roleLabel(t, role.name)}</h1>
      {isWithoutLibrary(role) ? (
        <WarningSign summary={t("warning.roleWithoutLibrary")} forAdmin={t("roles.noLibrary")} />
      ) : null}
      <p>{t("roles.holders", { count: role.holderCount })}</p>
    </>
  );
}
