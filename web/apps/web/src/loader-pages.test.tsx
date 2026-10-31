// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { PageNotControlled } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { handOffFragment } from "./discovery-start.ts";
import {
  forgetRefusedKey,
  leaveWhenSignedOut,
  markControlled,
  redeem,
  reloadOnceWhenUncontrolled,
  startAtClient,
  type AtClientOrigin,
} from "./loader-at-client-origin.tsx";
import { corePrefix, createSession } from "./session.ts";
import {
  codeParts,
  joinParts,
  startAtLoaderOrigin,
  type AtLoaderOrigin,
} from "./loader-at-loader-origin.tsx";
import { households, keyRefusedAddress, rememberHousehold } from "./loader-households.ts";
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

  it("goes no more to a household its own client origin says was signed out of, and offers the discovery's sign-in", async () => {
    const storage = memoryStorage();
    rememberHousehold(storage, household);
    const went: string[] = [];
    const shown = startAtLoaderOrigin(
      loaderAt(
        { search: "?signedout=k7xq2", referrer: "https://k7xq2.client.example/" },
        storage,
        went,
      ),
    );
    expect(went).toEqual([]);
    // Kept for the warning a realm sign-in gives when it finds another core for the realm (D312 §2).
    expect(households(storage).map((each) => [each.coreId, each.isSignedOut])).toEqual([
      ["k7xq2", true],
    ]);
    mounted = await mount(shown ?? <p>nothing</p>);
    expect(mounted.container.querySelector('input[name="name"]')).not.toBeNull();
    // Signed in again, it is gone to again.
    rememberHousehold(storage, household);
    const again: string[] = [];
    expect(startAtLoaderOrigin(loaderAt({}, storage, again))).toBeUndefined();
    expect(again).toEqual(["https://k7xq2.client.example/"]);
  });

  it("offers the sign-in, saying why, for a household whose core refused this device's key, though another is listed (the owner's Fire TV, 2026-10-28)", async () => {
    const storage = memoryStorage();
    rememberHousehold(storage, { ...household, coreId: "m3dz9", realm: "other" });
    rememberHousehold(storage, household);
    const went: string[] = [];
    const shown = startAtLoaderOrigin(
      loaderAt(
        {
          search: new URL(keyRefusedAddress(config.loaderOrigin, "k7xq2")).search,
          referrer: "https://k7xq2.client.example/",
        },
        storage,
        went,
      ),
    );
    expect(went).toEqual([]);
    expect(households(storage).map((each) => [each.coreId, each.isSignedOut])).toEqual([
      ["k7xq2", true],
      ["m3dz9", false],
    ]);
    mounted = await mount(shown ?? <p>nothing</p>);
    expect(mounted.container.querySelector('input[name="name"]')).not.toBeNull();
    expect(mounted.container.textContent).toContain("This device needs to sign in again");
    expect(mounted.container.querySelector(".what-happened pre")?.textContent).toContain(
      "the offer's proof does not hold",
    );
  });

  it("goes on to a household another page says refused this device's key", () => {
    const storage = memoryStorage();
    rememberHousehold(storage, household);
    const went: string[] = [];
    startAtLoaderOrigin(
      loaderAt(
        {
          search: new URL(keyRefusedAddress(config.loaderOrigin, "k7xq2")).search,
          referrer: "https://evil.example/",
        },
        storage,
        went,
      ),
    );
    expect(went).toEqual(["https://k7xq2.client.example/"]);
  });

  it("goes on to a household another page says was signed out of", () => {
    const storage = memoryStorage();
    rememberHousehold(storage, household);
    const went: string[] = [];
    startAtLoaderOrigin(
      loaderAt({ search: "?signedout=k7xq2", referrer: "https://evil.example/" }, storage, went),
    );
    expect(went).toEqual(["https://k7xq2.client.example/"]);
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
    showFailure: () => undefined,
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

  it("reloads once in a tab no worker controlled once it found its core, telling the core's log each time", async () => {
    const tab = memoryStorage();
    const told: string[] = [];
    let reloads = 0;
    const parts = {
      tab,
      tell: (text: string) => {
        told.push(text);
        return Promise.resolve();
      },
      reload: () => {
        reloads += 1;
      },
    };
    expect(await reloadOnceWhenUncontrolled(new PageNotControlled(), parts)).toBe(true);
    // Uncontrolled again after the reload: no loop, the failure shows.
    expect(await reloadOnceWhenUncontrolled(new PageNotControlled(), parts)).toBe(false);
    expect(reloads).toBe(1);
    expect(told).toEqual([
      `${new PageNotControlled().message}; reloading the page once`,
      `${new PageNotControlled().message}; reloaded for it before in this tab, so not again`,
    ]);
    // Carried once, a later time reloads again; any other failure never does.
    markControlled(tab);
    expect(await reloadOnceWhenUncontrolled(new Error("the channel closed"), parts)).toBe(false);
    expect(await reloadOnceWhenUncontrolled(new PageNotControlled(), parts)).toBe(true);
    expect(reloads).toBe(2);
  });
});

