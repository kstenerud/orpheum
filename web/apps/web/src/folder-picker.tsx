// SPDX-License-Identifier: MPL-2.0

import "./folder-picker.css";

import { allPages, type Api, type components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Loaded } from "./loaded.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** A place a library folder can be: a node, and a path on it. */
export interface Place {
  readonly nodeName: string;
  readonly path: string;
}

/** The folders in a folder on a node, as the server answers. */
type FolderListing = components["schemas"]["NodeFoldersOutputBody"];

/** What a listing that is not readable says, by its state. */
const unreadableKeys = {
  notAnswering: "libraries.browseNotAnswering",
  missing: "libraries.browseMissing",
  failed: "libraries.browseFailed",
} as const;

/** The folder above path, as far as start, where browsing began: none above start. */
export function folderAbove(path: string, start: string): string | undefined {
  if (path === start) {
    return undefined;
  }
  const cut = path.lastIndexOf("/");
  return cut <= 0 ? "/" : path.slice(0, cut);
}

/** The folder name inside path. */
export function folderInside(path: string, name: string): string {
  return path === "/" ? `/${name}` : `${path}/${name}`;
}

/**
 * Asks the server for the folders in a folder on a node (D359 §2): every page of them, the first
 * page's state and path with them all (D432 §3).
 */
async function listFolders(api: Api, place: Place): Promise<FolderListing> {
  let first: FolderListing | undefined;
  const folders = await allPages(async (query) => {
    const page = dataOrThrow(
      await api.GET("/nodes/{name}/folders", {
        params: { path: { name: place.nodeName }, query: { path: place.path, ...query } },
      }),
    );
    first ??= page;
    return { entries: page.folders ?? [], nextCursor: page.nextCursor };
  });
  if (first === undefined) {
    throw new Error("the server answered no page of folders");
  }
  // The listing is the first page's, its path, state and failure, with every page's folders.
  return {
    path: first.path,
    state: first.state,
    folders,
    ...(first.failure === undefined ? {} : { failure: first.failure }),
  };
}

/**
 * Choosing a library folder (D359): one of a node's configured roots, the starts, or any folder an
 * admin browses to beneath one, as the node lists them, never above the root it began at. A folder
 * whose disk or share does not answer says so, and can be asked again. The new-library form, adding a
 * folder to a library and saying a folder moved all choose with it.
 */
export function FolderPicker(props: {
  readonly starts: readonly Place[];
  readonly onChoose: (place: Place) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [at, setAt] = useState<{ readonly start: Place; readonly path: string } | undefined>();
  if (at === undefined) {
    return (
      <ul>
        {props.starts.map((start) => (
          <li key={`${start.nodeName}:${start.path}`}>
            <button
              type="button"
              onClick={() => {
                setAt({ start, path: start.path });
              }}
            >
              {t("libraries.browseOpen", { node: start.nodeName, path: start.path })}
            </button>
          </li>
        ))}
      </ul>
    );
  }
  const above = folderAbove(at.path, at.start.path);
  return (
    <FolderView
      place={{ nodeName: at.start.nodeName, path: at.path }}
      onOpen={(path) => {
        setAt({ start: at.start, path });
      }}
      onUp={() => {
        setAt(above === undefined ? undefined : { start: at.start, path: above });
      }}
      onChoose={props.onChoose}
    />
  );
}

/**
 * The folder browser as a modal dialog over the page (D361 §3): a button opens it, the admin browses
 * the starts and the folders beneath them as FolderPicker does, and choosing a folder closes it and
 * says which; Cancel, Escape or a remote's Back close it with nothing chosen. It is a ModalDialog, the
 * app's one way to show one, so focus is held inside it while it is open. New-library, adding a folder
 * and saying a folder moved each open it.
 */
export function FolderBrowser(props: {
  /** What the button that opens it says. */
  readonly opener: string;
  /** The dialog's heading, and its name for a screen reader. */
  readonly heading: string;
  readonly starts: readonly Place[];
  readonly onChoose: (place: Place) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const close = (): void => {
    setIsOpen(false);
  };
  return (
    <>
      <button
        type="button"
        onClick={() => {
          setIsOpen(true);
        }}
      >
        {props.opener}
      </button>
      {isOpen ? (
        <ModalDialog label={props.heading} isWide onClose={close}>
          <div className="folder-browser">
            <h2>{props.heading}</h2>
            <FolderPicker
              starts={props.starts}
              onChoose={(place) => {
                close();
                props.onChoose(place);
              }}
            />
            <p>
              <button type="button" onClick={close}>
                {t("libraries.browseCancel")}
              </button>
            </p>
          </div>
        </ModalDialog>
      ) : null}
    </>
  );
}

/** One folder being browsed: where it is, going up, choosing it, and the folders in it to open. */
function FolderView(props: {
  readonly place: Place;
  readonly onOpen: (path: string) => void;
  readonly onUp: () => void;
  readonly onChoose: (place: Place) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { place } = props;
  const [listing, reload] = useResource(`folders:${place.nodeName}:${place.path}`, () =>
    listFolders(api, place),
  );
  return (
    <div>
      <p>
        <code>{t("libraries.root", { node: place.nodeName, path: place.path })}</code>{" "}
        <button type="button" onClick={props.onUp}>
          {t("libraries.browseUp")}
        </button>{" "}
        <button
          type="button"
          onClick={() => {
            props.onChoose(place);
          }}
        >
          {t("libraries.browseChoose")}
        </button>
      </p>
      <Loaded
        resource={listing}
        render={(value) =>
          value.state === "readable" ? (
            <Subfolders
              names={value.folders ?? []}
              onOpen={(name) => {
                props.onOpen(folderInside(place.path, name));
              }}
            />
          ) : (
            <p role="alert">
              {t(unreadableKeys[value.state])}{" "}
              <button type="button" onClick={reload}>
                {t("libraries.browseAgain")}
              </button>
            </p>
          )
        }
      />
    </div>
  );
}

/** The folders in the folder browsed, each opened by a press. */
function Subfolders(props: {
  readonly names: readonly string[];
  readonly onOpen: (name: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  if (props.names.length === 0) {
    return <p>{t("libraries.browseEmpty")}</p>;
  }
  return (
    <ul>
      {props.names.map((name) => (
        <li key={name}>
          <button
            type="button"
            onClick={() => {
              props.onOpen(name);
            }}
          >
            {name}
          </button>
        </li>
      ))}
    </ul>
  );
}
