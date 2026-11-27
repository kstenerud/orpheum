// SPDX-License-Identifier: MPL-2.0

/**
 * What every file of the fast project starts from. Its files share a worker's modules
 * (vitest.config.ts), and the SDK's failure reporter is one of them: it holds what is reported
 * before there is anywhere to send it, counts what it sent against a page's most, and keeps the
 * last sink named. A file's test naming its own sink was handed what the files before it had
 * left, in an order that comes from Vitest's kept results, so three tests failed on a fresh
 * checkout alone (polling.test.ts, failure-reports.test.ts, data-cap.test.ts; CI, 2026-11-27).
 * Each file now starts with nothing held, nothing counted and a sink that drops. Not exported by
 * the package (index.ts).
 */

import { sendFailuresTo } from "./problem.ts";
import { forgetSteps } from "./steps.ts";

// Vitest runs a setup file again before each file, in a shared worker too.
sendFailuresTo(() => undefined);
// The steps a page took are kept the same way: steps.test.ts counted those a file before it had
// noted, one fresh run in four with two workers.
forgetSteps();
