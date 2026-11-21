// SPDX-License-Identifier: MPL-2.0

import { createContext, type Context } from "react";

/**
 * Whether the server offers the device lab's tests: its shouldOfferDeviceTests, which an admin
 * turns on in Settings (the owner, 2026-10-25: for running device tests with beta participants);
 * undefined until the client settings are read.
 */
export const DeviceTestsOfferedContext: Context<boolean | undefined> = createContext<
  boolean | undefined
>(undefined);
