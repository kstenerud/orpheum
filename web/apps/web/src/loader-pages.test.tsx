// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { handOffFragment } from "./discovery-start.ts";
import { redeem, startAtClient, type AtClientOrigin } from "./loader-at-client-origin.tsx";
import { createSession } from "./session.ts";
import {
  codeParts,
  joinParts,
  startAtLoaderOrigin,
  type AtLoaderOrigin,
} from "./loader-at-loader-origin.tsx";
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
  address: {
    readonly hash?: string;
    readonly search?: string;
    readonly referrer?: string;
    readonly pathname?: string;
  },
  storage: StringStorage,
  went: string[],
): AtLoaderOrigin {
  return {
    config,
    storage,
    location: {
      hash: address.hash ?? "",
      pathname: address.pathname ?? "/register",
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

  it("keeps a household its own client origin says is gone as gone, unlisted, and offers the realm sign-in", async () => {
    const storage = memoryStorage();
    rememberHousehold(storage, household);
    const went: string[] = [];
    const search = "?households=&gone=k7xq2";
    const shown = startAtLoaderOrigin(
      loaderAt({ search, referrer: "https://k7xq2.client.example/" }, storage, went),
    );
    expect(went).toEqual([]);
    // Kept for the warning a realm sign-in gives when it finds another core for the realm (D312 §2).
    expect(households(storage).map((each) => [each.coreId, each.isGone])).toEqual([
      ["k7xq2", true],
    ]);
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

  it("offers a television a code beside the realm sign-in, and keeps its household as a sign-in does (D293 §2)", async () => {
    mounted = await mount(startAtLoaderOrigin(loaderAt({}, memoryStorage(), [])) ?? <p>nothing</p>);
    expect(buttonWithText(mounted.container, "Get a code")).not.toBeNull();
    const storage = memoryStorage();
    const went: string[] = [];
    const parts = codeParts(loaderAt({}, storage, went));
    expect(parts.originOf("k7xq2")).toBe("https://k7xq2.client.example");
    parts.remember({ coreId: "k7xq2", fingerprint, realm: "nas" });
    parts.go("https://k7xq2.client.example/#handoff=x");
    expect(households(storage).map((each) => [each.coreId, each.realm, each.lastUsedAt])).toEqual([
      ["k7xq2", "nas", 5],
    ]);
    expect(went).toEqual(["https://k7xq2.client.example/#handoff=x"]);
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

describe("the loader's /activate, which a TV's QR code opens (D321)", () => {
  const scanned = { pathname: "/activate", search: "?code=abcd-efgh&key=rTBVo-NI60s-RZ3ENfKqkw" };
  const activate = "/account?code=abcd-efgh&key=rTBVo-NI60s-RZ3ENfKqkw";

  it("with one realm, goes straight to its Activate a device, the code filled in", () => {
    const storage = memoryStorage();
    rememberHousehold(storage, household);
    const went: string[] = [];
    expect(startAtLoaderOrigin(loaderAt(scanned, storage, went))).toBeUndefined();
    expect(went).toEqual([`https://k7xq2.client.example${activate}`]);
  });

  it("with several, asks which realm, each going to its Activate a device", async () => {
    const storage = memoryStorage();
    rememberHousehold(storage, household);
    rememberHousehold(storage, { ...household, coreId: "m3dz9", realm: "cottage" });
    mounted = await mount(startAtLoaderOrigin(loaderAt(scanned, storage, [])) ?? <p>nothing</p>);
    const page = mounted.container;
    expect(page.textContent).toContain("Activate the TV in which realm?");
    const links = [...page.querySelectorAll(".loader-households a")].map((link) => [
      link.textContent,
      link.getAttribute("href"),
    ]);
    expect(links).toEqual(
      expect.arrayContaining([
        ["nas", `https://k7xq2.client.example${activate}`],
        ["cottage", `https://m3dz9.client.example${activate}`],
      ]),
    );
  });

  it("with none, offers the link to copy, or a sign-in here", async () => {
    mounted = await mount(
      startAtLoaderOrigin(loaderAt(scanned, memoryStorage(), [])) ?? <p>nothing</p>,
    );
    const page = mounted.container;
    expect(page.textContent).toContain("This browser is not signed in to a household.");
    expect(page.querySelector(".loader-activate__link")?.textContent).toBe(
      `https://app.example/activate${scanned.search}`,
    );
    expect(buttonWithText(page, "Copy link")).not.toBeUndefined();
    await click(buttonWithText(page, "Sign in here"));
    expect(page.querySelector('input[name="name"]')).not.toBeNull();
  });

  it("hands a sign-in it carries on to the client origin's Activate a device, the hand-off after", async () => {
    const went: string[] = [];
    const parts = joinParts(loaderAt(scanned, memoryStorage(), went), activate);
    const transport = {
      fetch: () => Promise.resolve(Response.json({ code: "a-hand-off-code" })),
      close: () => undefined,
    };
    const reaching = {
      first: { kind: "webrtc", transport },
      another: () => Promise.reject(new Error("none")),
      better: () => Promise.resolve(undefined),
    } as unknown as Parameters<typeof parts.onConnected>[1];
    const signIn = { name: "karl", password: "pw", held: { bootstrapToken: "a-token" } };
    await parts.onConnected({ coreId: "k7xq2", fingerprint, realm: "nas" }, reaching, signIn);
    expect(went).toEqual([
      `https://k7xq2.client.example${activate}${handOffFragment({
        code: "a-hand-off-code",
        bootstrapToken: "a-token",
        fingerprint,
        realm: "nas",
      })}`,
    ]);
  });
});

describe("taking a hand-off's session at the client origin", () => {
  it("asks once, so a repeat on another path after its answer was lost gets the same session (2026-10-20)", async () => {
    const keys: (string | null)[] = [];
    let attempts = 0;
    // The first attempt reaches the core and its answer is lost, as Firefox's Local Network Access
    // drops a LAN name's; the keeper sends the same request again on another path.
    const send = (request: Request): Promise<Response> => {
      keys.push(request.headers.get("Idempotency-Key"));
      attempts += 1;
      if (attempts === 1) {
        return Promise.reject(new TypeError("NetworkError when attempting to fetch resource."));
      }
      return Promise.resolve(
        Response.json({
          token: "a-token",
          deviceSecret: "a-secret",
          user: {
            id: 1,
            name: "karl",
            role: "admin",
            isAdmin: true,
            device: { id: 7, name: "Firefox" },
          },
        }),
      );
    };
    const again = async (request: Request): Promise<Response> => {
      const copy = request.clone();
      try {
        return await send(request);
      } catch {
        return await send(copy);
      }
    };
    const session = createSession(memoryStorage());
    await redeem(again, session, "a-code");
    expect(keys).toHaveLength(2);
    expect(keys[0]).not.toBeNull();
    expect(keys[1]).toBe(keys[0]);
    expect(session.token()).toBe("a-token");
  });
});
