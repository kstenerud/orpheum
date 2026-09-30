// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

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
  readShownCode,
  signInWithCode,
  signOutElsewhere,
} from "./accounts.ts";
import { IssuedInvitationView, PeopleList } from "./people.tsx";
import { QrCode, qrDataUrl, qrPath } from "./qr-code.tsx";
import { parseRoute } from "./router.ts";
import { Share } from "./share.tsx";
import { servedBy, type Heard } from "./test-dom.tsx";
import { television } from "./test-fixtures.ts";
import { testOrigin, withServices } from "./test-services.tsx";
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

describe("accounts", () => {
  it("puts a registration invitation's secret after #, and reads it back from there", () => {
    const url = registrationUrl("https://orpheum.example", "abc_DEF-123");
    expect(url).toBe("https://orpheum.example/register#abc_DEF-123");
    expect(secretFromHash(new URL(url).hash)).toBe("abc_DEF-123");
    expect(secretFromHash("")).toBeUndefined();
    expect(parseRoute("/register", "").screen).toBe("register");
    expect(parseRoute("/people", "").screen).toBe("people");
    expect(parseRoute("/account", "").screen).toBe("account");
  });

  it("sends each call where the server takes it, with only what it needs", async () => {
    const sent: Heard[] = [];
    const secret = [200, { secret: "s", expiresAt: 1 }] as const;
    const session = [200, { token: "t", user: {} }] as const;
    const { api } = servedBy(
      {
        "POST /users": [
          200,
          { account: {}, registrationInvitation: { secret: "s", expiresAt: 1 } },
        ],
        "POST /registration": session,
        "POST /device-codes": secret,
        "POST /session/code": session,
      },
      sent,
    );
    await createAccount(api, "partner", "user");
    await register(api, { secret: "s", password: "p", device: { name: "Phone", platform: "web" } });
    await issueDeviceCode(api, { deviceName: "Living room TV" });
    await issueDeviceCode(api, { deviceName: "Bedroom TV" }, 7);
    await signInWithCode(api, "abcd-efgh", 7);
    expect(sent).toEqual([
      { methodAndPath: "POST /users", body: { name: "partner", role: "user" } },
      {
        methodAndPath: "POST /registration",
        body: { secret: "s", password: "p", device: { name: "Phone", platform: "web" } },
      },
      { methodAndPath: "POST /device-codes", body: { deviceName: "Living room TV" } },
      { methodAndPath: "POST /device-codes", body: { deviceName: "Bedroom TV", userId: 7 } },
      {
        methodAndPath: "POST /session/code",
        body: { code: "abcd-efgh", platform: "web", previousDeviceId: 7 },
      },
    ]);
    const changing: Heard[] = [];
    await changePassword(
      servedBy({ "PUT /me/password": [204, undefined] }, changing).api,
      "old",
      "new",
    );
    expect(changing).toEqual([
      { methodAndPath: "PUT /me/password", body: { currentPassword: "old", newPassword: "new" } },
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
        <Share kind="code" text="abcd-efgh" invitation={{ secret: "abcd-efgh", expiresAt: 0 }} />,
      ),
    );
    expect(code).toContain('<output class="share__code">abcd-efgh</output>');
    expect(code).toContain("It works once, until");
  });

  it("shows a device code again after a reload, until it expires", () => {
    const storage = memoryStorage();
    keepShownCode(storage, { secret: "abcd-efgh", expiresAt: 1000, deviceName: "TV" });
    expect(readShownCode(storage, 999)).toEqual({
      secret: "abcd-efgh",
      expiresAt: 1000,
      deviceName: "TV",
    });
    expect(readShownCode(storage, 1000)).toBeUndefined();
    expect(storage.length).toBe(0);
    storage.setItem("orpheum.deviceCode", "not json");
    expect(readShownCode(storage, 0)).toBeUndefined();
  });

  it("lists devices with which is this one, and renames, removes and signs out elsewhere", async () => {
    const tv = television({ id: 5, isSignedIn: false });
    expect(deviceState(tv)).toBe("signedOut");
    expect(deviceState({ ...tv, isSignedIn: true })).toBe("signedIn");
    expect(deviceState({ ...tv, isThisDevice: true })).toBe("thisDevice");
    const nothing = (): void => undefined;
    const markup = renderToStaticMarkup(
      withServices(
        <DeviceList devices={[tv]} onRename={nothing} onRemove={nothing} onMakeCode={nothing} />,
      ),
    );
    expect(markup).toContain("Living room TV");
    expect(markup).toContain("Signed out");
    expect(markup).toContain("Make a code to sign it back in");
    expect(markup).toContain('aria-label="Remove Living room TV; it is signed out"');
    const sent: Heard[] = [];
    const { api } = servedBy(
      {
        "PUT /me/devices/5": [204, undefined],
        "DELETE /me/devices/5": [204, undefined],
        "POST /me/devices/sign-out-elsewhere": [204, undefined],
      },
      sent,
    );
    await renameMyDevice(api, 5, "Lounge TV");
    await removeMyDevice(api, 5);
    await signOutElsewhere(api);
    expect(sent).toEqual([
      { methodAndPath: "PUT /me/devices/5", body: { name: "Lounge TV" } },
      { methodAndPath: "DELETE /me/devices/5", body: undefined },
      { methodAndPath: "POST /me/devices/sign-out-elsewhere", body: undefined },
    ]);
  });

  it("shares a registration invitation at this app's own origin", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <IssuedInvitationView
          issued={{
            kind: "link",
            name: "Ada",
            invitation: { secret: "abc_DEF-123", expiresAt: 1 },
          }}
        />,
      ),
    );
    expect(markup).toContain(`${testOrigin}/register#abc_DEF-123`);
  });

  it("draws a registration invitation as a QR code, an image with its own alt text", () => {
    const url = "https://orpheum.example/register#secret";
    const { path, moduleCount } = qrPath(url);
    expect(moduleCount).toBeGreaterThanOrEqual(21);
    expect(path).toMatch(/^M\d/u);
    expect(qrDataUrl(url)).toMatch(/^data:image\/svg\+xml,/u);
    const markup = renderToStaticMarkup(<QrCode text={url} label="partner's link" />);
    expect(markup).toContain('alt="partner&#x27;s link"');
  });
});
