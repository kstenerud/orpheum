// SPDX-License-Identifier: MPL-2.0

/** A sign's mark: blue and pulsing while its function is on, the banner's white while it is off. */
export function signMark(isOn: boolean): "running" | undefined {
  return isOn ? "running" : undefined;
}
