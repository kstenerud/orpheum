// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { blankLibrary, libraryToCreate, libraryWords } from "./library-draft.ts";
import { listFolders } from "./file-browser.tsx";
import { idempotent } from "./idempotent.ts";
import { holdsTwoKinds, offerRows, type OfferKind, type OfferRow } from "./library-offer-kinds.ts";
import { useReadLibrariesAgain } from "./libraries-reached.ts";
import { Loaded } from "./loaded.tsx";
import { dataOrThrow, fieldProblemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { StatusNote } from "./status-note.tsx";
import { ValueChooser } from "./route-chooser.tsx";
import type { TFunction } from "i18next";

/** The kinds a folder may be offered as, each by its name, for its pop-up chooser. */
function offerKinds(
  t: TFunction,
): readonly { readonly value: OfferKind; readonly label: string }[] {
  return [
    { value: "film", label: t("libraries.kindMovies") },
    { value: "series", label: t("libraries.kindShows") },
    { value: "other", label: t("libraries.kindOther") },
  ];
}

/** The folders of one media folder, as offered. */
interface Offer {
  readonly nodeName: string;
  readonly root: string;
  readonly rows: readonly OfferRow[];
}

/**
 * The offer of a library for each folder inside each media folder of each connected node (D527 §3):
 * "Your media folder holds these folders. Make a library of each?" Each row has a tick, the folder's
 * name and a kind guessed from it, which can be changed; one press makes each ticked library, and
 * one the server refuses is named with why, the others made. Nothing shows where there are no
 * folders to offer. onMade is told once every ticked library was made.
 */
export function LibraryOffer(props: { readonly onMade: () => void }): JSX.Element {
  const { api } = useServices();
  const [offers] = useResource("library-offer", async (): Promise<Offer[]> => {
    const nodes = dataOrThrow(await api.GET("/nodes")).nodes ?? [];
    const found: Offer[] = [];
    for (const node of nodes.filter((each) => each.isConnected)) {
      for (const root of node.rootPaths ?? []) {
        const listing = await listFolders(api, { nodeName: node.name, path: root });
        if (listing.state === "readable" && (listing.folders ?? []).length > 0) {
          found.push({
            nodeName: node.name,
            root,
            rows: offerRows(node.name, root, listing.folders ?? []),
          });
        }
      }
    }
    return found;
  });
  return (
    <Loaded
      resource={offers}
      render={(found) => <OfferView offers={found} onMade={props.onMade} />}
    />
  );
}

/** What one press made and what the server refused: the paths made, and why each other was not. */
interface Made {
  readonly made: ReadonlySet<string>;
  readonly problems: ReadonlyMap<string, string>;
}

/** The press that makes each ticked row's library; one refused is named, the others made. */
function useMakeLibraries(
  rows: readonly OfferRow[],
  onMade: () => void,
): Made & {
  readonly ticked: readonly OfferRow[];
  readonly make: () => void;
  readonly isBusy: boolean;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const readLibrariesAgain = useReadLibrariesAgain();
  const [result, setResult] = useState<Made>(() => ({ made: new Set(), problems: new Map() }));
  const request = useRequest((cause) => fieldProblemText(t, cause, libraryWords(t)));
  const ticked = rows.filter((row) => row.isTicked && !result.made.has(row.path));
  const make = (): void => {
    request.runRequest(async (idempotencyKey) => {
      const nowMade = new Set(result.made);
      const nowProblems = new Map<string, string>();
      for (const row of ticked) {
        try {
          dataOrThrow(
            await api.POST("/libraries", {
              body: libraryToCreate({
                ...blankLibrary(),
                name: row.name,
                kind: row.kind,
                nodeName: row.nodeName,
                rootPaths: [row.path],
              }),
              ...idempotent(`${idempotencyKey}:${row.path}`),
            }),
          );
          nowMade.add(row.path);
        } catch (cause: unknown) {
          nowProblems.set(row.path, fieldProblemText(t, cause, libraryWords(t)));
        }
      }
      setResult({ made: nowMade, problems: nowProblems });
      if (nowMade.size > result.made.size) {
        readLibrariesAgain();
      }
      if (nowProblems.size === 0) {
        onMade();
      }
    });
  };
  return { ...result, ticked, make, isBusy: request.isBusy };
}

/** The offered rows, their ticks and kinds, the press that makes the ticked ones, and what it said. */
function OfferView(props: {
  readonly offers: readonly Offer[];
  readonly onMade: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const [rows, setRows] = useState<readonly OfferRow[]>(() =>
    props.offers.flatMap((offer) => offer.rows),
  );
  const change = (path: string, changed: Partial<OfferRow>): void => {
    setRows((held) => held.map((row) => (row.path === path ? { ...row, ...changed } : row)));
  };
  const { made, problems, ticked, make, isBusy } = useMakeLibraries(rows, props.onMade);
  if (props.offers.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="library-offer-heading">
      <h2 id="library-offer-heading">{t("libraries.offerHeading")}</h2>
      {props.offers.map((offer) => (
        <OfferGroup
          key={`${offer.nodeName}:${offer.root}`}
          offer={offer}
          rows={rows.filter(
            (row) => row.path.startsWith(offer.root) && row.nodeName === offer.nodeName,
          )}
          made={made}
          problems={problems}
          onChange={change}
        />
      ))}
      <button type="button" disabled={ticked.length === 0 || isBusy} onClick={make}>
        {t("libraries.offerMake")}
      </button>
    </section>
  );
}

/** One media folder's rows, and the warning when taking it whole would mix two kinds. */
function OfferGroup(props: {
  readonly offer: Offer;
  readonly rows: readonly OfferRow[];
  readonly made: ReadonlySet<string>;
  readonly problems: ReadonlyMap<string, string>;
  readonly onChange: (path: string, changed: Partial<OfferRow>) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { offer } = props;
  return (
    <div>
      <p>
        <code>{t("libraries.root", { node: offer.nodeName, path: offer.root })}</code>
      </p>
      <ul>
        {props.rows.map((row) => (
          <OfferRowView
            key={row.path}
            row={row}
            isMade={props.made.has(row.path)}
            problem={props.problems.get(row.path)}
            onChange={props.onChange}
          />
        ))}
      </ul>
      {holdsTwoKinds(offer.rows) ? (
        <p role="note">{t("libraries.offerTwoKinds", { path: offer.root })}</p>
      ) : null}
    </div>
  );
}

/** One offered folder: its tick, its name, its kind to change, and whether it was made or refused. */
function OfferRowView(props: {
  readonly row: OfferRow;
  readonly isMade: boolean;
  readonly problem: string | undefined;
  readonly onChange: (path: string, changed: Partial<OfferRow>) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { row } = props;
  return (
    <li>
      <label>
        <input
          type="checkbox"
          checked={row.isTicked || props.isMade}
          disabled={props.isMade}
          onChange={(event) => {
            props.onChange(row.path, { isTicked: event.target.checked });
          }}
        />
        {row.name}
      </label>{" "}
      {props.isMade ? (
        <span>{offerKinds(t).find((kind) => kind.value === row.kind)?.label}</span>
      ) : (
        <ValueChooser
          fieldLabel={t("libraries.offerKindOf", { name: row.name })}
          isFieldLabelShown={false}
          choices={offerKinds(t)}
          current={row.kind}
          onChoose={(kind) => {
            props.onChange(row.path, { kind });
          }}
        />
      )}
      {props.isMade ? <output> {t("libraries.offerMade")}</output> : null}
      {props.problem === undefined ? null : (
        <StatusNote
          told={{
            kind: "problem",
            text: t("libraries.offerRefused", { name: row.name, problem: props.problem }),
          }}
        />
      )}
    </li>
  );
}
