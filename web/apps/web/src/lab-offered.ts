// SPDX-License-Identifier: MPL-2.0

import { createContext } from "react";

/**
 * Whether the server offers the device lab's tests: its should_offer_device_tests, which an admin
 * turns on in Settings (the owner, 2026-10-25: for running device tests with beta participants);
 * undefined until the client settings are read.
 */
export const DeviceTestsOfferedContext = createContext<boolean | undefined>(undefined);