describe("signing out at a core's client origin (D308)", () => {
  /** A session at the client origin, signed in. */
  function signedIn(): ReturnType<typeof createSession> {
    const session = createSession(memoryStorage(), corePrefix("k7xq2"));
    session.signIn("token");
    return session;
  }

  it("goes to the discovery's page, saying so, once a realm's household signs out or is signed out", () => {
    for (const end of ["signOut", "signOutBecause"] as const) {
      const session = signedIn();
      const went: string[] = [];
      leaveWhenSignedOut(session, {
        core: { coreId: "k7xq2", fingerprint, realm: "nas" },
        hasRealms: true,
        loaderOrigin: config.loaderOrigin,
        go: (to) => {
          went.push(to);
        },
      });
      expect(went).toEqual([]);
      if (end === "signOut") {
        session.signOut();
      } else {
        session.signOutBecause("removed");
      }
      expect(went).toEqual(["https://app.example/?signedout=k7xq2"]);
    }
  });

  it("forgets a key its core refused, and goes to the discovery's sign-in for a realm's household", () => {
    for (const realm of ["nas", undefined]) {
      const session = signedIn();
      session.signIn("token", { locator: "a-revoked-locator" });
      let isMembershipForgotten = false;
      const went: string[] = [];
      forgetRefusedKey(
        {
          session,
          forgetMembership: () => {
            isMembershipForgotten = true;
          },
        },
        {
          core: { coreId: "k7xq2", fingerprint, realm },
          loaderOrigin: config.loaderOrigin,
          go: (to) => {
            went.push(to);
          },
        },
      );
      expect([session.token(), session.locator()]).toEqual([undefined, undefined]);
      expect(isMembershipForgotten).toBe(false);
      // A household reached by a link alone stays, its page saying a new link is needed.
      expect(went).toEqual(
        realm === undefined ? [] : ["https://app.example/?signedout=k7xq2&keyrefused="],
      );
    }
  });

  it("forgets the Watch Together membership a key was refused for when there is no session", () => {
    const session = createSession(memoryStorage(), corePrefix("k7xq2"));
    let isMembershipForgotten = false;
    forgetRefusedKey(
      {
        session,
        forgetMembership: () => {
          isMembershipForgotten = true;
        },
      },
      {
        core: { coreId: "k7xq2", fingerprint, realm: undefined },
        loaderOrigin: config.loaderOrigin,
        go: () => undefined,
      },
    );
    expect(isMembershipForgotten).toBe(true);
  });

  // The owner, 2026-10-30: signing out lands on "the discovery's sign-in, except where there is no
  // discovery". The S23, its household reached by a link, landed on the core's own; the Fire TV, its
  // reached through a realm, on the discovery's.
  it("goes to the discovery's sign-in for a household reached by a link alone, at a discovery with realms", () => {
    const session = signedIn();
    const went: string[] = [];
    leaveWhenSignedOut(session, {
      core: { coreId: "k7xq2", fingerprint, realm: undefined },
      hasRealms: true,
      loaderOrigin: config.loaderOrigin,
      go: (to) => {
        went.push(to);
      },
    });
    expect(went).toEqual([]);
    session.signOut();
    expect(went).toEqual(["https://app.example/?signedout=k7xq2"]);
  });

  it("stays at the core's own sign-in for a household reached by a link alone", () => {
    const session = signedIn();
    const went: string[] = [];
    // At a discovery with no realms, whose page has no sign-in of its own.
    leaveWhenSignedOut(session, {
      core: { coreId: "k7xq2", fingerprint, realm: undefined },
      hasRealms: false,
      loaderOrigin: config.loaderOrigin,
      go: (to) => {
        went.push(to);
      },
    });
    session.signOut();
    expect(went).toEqual([]);
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

  // Left open, its connections stayed for as long as the page did, each the core turned away
  // replaced at once (staging, 2026-10-25).
  it("closes its WebRTC transport when the hand-off fails too, going nowhere", async () => {
    const went: string[] = [];
    const parts = joinParts(loaderAt(scanned, memoryStorage(), went), activate);
    let isClosed = false;
    const transport = {
      fetch: () => Promise.reject(new TypeError("the channel for /api/v1/session/handoff failed")),
      close: () => {
        isClosed = true;
      },
    };
    const reaching = {
      first: { kind: "webrtc", transport },
      another: () => Promise.reject(new Error("none")),
      better: () => Promise.resolve(undefined),
    } as unknown as Parameters<typeof parts.onConnected>[1];
    const signIn = { name: "karl", password: "pw", held: { bootstrapToken: "a-token" } };
    const handedOn = parts.onConnected(
      { coreId: "k7xq2", fingerprint, realm: "nas" },
      reaching,
      signIn,
    );
    await expect(handedOn).rejects.toThrow();
    expect(isClosed).toBe(true);
    expect(went).toEqual([]);
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
