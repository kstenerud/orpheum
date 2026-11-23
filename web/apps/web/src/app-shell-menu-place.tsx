// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  attentionPageOrder,
  attentionPageRoute,
  itemCountOf,
  type AttentionPageKey,
} from "./attention-pages.ts";
import { WarningIcon } from "./icons.tsx";
import { ListOption } from "./list-box.tsx";
import { needsAttentionKey, readNeedsAttention } from "./needs-attention-read.ts";
import type { Route } from "./router.ts";
import { useServices } from "./services.tsx";
import { useShellPart } from "./shell-read.ts";
import { isUpdateKnown, useUpdate } from "./update.tsx";
import { useResource } from "./use-resource.ts";

/** One of the menu's places (app-shell.tsx), as its list draws it. */
interface MenuPlaceTold {
  readonly route: Route;
  /** Its name in the catalogue's words. */
  readonly name: string;
  /** Whether it is Update, whose name says when an update is known (UpdateName). */
  readonly isUpdate: boolean;
  /** Whether it is an admin's, under Manage, which Needs attention's pages can be. */
  readonly isAdmins: boolean;
  readonly isCurrent: boolean;
}

/**
 * The page of Needs attention's a place is, where things can need the admin on it (D538 §12): only
 * an admin's, under Manage.
 */
function attentionPageOf(route: Route, isAdmins: boolean): AttentionPageKey | undefined {
  if (!isAdmins) {
    return undefined;
  }
  return attentionPageOrder.find((page) => attentionPageRoute[page].screen === route.screen);
}

/**
 * One of the menu's places: a choice of its list (list-box.tsx), by its name, marked where things
 * can need the admin on its page.
 */
export function AppShellMenuPlace(props: MenuPlaceTold): JSX.Element {
  const page = attentionPageOf(props.route, props.isAdmins);
  return page === undefined ? (
    <ListOption to={props.route} isCurrent={props.isCurrent}>
      <PlaceName name={props.name} isUpdate={props.isUpdate} />
    </ListOption>
  ) : (
    <MarkedMenuPlace
      route={props.route}
      name={props.name}
      isUpdate={props.isUpdate}
      isCurrent={props.isCurrent}
      page={page}
    />
  );
}

/** A menu place's name: Update's its own (UpdateName), the rest the one it is told. */
function PlaceName(props: Pick<MenuPlaceTold, "name" | "isUpdate">): JSX.Element {
  return props.isUpdate ? <UpdateName /> : <>{props.name}</>;
}

/**
 * An admin's place things can need them on, marked while any does (D538 §12): the warning icon at
 * its end, and ", needs attention" after its name for a screen reader; no number. It reads what the
 * tray's sign does, which the shell's read says first (D476).
 */
function MarkedMenuPlace(
  props: Omit<MenuPlaceTold, "isAdmins"> & { readonly page: AttentionPageKey },
): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [counts] = useResource(needsAttentionKey, async () => await readNeedsAttention(api), {
    given: useShellPart((shell) => shell.needsAttention),
  });
  const isNeeded = counts.state === "loaded" && itemCountOf(counts.value.pages, props.page) > 0;
  return (
    <ListOption
      to={props.route}
      isCurrent={props.isCurrent}
      spoken={isNeeded ? t("attention.needsAttention", { name: props.name }) : undefined}
    >
      <PlaceName name={props.name} isUpdate={props.isUpdate} />
      {isNeeded ? (
        <span className="app-shell__attention-mark">
          <WarningIcon />
        </span>
      ) : null}
    </ListOption>
  );
}

/** Update's name in the menu: "! Update" while an update is known, else "Update" (D462 §8). */
function UpdateName(): JSX.Element {
  const { t } = useTranslation();
  const update = useUpdate(true);
  const isKnown = isUpdateKnown(update.state === "loaded" ? update.value : undefined);
  return <>{isKnown ? t("nav.updateKnown") : t("nav.update")}</>;
}
