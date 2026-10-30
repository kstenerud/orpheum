// SPDX-License-Identifier: MPL-2.0

/**
 * Where a page showing a cast that no one controls goes (D383 §2): a page loaded again, by a reload or
 * a deploy, whose cast the server no longer hands it, or a cast whose session ended unseen.
 */

import type { Api } from "@orpheum/sdk";

import type { Navigation } from "./navigation.ts";
import { routeHref, type Route } from "./router.ts";

/**
 * Whether the person may see the title itemId: not where the server refuses it (403) or knows it not
 * (404). Any other failure is the title page's to tell.
 */
async function maySee(api: Api, itemId: number): Promise<boolean> {
  try {
    const answer = await api.GET("/items/{id}", { params: { path: { id: itemId } } });
    return answer.response.status !== 403 && answer.response.status !== 404;
  } catch {
    return true;
  }
}

/**
 * Goes, in the current page's place, from a cast's page no one controls to the film's page, which its
 * Play page noted on its history entry (navigation.noteInPlace), or home where none was noted, the
 * page is the ready page between videos, or the person may not see the film's page (the owner: "go
 * back to the info page, or if it doesn't have access to it, to the home page"). Nothing where the
 * person moved on meanwhile.
 */
export async function leaveUncontrolledCast(api: Api, navigation: Navigation): Promise<void> {
  const from = routeHref(navigation.route());
  const noted = navigation.route().screen === "play" ? navigation.notedInPlace() : undefined;
  const home: Route = { screen: "home" };
  const to = noted?.screen === "title" && (await maySee(api, noted.itemId)) ? noted : home;
  if (routeHref(navigation.route()) === from) {
    navigation.replace(to);
  }
}
