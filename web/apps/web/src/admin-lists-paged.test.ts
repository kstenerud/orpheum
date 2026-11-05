// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import { listAccounts, listInvitations } from "./accounts.ts";
import { listJobs } from "./jobs.tsx";
import { failOnUnexpectedRequests, servedBy, type Heard } from "./test-dom.tsx";

afterEach(failOnUnexpectedRequests);

// The admin's lists are shown whole: each follows its nextCursor to the last page, asking 500 at a
// time (D416 §6).
describe("the admin's whole lists", () => {
  it("read everyone, the invitations and the job view page by page to the last", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy(
      {
        "GET /users": [
          [200, { users: [{ id: 1 }], nextCursor: "u2" }],
          [200, { users: [{ id: 2 }] }],
        ],
        "GET /invitations": [
          [200, { invitations: [{ id: 3 }], nextCursor: "i2" }],
          [200, { invitations: [{ id: 4 }] }],
        ],
        "GET /jobs": [
          [200, { jobs: [{ id: 6 }], nextCursor: "j2" }],
          [200, { jobs: [{ id: 5 }] }],
        ],
      },
      heard,
    );
    expect((await listAccounts(api)).map((each) => each.id)).toEqual([1, 2]);
    expect((await listInvitations(api)).map((each) => each.id)).toEqual([3, 4]);
    expect((await listJobs(api)).map((each) => each.id)).toEqual([6, 5]);
    expect(heard.map((each) => `${each.methodAndPath}${each.query ?? ""}`)).toEqual([
      "GET /users?limit=500",
      "GET /users?limit=500&cursor=u2",
      "GET /invitations?limit=500",
      "GET /invitations?limit=500&cursor=i2",
      "GET /jobs?limit=500",
      "GET /jobs?limit=500&cursor=j2",
    ]);
  });
});
