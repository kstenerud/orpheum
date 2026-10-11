// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { components } from "@orpheum/sdk";
import { afterEach, expect, it } from "vitest";

import { PosterGrid, type Entry } from "./browse.tsx";
import { ConfirmButton } from "./confirm-button.tsx";
import { RootWarning } from "./root-warning.tsx";
import { click, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { IsAdminContext, WarningSign } from "./warning-sign.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The tree, rendered for an administrator or for anyone else. */
async function mountAs(isAdmin: boolean, tree: React.ReactNode): Promise<Mounted> {
  return await mount(
    withServices(<IsAdminContext value={isAdmin}>{tree}</IsAdminContext>, servedBy({})),
  );
}

/** The warning sign's button in the container. */
function sign(container: HTMLElement): HTMLButtonElement | null {
  return container.querySelector<HTMLButtonElement>(".warning-sign__button");
}

const sample = (
  <WarningSign
    summary="Season 4 could not be read"
    forAdmin="chown -R 1000:100 it"
    forViewer="Part of it is missing."
  />
);

it("opens what is wrong and how to fix it for an administrator, beside what it affects (D278 §1, §2)", async () => {
  mounted = await mountAs(true, sample);
  expect(sign(mounted.container)?.getAttribute("aria-label")).toBe(
    "What is wrong: Season 4 could not be read",
  );
  expect(mounted.container.textContent).not.toContain("chown");
  await click(sign(mounted.container));
  expect(sign(mounted.container)?.getAttribute("aria-expanded")).toBe("true");
  expect(mounted.container.textContent).toContain("Season 4 could not be read");
  expect(mounted.container.textContent).toContain("chown -R 1000:100 it");
  expect(mounted.container.textContent).not.toContain("Tell whoever runs this server");
});

it("tells anyone else only that something is wrong, and to tell the administrator (D278 §3)", async () => {
  mounted = await mountAs(false, sample);
  await click(sign(mounted.container));
  const text = mounted.container.textContent;
  expect(text).toContain("Part of it is missing.");
  expect(text).toContain("Tell whoever runs this server, so they can fix it.");
  expect(text).not.toContain("chown");
});

it("marks a title that cannot be played now with the sign over its poster", async () => {
  const entry = {
    id: 10,
    name: "Metropolis",
    identificationState: "identified",
    libraryId: 1,
    addedAt: 1,
    unavailable: { reason: "node_away", nodeName: "nas" },
  } as unknown as Entry;
  mounted = await mountAs(true, <PosterGrid kind="movie" entries={[entry]} />);
  await click(sign(mounted.container));
  expect(mounted.container.textContent).toContain("This cannot be played now");
  expect(mounted.container.textContent).toContain("The Libraries page says more");
});

it("marks a folder whose scan waits with the sign, saying why", async () => {
  const root = {
    nodeName: "sandbox",
    rootPath: "/media/shows",
    lastScanOutcome: "withheld",
    withheldReason: "the scan would remove 7 of the root's 95 files",
  } as unknown as components["schemas"]["RootBody"];
  mounted = await mountAs(true, <RootWarning root={root} />);
  await click(sign(mounted.container));
  expect(mounted.container.textContent).toContain("A scan waits for you");
  expect(mounted.container.textContent).toContain("would remove 7");
  mounted.unmount();
  const well = {
    nodeName: "sandbox",
    rootPath: "/media/shows",
    lastScanOutcome: "complete",
  } as unknown as components["schemas"]["RootBody"];
  mounted = await mountAs(true, <RootWarning root={well} />);
  expect(sign(mounted.container)).toBeNull();
});

it("colours what cannot be undone red: outlined to ask, filled to confirm, its words too (D278 §4)", async () => {
  mounted = await mountAs(
    true,
    <ConfirmButton
      label="Remove Dan…"
      consequence="This cannot be undone."
      confirmLabel="Remove them"
      onConfirm={() => undefined}
    />,
  );
  const ask = mounted.container.querySelector("button");
  expect(ask?.className).toBe("irreversible-outline");
  await click(ask);
  expect(mounted.container.querySelector(".irreversible-words")?.textContent).toBe(
    "This cannot be undone.",
  );
  expect(
    [...mounted.container.querySelectorAll("button")].find((b) => b.textContent === "Remove them")
      ?.className,
  ).toBe("irreversible");
});
