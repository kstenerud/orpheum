// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createApi } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  changePassword,
  createAccount,
  issueDeviceCode,
  register,
  registrationUrl,
  secretFromHash,
  signInWithCode,
} from "./accounts.ts";
import { PeopleList } from "./people.tsx";
import { parseRoute } from "./router.ts";
import { Share } from "./share.tsx";
import { withServices } from "./test-services.tsx";

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
    await issueDeviceCode(api, "Living room TV");
    await issueDeviceCode(api, "Bedroom TV", 7);
    await signInWithCode(api, "abcd-efgh");
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
        body: { code: "abcd-efgh", platform: "web" },
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
    const nothing = (): void => undefined;
    const list = renderToStaticMarkup(
      withServices(
        <PeopleList
          accounts={[
            { id: 1, name: "owner", role: "admin", isOutsider: false, hasPassword: true },
            { id: 2, name: "partner", role: "user", isOutsider: false, hasPassword: false },
          ]}
          act={{ linkFor: nothing, codeFor: nothing }}
        />,
      ),
    );
    expect(list).toContain("Has a password");
    expect(list).toContain("Has not used their link yet");
    const code = renderToStaticMarkup(
      withServices(
        <Share what="code" text="abcd-efgh" invitation={{ secret: "abcd-efgh", expiresAt: 0 }} />,
      ),
    );
    expect(code).toContain('<output class="share__code">abcd-efgh</output>');
    expect(code).toContain("It works once, until");
  });
});
