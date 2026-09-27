// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createApi } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  changePassword,
  createAccount,
  issueDeviceCode,
  keepShownCode,
  register,
  registrationUrl,
  removeMyDevice,
  renameMyDevice,
  secretFromHash,
  shownCode,
  signInWithCode,
  signOutElsewhere,
} from "./accounts.ts";
import { PeopleList } from "./people.tsx";
import { QrCode, qrImage, qrPath } from "./qr-code.tsx";
import { parseRoute } from "./router.ts";
import { Share } from "./share.tsx";
import { withServices } from "./test-services.tsx";
import { DeviceList, deviceState } from "./your-devices.tsx";

/** A Storage kept in memory, as a tab's sessionStorage is. */
function memoryStorage(): Storage {
  const items = new Map<string, string>();
  return {
    get length() {
      return items.size;
    },
    clear: () => {
      items.clear();
    },
    getItem: (key) => items.get(key) ?? null,
    key: (index) => [...items.keys()][index] ?? null,
    removeItem: (key) => {
      items.delete(key);
    },
    setItem: (key, value) => {
      items.set(key, value);
    },
  };
}

/** An API whose requests the test keeps, each answered with answer. */
function recording(answer: unknown): {
  api: ReturnType<typeof createApi>;
  sent: { method: string; path: string; body: unknown }[];
} {
  const sent: { method: string; path: string; body: unknown }[] = [];
  const api = createApi({ origin: "http://orpheum.test", token: () => "token" });
  // Answered here, before any request leaves: tests never touch the network.
  api.use({
    async onRequest({ request }) {
      const text = await request.text();
      sent.push({
        method: request.method,
        path: new URL(request.url).pathname,
        body: text === "" ? undefined : JSON.parse(text),
      });
      return answer === undefined
        ? new Response(null, { status: 204 })
        : new Response(JSON.stringify(answer), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
    },
  });
  return { api, sent };
}

describe("accounts", () => {
  it("puts a registration link's secret after #, and reads it back from there", () => {
    const url = registrationUrl("https://orpheum.example", "abc_DEF-123");
    expect(url).toBe("https://orpheum.example/register#abc_DEF-123");
    expect(secretFromHash(new URL(url).hash)).toBe("abc_DEF-123");
    expect(secretFromHash("")).toBeUndefined();
    expect(parseRoute("/register", "").screen).toBe("register");
    expect(parseRoute("/people", "").screen).toBe("people");
    expect(parseRoute("/account", "").screen).toBe("account");
  });

  it("sends each call where the server takes it, with only what it needs", async () => {
    const { api, sent } = recording({ token: "t", secret: "s", expiresAt: 1, accounts: [] });
    await createAccount(api, "partner", "user");
    await register(api, { secret: "s", password: "p", device: { name: "Phone", platform: "web" } });
    await issueDeviceCode(api, { deviceName: "Living room TV" });
    await issueDeviceCode(api, { deviceName: "Bedroom TV" }, 7);
    await signInWithCode(api, "abcd-efgh", 7);
    expect(sent).toEqual([
      { method: "POST", path: "/api/v1/users", body: { name: "partner", role: "user" } },
      {
        method: "POST",
        path: "/api/v1/registration",
        body: { secret: "s", password: "p", device: { name: "Phone", platform: "web" } },
      },
      { method: "POST", path: "/api/v1/device-codes", body: { deviceName: "Living room TV" } },
      {
        method: "POST",
        path: "/api/v1/device-codes",
        body: { deviceName: "Bedroom TV", userId: 7 },
      },
      {
        method: "POST",
        path: "/api/v1/session/code",
        body: { code: "abcd-efgh", platform: "web", resumeId: 7 },
      },
    ]);
    const changing = recording(undefined);
    await changePassword(changing.api, "old", "new");
    expect(changing.sent).toEqual([
      {
        method: "PUT",
        path: "/api/v1/me/password",
        body: { currentPassword: "old", newPassword: "new" },
      },
    ]);
  });

  it("lists who has registered, and shows a code large with when it stops working", () => {
    const list = renderToStaticMarkup(
      withServices(
        <PeopleList
          accounts={[
            { id: 1, name: "owner", role: "admin", isOutsider: false, hasPassword: true },
            { id: 2, name: "partner", role: "user", isOutsider: false, hasPassword: false },
          ]}
        />,
      ),
    );
    expect(list).toContain("Has a password");
    expect(list).toContain('href="/people/2"');
    expect(list).toContain("Has not used their link yet");
    const code = renderToStaticMarkup(
      withServices(
        <Share what="code" text="abcd-efgh" invitation={{ secret: "abcd-efgh", expiresAt: 0 }} />,
      ),
    );
    expect(code).toContain('<output class="share__code">abcd-efgh</output>');
    expect(code).toContain("It works once, until");
  });

  it("shows a device code again after a reload, until it expires", () => {
    const storage = memoryStorage();
    keepShownCode(storage, { secret: "abcd-efgh", expiresAt: 1000, deviceName: "TV" });
    expect(shownCode(storage, 999)).toEqual({
      secret: "abcd-efgh",
      expiresAt: 1000,
      deviceName: "TV",
    });
    expect(shownCode(storage, 1000)).toBeUndefined();
    expect(storage.length).toBe(0);
    storage.setItem("orpheum.deviceCode", "not json");
    expect(shownCode(storage, 0)).toBeUndefined();
  });

  it("lists devices with which is this one, and renames, removes and signs out elsewhere", async () => {
    const tv = {
      id: 5,
      name: "Living room TV",
      platform: "web",
      addedAt: 0,
      lastUsedAt: 0,
      isSignedIn: false,
      isThisDevice: false,
    };
    expect(deviceState(tv)).toBe("signedOut");
    expect(deviceState({ ...tv, isSignedIn: true })).toBe("signedIn");
    expect(deviceState({ ...tv, isThisDevice: true })).toBe("thisDevice");
    const nothing = (): void => undefined;
    const markup = renderToStaticMarkup(
      withServices(
        <DeviceList devices={[tv]} onRename={nothing} onRemove={nothing} onCodeFor={nothing} />,
      ),
    );
    expect(markup).toContain("Living room TV");
    expect(markup).toContain("Signed out");
    expect(markup).toContain("Make a code to sign it back in");
    expect(markup).toContain('aria-label="Remove Living room TV; it is signed out"');
    const { api, sent } = recording(undefined);
    await renameMyDevice(api, 5, "Lounge TV");
    await removeMyDevice(api, 5);
    await signOutElsewhere(api);
    expect(sent).toEqual([
      { method: "PUT", path: "/api/v1/me/devices/5", body: { name: "Lounge TV" } },
      { method: "DELETE", path: "/api/v1/me/devices/5", body: undefined },
      { method: "POST", path: "/api/v1/me/devices/sign-out-elsewhere", body: undefined },
    ]);
  });

  it("draws a registration link as a QR code, an image with its own alt text", () => {
    const url = "https://orpheum.example/register#secret";
    const { path, modules } = qrPath(url);
    expect(modules).toBeGreaterThanOrEqual(21);
    expect(path).toMatch(/^M\d/u);
    expect(qrImage(url)).toMatch(/^data:image\/svg\+xml,/u);
    const markup = renderToStaticMarkup(<QrCode text={url} label="partner's link" />);
    expect(markup).toContain('alt="partner&#x27;s link"');
  });
});
