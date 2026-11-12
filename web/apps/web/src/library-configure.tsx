// SPDX-License-Identifier: MPL-2.0

import "./library-configure.css";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { use, useCallback, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { IconButton } from "./icon-button.tsx";
import { GearIcon } from "./icons.tsx";
import { Link } from "./link.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";
import { IsAdminContext } from "./warning-sign.tsx";

/** A library, as the server sends it. */
type LibraryBody = components["schemas"]["LibraryBody"];

/** A person's own choices, as the server sends them. */
type MyPreferencesBody = components["schemas"]["MyPreferencesBody"];

/** The fields of a person's own choices that leave a library out of a place. */
type LeaveOutField = "leftOutLibraryIds" | "leftOutOfHomeLibraryIds";

/**
 * A library's Configure, on its own list of the Libraries page, for everyone (the owner, 2026-10-27):
 * a button opening a dialog of the person's own choices for it, kept on their account, the one place
 * they are made. Show in Films, or in Series (D349 §8, the owner: *"The user can exclude a library from
 * that aggregated view"*), a library of films or of series alone: on, Films or Series gathers it; off, its
 * titles show only on the Libraries page. Show on Home (D366 §5, *"It's a choice each person makes"*):
 * on, Home's rows draw from it; off, it adds nothing to Home. An admin's has Manage this library too,
 * leading to the library's own page, its folders and scans (the owner, 2026-11-08: "'manage this
 * library' should be an option via the gear icon in the library page"); no one else sees it. Escape
 * or a remote's Back closes it.
 */
export function LibraryConfigure(props: { readonly library: LibraryBody }): JSX.Element {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const close = useCallback(() => {
    setIsOpen(false);
  }, []);
  return (
    <>
      <IconButton
        label={t("libraryConfigure.open")}
        icon={<GearIcon />}
        isExpanded={isOpen}
        onClick={() => {
          setIsOpen(true);
        }}
      />
      {isOpen ? (
        <ModalDialog
          label={t("libraryConfigure.heading", { library: props.library.name })}
          onClose={close}
        >
          <LibraryChoices library={props.library} />
          <ManageThisLibrary libraryId={props.library.id} />
        </ModalDialog>
      ) : null}
    </>
  );
}

/** An admin's way from the dialog to the library's own page, where it is managed (D361); else nothing. */
function ManageThisLibrary(props: { readonly libraryId: number }): JSX.Element | null {
  const { t } = useTranslation();
  const isAdmin = use(IsAdminContext);
  if (!isAdmin) {
    return null;
  }
  return (
    <p className="library-configure__manage">
      <Link to={{ screen: "library", libraryId: props.libraryId }}>
        {t("libraryConfigure.manage")}
      </Link>
    </p>
  );
}

/** The dialog's choices for library, once the person's choices are read. */
function LibraryChoices(props: { readonly library: LibraryBody }): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { library } = props;
  const [preferences, reload] = useResource("library-configure-preferences", async () =>
    dataOrThrow(await api.GET("/me/preferences")),
  );
  const [problem, setProblem] = useState<string | undefined>(undefined);
  if (preferences.state !== "loaded") {
    return null;
  }
  const setShown = (field: LeaveOutField, isShown: boolean): void => {
    const next = new Set(leftOutOf(preferences.value, field));
    if (isShown) {
      next.delete(library.id);
    } else {
      next.add(library.id);
    }
    setProblem(undefined);
    api
      .PUT("/me/preferences", { body: { [field]: [...next] } })
      .then(throwIfRefused)
      .then(reload, (cause: unknown) => {
        setProblem(problemText(t, cause));
      });
  };
  const isShownIn = (field: LeaveOutField): boolean =>
    !leftOutOf(preferences.value, field).includes(library.id);
  const gathered = gatheredWords(t, library.kind);
  return (
    <>
      <h2>{library.name}</h2>
      {gathered === undefined ? null : (
        <Choice
          label={gathered.label}
          text={gathered.text}
          isOn={isShownIn("leftOutLibraryIds")}
          onChange={(isOn) => {
            setShown("leftOutLibraryIds", isOn);
          }}
        />
      )}
      <Choice
        label={t("libraryConfigure.showOnHome")}
        text={t("libraryConfigure.showOnHomeText")}
        isOn={isShownIn("leftOutOfHomeLibraryIds")}
        onChange={(isOn) => {
          setShown("leftOutOfHomeLibraryIds", isOn);
        }}
      />
      <p>{t("libraryConfigure.yoursAlone")}</p>
      {problem === undefined ? null : <output>{problem}</output>}
    </>
  );
}

/**
 * What Films and Series gather of a library of kind, and the choice's words: its films, or its
 * series; nothing of an other library's, which neither gathers.
 */
function gatheredWords(
  t: TFunction,
  kind: LibraryBody["kind"],
): { readonly label: string; readonly text: string } | undefined {
  switch (kind) {
    case "film":
      return {
        label: t("libraryConfigure.showInFilms"),
        text: t("libraryConfigure.showInFilmsText"),
      };
    case "series":
      return {
        label: t("libraryConfigure.showInSeries"),
        text: t("libraryConfigure.showInSeriesText"),
      };
    case "other":
      return undefined;
  }
}

/** The libraries a person's choices leave out of the place field names. */
function leftOutOf(preferences: MyPreferencesBody, field: LeaveOutField): readonly number[] {
  return (
    (field === "leftOutLibraryIds"
      ? preferences.leftOutLibraryIds
      : preferences.leftOutOfHomeLibraryIds) ?? []
  );
}

/** One of the dialog's choices: a checkbox, checked while on, and what it does. */
function Choice(props: {
  readonly label: string;
  readonly text: string;
  readonly isOn: boolean;
  readonly onChange: (isOn: boolean) => void;
}): JSX.Element {
  return (
    <div className="library-configure__choice">
      <label>
        <input
          type="checkbox"
          checked={props.isOn}
          onChange={(event) => {
            props.onChange(event.target.checked);
          }}
        />
        {props.label}
      </label>
      <p className="library-configure__text">{props.text}</p>
    </div>
  );
}
