// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { handOffFragment } from "./discovery-start.ts";
import { startAtClient, type AtClientOrigin } from "./loader-at-client-origin.tsx";
import { joinParts, startAtLoaderOrigin, type AtLoaderOrigin } from "./loader-at-loader-origin.tsx";
import { households, rememberHousehold } from "./loader-households.ts";
import { loaderOf, loaderRefusal } from "./loader-interface.ts";
import { buttonWithText, click, mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

function memoryStorage(): StringStorage {
  const items = new Map<string, string>();
  return {
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
    removeItem: (key) => {
      items.delete(key);
    },
  };
}

const fingerprint = "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
const config = {
  clientApi: "https://api.example/client/v1",
  hasRealms: true,
  clientOrigins: "https://{core}.client.example",
  loaderOrigin: "https://app.example",
};

/** The loader's origin at an address, where it went heard. */
function loaderAt(
  address: { readonly hash?: string; readonly search?: string; readonly referrer?: string },
  storage: StringStorage,
  went: string[],
): AtLoaderOrigin {
  return {
    config,
    storage,
    location: {
      hash: address.hash ?? "",
      pathname: "/register",
      search: address.search ?? "",
      replace: (to: string) => {
        went.push(to);
      },
      assign: (to: string) => {
        went.push(to);
      },
    },
    referrer: address.referrer ?? "",
    now: () => 5,
  };
}

const household = { coreId: "k7xq2", fingerprint, realm: "nas", name: "", lastUsedAt: 1 };

describe("the loader at its own origin (D308)", () => {
  it("sends a link on to its core's client origin, the link as it came", () => {
    const went: string[] = [];
    const hash = `#k7xq2.a-secret.${fingerprint}`;
    expect(startAtLoaderOrigin(loaderAt({ hash }, memoryStorage(), went))).toBeUndefined();
    expect(went).toEqual([`https://k7xq2.client.example/register${hash}`]);
  });

  it("goes to the one household it knows, and lists them when it knows more or is asked", async () => {
    const storage = memoryStorage();
    rememberHousehold(storage, household);
    const went: string[] = [];
    expect(startAtLoaderOrigin(loaderAt({}, storage, went))).toBeUndefined();
    expect(went).toEqual(["https://k7xq2.client.example/"]);
    const asked = startAtLoaderOrigin(loaderAt({ search: "?households=" }, storage, []));
    mounted = await mount(asked ?? <p>nothing</p>);
    const links = [...mounted.container.querySelectorAll("a")].map((link) => link.href);
    expect(links).toEqual(["https://k7xq2.client.example/"]);
    expect(mounted.container.textContent).toContain("Sign in to another household");
  });

  it("forgets a household its own client origin says is gone, and offers the realm sign-in", async () => {
    const storage = memoryStorage();
    rememberHousehold(storage, household);
    const went: string[] = [];
    const search = "?households=&gone=k7xq2";
    const shown = startAtLoaderOrigin(
      loaderAt({ search, referrer: "https://k7xq2.client.example/" }, storage, went),
    );
    expect(went).toEqual([]);
    expect(households(storage)).toEqual([]);
    mounted = await mount(shown ?? <p>nothing</p>);
    expect(mounted.container.querySelector('input[name="name"]')).not.toBeNull();
  });

  it("keeps a household another page says is gone", () => {
    const storage = memoryStorage();
    rememberHousehold(storage, household);
    startAtLoaderOrigin(
      loaderAt(
        { search: "?households=&gone=k7xq2", referrer: "https://evil.example/" },
        storage,
        [],
      ),
    );
    expect(households(storage).map((each) => each.coreId)).toEqual(["k7xq2"]);
  });

  it("forgets a household only once asked again, with a small red button (the owner, 2026-10-17)", async () => {
    const storage = memoryStorage();
    rememberHousehold(storage, household);
    mounted = await mount(
      startAtLoaderOrigin(loaderAt({ search: "?households=" }, storage, [])) ?? <p>nothing</p>,
    );
    const page = mounted.container;
    const forget = page.querySelector<HTMLButtonElement>('button[aria-label="Forget nas"]');
    expect(forget?.className).toBe("irreversible-outline");
    await click(forget);
    expect(page.textContent).toContain("This browser will forget nas.");
    await click(buttonWithText(page, "Keep it"));
    expect(households(storage)).toHaveLength(1);
    await click(page.querySelector('button[aria-label="Forget nas"]'));
    await click(buttonWithText(page, "Forget it"));
    expect(households(storage)).toEqual([]);
    expect(page.querySelectorAll(".loader-households li")).toHaveLength(0);
  });

  it("with none, opens the realm sign-in", async () => {
    mounted = await mount(startAtLoaderOrigin(loaderAt({}, memoryStorage(), [])) ?? <p>nothing</p>);
    expect(mounted.container.querySelector('input[name="name"]')).not.toBeNull();
  });

  it("reaches the core without its LAN names, which the client origin asks the browser for alone", () => {
    expect(joinParts(loaderAt({}, memoryStorage(), [])).shouldSkipLanNames).toBe(true);
  });
});

/** A client origin of core k7xq2 at hash, its fragment's clearing heard. */
function clientAt(hash: string, storage: StringStorage, cleared: string[]): AtClientOrigin {
  return {
    config,
    coreId: "k7xq2",
    storage,
    location: { hash, pathname: "/", search: "", assign: () => undefined },
    history: {
      replaceState: (_data: unknown, _unused: string, url?: string | URL | null) => {
        cleared.push(String(url));
      },
    },
    loaderOrigin: config.loaderOrigin,
    unmount: () => undefined,
    tellWorker: () => Promise.resolve(),
  };
}

describe("the loader at a core's client origin (D308)", () => {
  it("starts from a hand-off, clearing it from the address, or a link to this origin's core alone", () => {
    const cleared: string[] = [];
    const handOff = { code: "a-code", bootstrapToken: "a-token", fingerprint, realm: "nas" };
    expect(startAtClient(clientAt(handOffFragment(handOff), memoryStorage(), cleared))).toEqual({
      kind: "handoff",
      core: { coreId: "k7xq2", fingerprint, realm: "nas" },
      handOff,
    });
    expect(cleared).toEqual(["/"]);
    expect(
      startAtClient(clientAt(`#k7xq2.a-secret.${fingerprint}`, memoryStorage(), [])).kind,
    ).toBe("link");
    // Another core's link opens nothing here: its client would run on this core's origin.
    expect(startAtClient(clientAt(`#m3dz9.a-secret.${fingerprint}`, memoryStorage(), []))).toEqual({
      kind: "none",
    });
  });
});

describe("the loader the client finds on the window (D308)", () => {
  it("is one of its shape alone, and refused when its major is not the client's", () => {
    expect(loaderOf({})).toBeUndefined();
    expect(loaderOf({ orpheumLoader: { major: 1 } })).toBeUndefined();
    expect(loaderRefusal({ major: 1 })).toBeUndefined();
    expect(loaderRefusal({ major: 2 })).toEqual({ needs: 1, has: 2 });
  });
});
