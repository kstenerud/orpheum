// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { SystemAddressAtHome, type AddressAtHome } from "./system-address-at-home.tsx";
import {
  bodyField,
  enter,
  mount,
  refusal,
  servedBy,
  submit,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

async function shown(
  addressAtHome: AddressAtHome,
  answers: Answers = {},
  { heard = [], onChanged }: { heard?: Heard[]; onChanged?: () => void } = {},
): Promise<HTMLElement> {
  mounted = await mount(
    withServices(
      <SystemAddressAtHome addressAtHome={addressAtHome} onChanged={onChanged} />,
      servedBy(answers, heard),
    ),
  );
  return mounted.container;
}

it("shows the address at home and how the server knew it (D452 §3)", async () => {
  const expectations: readonly [AddressAtHome, string][] = [
    [{ address: "", origin: "" }, "does not know its address in your home yet"],
    [{ address: "192.168.1.5", origin: "configured" }, "configuration names it"],
    [
      { address: "192.168.1.20", origin: "learned", learnedFrom: "Firefox on Linux", at: 1 },
      "Learned from Firefox on Linux",
    ],
    [{ address: "192.168.1.21", origin: "set", setByName: "karl", at: 1 }, "Set by karl"],
    [{ address: "192.168.1.21", origin: "set", at: 1 }, "Set by an admin"],
    [{ address: "192.168.1.22", origin: "guessed" }, "Guessed from the machine's own network"],
  ];
  for (const [addressAtHome, words] of expectations) {
    const container = await shown(addressAtHome);
    expect(container.textContent).toContain(words);
    if (addressAtHome.address !== "") {
      expect(container.textContent).toContain(`reach the server at ${addressAtHome.address}.`);
    }
    // A configured address wins, so the form that would only be refused is not offered.
    expect(container.querySelector("form") === null).toBe(addressAtHome.origin === "configured");
    mounted?.unmount();
    mounted = undefined;
  }
});

it("sets the address an admin types, and says in plain words why the server refused one (D452 §4)", async () => {
  const heard: Heard[] = [];
  let changes = 0;
  const container = await shown(
    { address: "", origin: "" },
    {
      "PUT /system/address-at-home": [
        refusal(409, "conflict"),
        refusal(400, "invalid_request"),
        [204, undefined],
      ],
    },
    {
      heard,
      onChanged: () => {
        changes += 1;
      },
    },
  );
  const form = container.querySelector("form");
  await enter(container.querySelector("input"), " 192.168.1.20 ");
  await submit(form);
  expect(container.querySelector('[role="alert"]')?.textContent).toContain(
    "configuration names its address (ORPHEUM_LAN_ADDRESS)",
  );
  await submit(form);
  expect(container.querySelector('[role="alert"]')?.textContent).toContain(
    "not an address in a home network",
  );
  await submit(form);
  expect(heard.map((each) => each.methodAndPath)).toEqual([
    "PUT /system/address-at-home",
    "PUT /system/address-at-home",
    "PUT /system/address-at-home",
  ]);
  expect(heard.map((each) => bodyField(each, "address"))).toEqual([
    "192.168.1.20",
    "192.168.1.20",
    "192.168.1.20",
  ]);
  expect(changes).toBe(1);
  expect(container.querySelector('[role="alert"]')).toBeNull();
});
