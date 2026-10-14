// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  activateDevice,
  answerActivation,
  changePassword,
  createAccount,
  invite,
  register,
  registrationUrl,
  removeMyDevice,
  renameMyDevice,
  secretFromHash,
  signOutElsewhere,
} from "./accounts.ts";
import {
  keepPairing,
  pairingStateOf,
  readPairing,
  requestPairing,
  takePairedSession,
} from "./pairing.ts";
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
  it("puts a registration link's secret after #, and reads it back from there", () => {
    const url = registrationUrl("https://orpheum.example", { secret: "abc_DEF-123" });
    expect(url).toBe("https://orpheum.example/register#abc_DEF-123");
    // While a discovery has claimed the core, the core's link in the discovery's form is the one shared.
    const discoveryLink =
      "https://app.example/register#k7xq2.abc_DEF-123.Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw";
    expect(
      registrationUrl("https://orpheum.example", { secret: "abc_DEF-123", link: discoveryLink }),
    ).toBe(discoveryLink);
    expect(secretFromHash(new URL(url).hash)).toBe("abc_DEF-123");
    expect(secretFromHash("")).toBeUndefined();
    // A link a discovery serves names the core and its fingerprint around the secret (the protocol's §3.1).
    expect(secretFromHash("#k7xq2.abc_DEF-123.Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw")).toBe(
      "abc_DEF-123",
    );
    expect(parseRoute("/register", "").screen).toBe("register");
    expect(parseRoute("/people", "").screen).toBe("people");
    expect(parseRoute("/account", "").screen).toBe("account");
  });

  it("sends each call where the server takes it, with only what it needs", async () => {
    const sent: Heard[] = [];
    const session = [200, { token: "t", user: {} }] as const;
    const { api } = servedBy(
      {
        "POST /users": [200, { account: {} }],
        "POST /invitations": [200, { invitation: {}, link: { secret: "s", expiresAt: 1 } }],
        "POST /registration": session,
        "POST /pairings": [200, { secret: "s", code: "abcd-efgh", expiresAt: 1 }],
        "POST /pairings/state": [200, { state: "waiting" }],
        "POST /pairings/session": session,
        "POST /activations": [200, { checkCode: "wxyz-0123", deviceName: "TV" }],
        "POST /activations/answer": [204, undefined],
      },
      sent,
    );
    await createAccount(
      api,
      { name: "partner", password: "pw", role: "user", adminNote: "" },
      "click-1",
    );
    await invite(api, { role: "user", adminNote: "Grandma" }, "click-0");
    await register(
      api,
      { secret: "s", name: "Gran", password: "p", device: { name: "Phone", platform: "web" } },
      "click-2",
    );
    await requestPairing(api, {
      secret: undefined,
      previousDeviceId: 7,
      previousDeviceProof: "proof",
    });
    await requestPairing(api, {
      secret: "s",
      previousDeviceId: undefined,
      previousDeviceProof: undefined,
    });
    await pairingStateOf(api, "s");
    await activateDevice(api, { code: "abcd-efgh", deviceName: "Living room TV" }, "click-3");
    await activateDevice(
      api,
      { code: "abcd-efgh", deviceName: "Bedroom TV", userId: 7 },
      "click-4",
    );
    await answerActivation(api, { code: "abcd-efgh", isMatch: true }, "click-5");
    await takePairedSession(api, "s");
    expect(sent).toEqual([
      { methodAndPath: "POST /users", body: { name: "partner", password: "pw", role: "user" } },
      { methodAndPath: "POST /invitations", body: { role: "user", adminNote: "Grandma" } },
      {
        methodAndPath: "POST /registration",
        body: {
          secret: "s",
          name: "Gran",
          password: "p",
          device: { name: "Phone", platform: "web" },
        },
      },
      {
        methodAndPath: "POST /pairings",
        body: { platform: "web", previousDeviceId: 7, previousDeviceProof: "proof" },
      },
      { methodAndPath: "POST /pairings", body: { platform: "web", secret: "s" } },
      { methodAndPath: "POST /pairings/state", body: { secret: "s" } },
      {
        methodAndPath: "POST /activations",
        body: { code: "abcd-efgh", deviceName: "Living room TV" },
      },
      {
        methodAndPath: "POST /activations",
        body: { code: "abcd-efgh", deviceName: "Bedroom TV", userId: 7 },
      },
      { methodAndPath: "POST /activations/answer", body: { code: "abcd-efgh", isMatch: true } },
      { methodAndPath: "POST /pairings/session", body: { secret: "s" } },
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

  it("lists who has registered, and shows a link with when it stops working", () => {
    const list = renderToStaticMarkup(
      withServices(
        <PeopleList
          accounts={[
            { id: 1, name: "owner", role: "admin", hasPassword: true },
            { id: 2, name: "partner", role: "user", adminNote: "Anna", hasPassword: false },
          ]}
        />,
      ),
    );
    expect(list).toContain("Has a password");
    expect(list).toContain('href="/people/2"');
    expect(list).toContain("Has not used their link yet");
    expect(list).toContain("(Anna)");
    const link = renderToStaticMarkup(
      withServices(
        <Share text="https://x/register#s" invitation={{ secret: "s", expiresAt: 0 }} />,
      ),
    );
    expect(link).toContain('<output class="share__link">https://x/register#s</output>');
    expect(link).toContain("It works once, until");
  });

  it("shows a device's code again after a reload, until it expires (D288)", () => {
    const storage = memoryStorage();
    keepPairing(storage, { secret: "s", code: "abcd-efgh", expiresAt: 1000 });
    expect(readPairing(storage, 999)).toEqual({ secret: "s", code: "abcd-efgh", expiresAt: 1000 });
    expect(readPairing(storage, 1000)).toBeUndefined();
    expect(storage.length).toBe(0);
    storage.setItem("orpheum.pairing", "not json");
    expect(readPairing(storage, 0)).toBeUndefined();
  });

  it("lists devices with which is this one, and renames, removes and signs out elsewhere", async () => {
    const tv = television({ id: 5, isSignedIn: false });
    expect(deviceState(tv)).toBe("signedOut");
    expect(deviceState({ ...tv, isSignedIn: true })).toBe("signedIn");
    expect(deviceState({ ...tv, isThisDevice: true })).toBe("thisDevice");
    const nothing = (): void => undefined;
    const markup = renderToStaticMarkup(
      withServices(<DeviceList devices={[tv]} onRename={nothing} onRemove={nothing} />),
    );
    expect(markup).toContain("Living room TV");
    expect(markup).toContain("Signed out");
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
    await signOutElsewhere(api, "click-6");
    expect(sent).toEqual([
      { methodAndPath: "PUT /me/devices/5", body: { name: "Lounge TV" } },
      { methodAndPath: "DELETE /me/devices/5", body: undefined },
      { methodAndPath: "POST /me/devices/sign-out-elsewhere", body: undefined },
    ]);
  });

  it("shares a registration link at this app's own origin", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <IssuedInvitationView
          issued={{
            name: "Ada",
            invitation: { secret: "abc_DEF-123", expiresAt: 1 },
          }}
        />,
      ),
    );
    expect(markup).toContain(`${testOrigin}/register#abc_DEF-123`);
  });

  it("draws a registration link as a QR code, an image with its own alt text", () => {
    const url = "https://orpheum.example/register#secret";
    const { path, moduleCount } = qrPath(url);
    expect(moduleCount).toBeGreaterThanOrEqual(21);
    expect(path).toMatch(/^M\d/u);
    expect(qrDataUrl(url)).toMatch(/^data:image\/svg\+xml,/u);
    const markup = renderToStaticMarkup(<QrCode text={url} label="partner's link" />);
    expect(markup).toContain('alt="partner&#x27;s link"');
  });
});
