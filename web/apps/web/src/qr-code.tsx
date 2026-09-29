// SPDX-License-Identifier: MPL-2.0

import "./qr-code.css";

import qrcode from "qrcode-generator";
import type { JSX } from "react";

/** The quiet zone around a QR code, in modules, as the standard asks. */
const quietZoneModules = 4;

/** The dark modules of text's QR code as one SVG path, and the code's size in modules. */
export function qrPath(text: string): { path: string; moduleCount: number } {
  const code = qrcode(0, "M");
  code.addData(text);
  code.make();
  const moduleCount = code.getModuleCount();
  const squares: string[] = [];
  for (let row = 0; row < moduleCount; row++) {
    for (let column = 0; column < moduleCount; column++) {
      if (code.isDark(row, column)) {
        squares.push(
          `M${String(column + quietZoneModules)} ${String(row + quietZoneModules)}h1v1h-1z`,
        );
      }
    }
  }
  return { path: squares.join(""), moduleCount };
}

/** Text's QR code as an SVG image's address: dark modules on light, with the quiet zone. */
export function qrDataUrl(text: string): string {
  const { path, moduleCount } = qrPath(text);
  const size = String(moduleCount + 2 * quietZoneModules);
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${size} ${size}" shape-rendering="crispEdges">` +
    `<rect width="${size}" height="${size}" fill="#fff"/><path d="${path}" fill="#000"/></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

/** Text as a QR code a phone's camera opens (D218 §1). */
export function QrCode(props: { readonly text: string; readonly label: string }): JSX.Element {
  return <img className="qr-code" src={qrDataUrl(props.text)} alt={props.label} />;
}
