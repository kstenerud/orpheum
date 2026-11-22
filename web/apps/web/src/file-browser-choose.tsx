// SPDX-License-Identifier: MPL-2.0

import { useEffect, useLayoutEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { ActionRow } from "./action-row.tsx";
import { TextField } from "./field.tsx";
import {
  FileBrowser,
  folderAbove,
  folderInside,
  folderName,
  inNaturalOrder,
  listFolders,
  type Crumb,
  type FileBrowserShown,
  type FileGroup,
  type FileRow,
  type FolderListing,
  type Place,
} from "./file-browser.tsx";
import { submitWith } from "./form-row.tsx";
import { isOnTelevision } from "./input-mode.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { useServices } from "./services.tsx";
import { StatusNote } from "./status-note.tsx";
import { SubmitButton } from "./submit-button.tsx";
import type { Resource } from "./resource.ts";
import { useResource } from "./use-resource.ts";

/** Where Choose is: inside one of the places, at a folder in it; undefined at the places. */
interface ChooseAt {
  readonly start: Place;
  readonly path: string;
  /** Whether it is still finding the nearest folder that exists to a folder it was opened at. */
  readonly isSeeking?: boolean;
  /** The folder it came up from, which focus arrives on. */
  readonly cameFrom?: string;
}

/** What Choose is told: where it may go, and what to do with the folder chosen. */
export interface ChooseProps {
  /** The dialog's heading, and its name for a screen reader: "Choose a folder for Films". */
  readonly heading: string;
  /** Every place of every machine that answers, the folders their configuration lists. */
  readonly places: readonly Place[];
  /** The machines that are not answering, each said in place of its places. */
  readonly away?: readonly string[] | undefined;
  /** The folder to open at, as when changing one: it, or the nearest folder above that exists. */
  readonly startAt?: Place | undefined;
  readonly onChoose: (place: Place) => void;
  readonly onClose: () => void;
}

/** The place a folder is in: the one whose path holds it, the longest where several do. */
function placeHolding(places: readonly Place[], folder: Place): Place | undefined {
  return places
    .filter(
      (place) =>
        place.nodeName === folder.nodeName &&
        (folder.path === place.path ||
          folder.path.startsWith(place.path === "/" ? "/" : `${place.path}/`)),
    )
    .toSorted((one, other) => other.path.length - one.path.length)[0];
}

/** Where Choose opens: at the folder it was given, inside the one place there is, else at the places. */
function openingAt(props: ChooseProps): ChooseAt | undefined {
  if (props.startAt !== undefined) {
    const start = placeHolding(props.places, props.startAt);
    if (start !== undefined) {
      return { start, path: props.startAt.path, isSeeking: true };
    }
  }
  const [only, ...others] = props.places;
  return only !== undefined && others.length === 0 && (props.away ?? []).length === 0
    ? { start: only, path: only.path }
    : undefined;
}

/**
 * Choose, the file browser's dialog that picks a folder on a machine (D539 §2): it starts at the
 * places, grouped by machine, a machine not answering said in their place, or inside the one place
 * there is, or at the folder it is changing; it opens folders, never above a place, and its main
 * button uses the folder it is in. Back and Escape go up one folder, closing it only at the places
 * (§4); a press outside closes it. Go to… opens a typed path, off a television.
 */
export function Choose(props: ChooseProps): JSX.Element {
  const { t } = useTranslation();
  const [at, setAt] = useState<ChooseAt | undefined>(() => openingAt(props));
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const [goTo, setGoTo] = useState<string | undefined>(undefined);
  const up = upOf(at);
  const go = (next: ChooseAt | undefined): void => {
    setProblem(undefined);
    setAt(next);
  };
  const onBack = (): boolean => {
    if (goTo !== undefined) {
      setGoTo(undefined);
      return true;
    }
    if (at === undefined) {
      return false;
    }
    go(up);
    return true;
  };
  const choose = (): void => {
    if (at === undefined) {
      setProblem(t("fileBrowser.openFirst"));
      return;
    }
    props.onClose();
    props.onChoose({ nodeName: at.start.nodeName, path: at.path });
  };
  return (
    <ModalDialog
      label={props.heading}
      heading={props.heading}
      isLarge
      onClose={props.onClose}
      onBack={onBack}
      footer={<ChooseFoot at={at} problem={problem} onChoose={choose} onClose={props.onClose} />}
    >
      {at === undefined ? (
        <ChoosePlaces
          places={props.places}
          away={props.away ?? []}
          goTo={goTo}
          onGoTo={setGoTo}
          onGo={go}
        />
      ) : (
        <ChooseFolder
          key={`${at.start.nodeName}:${at.path}`}
          at={at}
          places={props.places}
          goTo={goTo}
          onGoTo={setGoTo}
          onGo={go}
        />
      )}
    </ModalDialog>
  );
}

/** Choose's foot: what went wrong in choosing, Use the folder it is in, and Cancel. */
function ChooseFoot(props: {
  readonly at: ChooseAt | undefined;
  readonly problem: string | undefined;
  readonly onChoose: () => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { at, problem } = props;
  return (
    <form className="file-browser__foot" onSubmit={submitWith(props.onChoose)}>
      {problem === undefined ? null : <StatusNote told={{ kind: "problem", text: problem }} />}
      <ActionRow
        main={
          <SubmitButton
            label={
              at === undefined
                ? t("fileBrowser.useThis")
                : t("fileBrowser.use", { folder: folderName(at.path) })
            }
            isBusy={false}
          />
        }
        secondary={
          <button type="button" onClick={props.onClose}>
            {t("dialog.cancel")}
          </button>
        }
      />
    </form>
  );
}

/** Where up one folder from at is: the folder above, as far as its place; above a place, the places. */
function upOf(at: ChooseAt | undefined): ChooseAt | undefined {
  if (at === undefined) {
    return undefined;
  }
  const above = folderAbove(at.path, at.start.path);
  return above === undefined
    ? undefined
    : { start: at.start, path: above, cameFrom: folderName(at.path) };
}

/** What Go to… and the folders Choose shows are told: the typed path, and where to go. */
interface ChooseMoves {
  readonly places: readonly Place[];
  readonly goTo: string | undefined;
  readonly onGoTo: (typed: string | undefined) => void;
  readonly onGo: (at: ChooseAt | undefined) => void;
}

/** Choose at its places: each machine's places under its name, one not answering said in their place. */
function ChoosePlaces(
  props: Omit<ChooseMoves, "places"> & {
    readonly places: readonly Place[];
    readonly away: readonly string[];
  },
): JSX.Element {
  const { t } = useTranslation();
  const machines = [...new Set(props.places.map((place) => place.nodeName))];
  const groups: FileGroup[] = [
    ...inNaturalOrder(machines, (name) => name).map((machine) => ({
      key: machine,
      heading: machine,
      rows: inNaturalOrder(
        props.places.filter((place) => place.nodeName === machine),
        (place) => place.path,
      ).map((place): FileRow => ({
        key: `${place.nodeName}:${place.path}`,
        kind: "place",
        name: folderName(place.path),
        under: place.path,
        onOpen: () => {
          props.onGo({ start: place, path: place.path });
        },
      })),
    })),
    ...inNaturalOrder(props.away, (name) => name).map((machine) => ({
      key: `away:${machine}`,
      heading: machine,
      rows: [],
      told: { kind: "problem" as const, text: t("fileBrowser.notAnswering", { machine }) },
    })),
  ];
  return (
    <ChooseBrowser
      crumbs={[{ key: "places", name: t("fileBrowser.places"), onOpen: () => undefined }]}
      shown={{ kind: "listed", groups, empty: { kind: "empty", text: t("fileBrowser.noPlaces") } }}
      moves={{ ...props, places: props.places }}
      at={undefined}
    />
  );
}

/** Choose inside a place: the folders in the folder it is at, as the machine lists them. */
function ChooseFolder(props: ChooseMoves & { readonly at: ChooseAt }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { at } = props;
  const [listing, reload] = useResource(`choose-folder/${at.start.nodeName}:${at.path}`, () =>
    listFolders(api, { nodeName: at.start.nodeName, path: at.path }),
  );
  const above = folderAbove(at.path, at.start.path);
  const { onGo } = props;
  // Opened at a folder that has gone, it goes up until one is there (D539 §2).
  const isGone =
    at.isSeeking === true && listing.state === "loaded" && listing.value.state === "missing";
  useEffect(() => {
    if (isGone && above !== undefined) {
      onGo({ ...at, path: above });
    }
  }, [isGone, above, at, onGo]);
  const crumbs = crumbsOf(at, props.onGo, t);
  return (
    <ChooseBrowser
      crumbs={crumbs}
      shown={shownOf(listing, at, { onGo: props.onGo, reload, t })}
      moves={props}
      at={at}
    />
  );
}

/** The crumbs of a folder in a place: the places, the place, each folder down to it. */
function crumbsOf(at: ChooseAt, onGo: (at: ChooseAt | undefined) => void, t: TFunction): Crumb[] {
  const below = at.path
    .slice(at.start.path.length)
    .split("/")
    .filter((part) => part !== "");
  return [
    {
      key: "places",
      name: t("fileBrowser.places"),
      onOpen: () => {
        onGo(undefined);
      },
    },
    {
      key: at.start.path,
      name: `${at.start.nodeName}: ${at.start.path}`,
      onOpen: () => {
        onGo({ start: at.start, path: at.start.path });
      },
    },
    ...below.map((name, index) => {
      const path = below
        .slice(0, index + 1)
        .reduce((folder, part) => folderInside(folder, part), at.start.path);
      return {
        key: path,
        name,
        onOpen: () => {
          onGo({ start: at.start, path });
        },
      };
    }),
  ];
}

/** What a folder's listing in Choose needs: how to go to another, how to ask again, the words. */
interface ListingUse {
  readonly onGo: (at: ChooseAt | undefined) => void;
  readonly reload: () => void;
  readonly t: TFunction;
}

/** What a folder in Choose shows: its folders, each with its count where counted, or its state. */
function shownOf(
  listing: Resource<FolderListing>,
  at: ChooseAt,
  use: ListingUse,
): FileBrowserShown {
  const { onGo, reload, t } = use;
  const folder = folderName(at.path);
  if (listing.state === "loading") {
    return { kind: "loading" };
  }
  if (listing.state === "failed" || listing.value.state === "failed") {
    return {
      kind: "told",
      told: { kind: "problem", text: t("fileBrowser.cantRead", { folder }), onTryAgain: reload },
    };
  }
  const { value } = listing;
  if (value.state === "notAnswering") {
    return {
      kind: "told",
      told: {
        kind: "problem",
        text: t("fileBrowser.notAnswering", { machine: at.start.nodeName }),
        onTryAgain: reload,
      },
    };
  }
  if (value.state === "missing") {
    const up = upOf(at);
    return {
      kind: "told",
      told: {
        kind: "empty",
        text: t("fileBrowser.gone", { folder }),
        next: {
          label: t("fileBrowser.up"),
          onPress: () => {
            onGo(up);
          },
        },
      },
    };
  }
  return listedOf(value, at, use);
}

/** A readable folder in Choose: its folders, each with its count where counted, or that it has none. */
function listedOf(value: FolderListing, at: ChooseAt, use: ListingUse): FileBrowserShown {
  const { onGo, t } = use;
  const folder = folderName(at.path);
  const rows = inNaturalOrder(value.folders, (name) => name).map((name): FileRow => {
    const count = value.videoCounts[name];
    return {
      key: name,
      kind: "folder",
      name,
      details: count === undefined ? undefined : t("fileBrowser.videos", { count }),
      onOpen: () => {
        onGo({ start: at.start, path: folderInside(at.path, name) });
      },
    };
  });
  return {
    kind: "listed",
    groups: [{ key: at.path, rows }],
    empty: { kind: "empty", text: t("fileBrowser.noFolders", { folder }) },
  };
}

/** The file browser as Choose draws it, with Go to… at its bar's end, off a television. */
function ChooseBrowser(props: {
  readonly crumbs: readonly Crumb[];
  readonly shown: FileBrowserShown;
  readonly moves: ChooseMoves;
  readonly at: ChooseAt | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { moves } = props;
  const isGoToShown = !isOnTelevision();
  return (
    <FileBrowser
      use="choose"
      crumbs={props.crumbs}
      listLabel={props.crumbs.at(-1)?.name ?? ""}
      shown={props.shown}
      arrivalKey={props.at?.cameFrom}
      barEnd={
        isGoToShown ? (
          <button
            type="button"
            className="file-browser__crumb"
            onClick={() => {
              moves.onGoTo(props.at?.path ?? moves.places[0]?.path ?? "/");
            }}
          >
            {t("fileBrowser.goTo")}
          </button>
        ) : undefined
      }
      barInstead={
        moves.goTo === undefined ? undefined : (
          <GoToField moves={moves} nodeName={props.at?.start.nodeName} />
        )
      }
    />
  );
}

/**
 * Go to… (D539 §2): a field of the path to open, filled with where Choose is; Enter opens it where
 * it is inside a place of the machine Choose is on, or of any machine at the places, else says it
 * is not; Back and Escape leave it (Choose's onBack).
 */
function GoToField(props: {
  readonly moves: ChooseMoves;
  readonly nodeName: string | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { moves } = props;
  const [error, setError] = useState<string | undefined>(undefined);
  const formRef = useRef<HTMLFormElement>(null);
  // It opens filled with where Choose is, selected, so typing replaces it.
  useLayoutEffect(() => {
    const field = formRef.current?.querySelector("input");
    field?.focus();
    field?.select();
  }, []);
  const typed = moves.goTo ?? "";
  const open = (): void => {
    const path = typed.length > 1 ? typed.replace(/\/+$/u, "") : typed;
    const holding = moves.places
      .filter((place) => props.nodeName === undefined || place.nodeName === props.nodeName)
      .map((place) => placeHolding([place], { nodeName: place.nodeName, path }))
      .find((place) => place !== undefined);
    if (holding === undefined) {
      setError(t("fileBrowser.goToNowhere"));
      return;
    }
    moves.onGoTo(undefined);
    moves.onGo({ start: holding, path, isSeeking: true });
  };
  return (
    <form ref={formRef} className="file-browser__go-to" onSubmit={submitWith(open)}>
      <TextField
        label={t("fileBrowser.goToField")}
        value={typed}
        error={error}
        isVerbatim
        onChange={(value) => {
          setError(undefined);
          moves.onGoTo(value);
        }}
      />
    </form>
  );
}

/**
 * A button that opens Choose (D539 §2), and Choose while it is open: what Create a library, Add a
 * folder and A folder moved each offer.
 */
export function ChooseOpener(
  props: Omit<ChooseProps, "onClose"> & {
    /** What the button says: "Browse…". */
    readonly opener: string;
  },
): JSX.Element {
  const [isOpen, setIsOpen] = useState(false);
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
        <Choose
          {...props}
          onClose={() => {
            setIsOpen(false);
          }}
        />
      ) : null}
    </>
  );
}
