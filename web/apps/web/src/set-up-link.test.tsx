// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { SetUp } from "./set-up.tsx";
import { setupCodeFromFragment } from "./set-up-link.ts";
import type { Services } from "./services.tsx";
import { enter, mount, servedBy, submit, type Answers, type Heard } from "./test-dom.tsx";
import { fakePlace, withServices } from "./test-services.tsx";

const linkedCode = "7kqm-2xhp-9rtw";

/** The services of a browser that opened address, with the server answering answers. */
function openedAt(address: string, answers: Answers, heard: Heard[] = []): Services {
  const place = fakePlace(address);
  return { ...servedBy(answers, heard), navigation: createNavigation(place) };
}

describe("the setup code in a magic link (D527 §1.2)", () => {
  it("reads the code from the fragment, and nothing from another fragment", () => {
    expect(setupCodeFromFragment(`code=${linkedCode}`)).toBe(linkedCode);
    expect(setupCodeFromFragment(`#code=${linkedCode}`)).toBe(linkedCode);
    expect(setupCodeFromFragment("")).toBeUndefined();
    expect(setupCodeFromFragment("code=")).toBeUndefined();
    // An invitation's link is a bare secret after the #, which is not a setup code.
    expect(setupCodeFromFragment("hkq3d9wx2c")).toBeUndefined();
  });

  it("fills the code in, takes the fragment out of the address, and sends the code with the admin", async () => {
    const heard: Heard[] = [];
    const services = openedAt(
      `/#code=${linkedCode}`,
      {
        "POST /setup": [201, { id: 1, name: "owner", role: "admin" }],
        "POST /session": [
          201,
          {
            token: "admin-token",
            deviceSecret: "s",
            user: { device: { id: 1, name: "Laptop" } },
          },
        ],
      },
      heard,
    );
    expect(services.navigation.fragment()).toBe(`code=${linkedCode}`);
    const mounted = await mount(withServices(<SetUp onBack={() => undefined} />, services));
    const code = mounted.container.querySelector<HTMLInputElement>('input[name="setupCode"]');
    expect(code?.value).toBe(linkedCode);
    expect(mounted.container.textContent).toContain("filled in from the link");
    // The code is gone from the address bar, and from the history entry that held it.
    expect(services.navigation.fragment()).toBe("");
    await enter(mounted.container.querySelector('input[name="name"]'), "owner");
    await enter(mounted.container.querySelector('input[type="password"]'), "a password");
    await submit(mounted.container.querySelector("form"));
    expect(heard.find((each) => each.methodAndPath === "POST /setup")?.body).toEqual({
      setupCode: linkedCode,
      name: "owner",
      password: "a password",
    });
    mounted.unmount();
  });

  it("leaves the code empty, and the address alone, when the link carries none", async () => {
    const services = openedAt("/#somethingelse", {});
    const mounted = await mount(withServices(<SetUp onBack={() => undefined} />, services));
    const code = mounted.container.querySelector<HTMLInputElement>('input[name="setupCode"]');
    expect(code?.value).toBe("");
    expect(mounted.container.textContent).not.toContain("filled in from the link");
    expect(services.navigation.fragment()).toBe("somethingelse");
    mounted.unmount();
  });
});
