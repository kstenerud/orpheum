// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { browserLocalNetwork, type PermissionsByName } from "./local-network.ts";

/** A permission's status as a browser keeps it, changed by hand. */
class FakeStatus extends EventTarget {
  constructor(public state: PermissionState) {
    super();
  }

  change(to: PermissionState): void {
    this.state = to;
    this.dispatchEvent(new Event("change"));
  }
}

/** A Permissions API that knows only the names given, each with its status. */
function permissionsKnowing(known: Record<string, FakeStatus>): PermissionsByName {
  return {
    query: (descriptor) => {
      const status = known[descriptor.name];
      return status === undefined
        ? Promise.reject(new TypeError(`${descriptor.name} is not a permission here`))
        : Promise.resolve(status as unknown as PermissionStatus);
    },
  };
}

describe("the browser's local-network permission (W2c item 1)", () => {
  it("reads the current name, else the older one, else says the browser has none", async () => {
    expect(
      await browserLocalNetwork(
        permissionsKnowing({ "local-network": new FakeStatus("prompt") }),
      ).state(),
    ).toBe("prompt");
    expect(
      await browserLocalNetwork(
        permissionsKnowing({ "local-network-access": new FakeStatus("denied") }),
      ).state(),
    ).toBe("denied");
    expect(await browserLocalNetwork(permissionsKnowing({})).state()).toBe("unknown");
    expect(await browserLocalNetwork(undefined).state()).toBe("unknown");
  });

  it("tells when it becomes granted, and nothing once the listener is removed", async () => {
    const status = new FakeStatus("prompt");
    const localNetwork = browserLocalNetwork(permissionsKnowing({ "local-network": status }));
    let told = 0;
    const remove = localNetwork.onGranted(() => {
      told += 1;
    });
    await localNetwork.state();
    status.change("denied");
    status.change("granted");
    expect(told).toBe(1);
    remove();
    status.change("granted");
    expect(told).toBe(1);
  });
});
