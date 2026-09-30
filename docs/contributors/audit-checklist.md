> **ABOUTME:** What every commit and every audit between milestones checks. It grows: each time
> an audit, a correction or a defect shows something no gate catches, the check for it is added.

# The audit checklist

The owner asked for an audit between milestones (*"between each milestone, we should do another
audit of the architecture, procedures, rules etc"*, 2026-09-24), and then for this list
(*"We may need to start maintaining a list of things to check on an audit"*, 2026-09-26). An
audit works through every item, writes its findings in
`design/research/audit-<date>.md` in the earlier audits' form, and puts each proposal to the
owner. Some checks here exist because no mechanical gate can do them (principles §17): the
audit is that rule's only enforcement.

**Adding an item:** when an owner's correction, an [agent failure](agent-failures.md) or a
[plan deviation](plan-deviations.md) shows a kind of defect that a gate cannot catch, add the
check here, citing where it came from. When a gate later takes a check over, say so beside it
rather than deleting it.

## At every commit

The 2026-09-26 naming audit found the naming rule broken across the whole codebase: 47 names a
reader had to look up and 99 that could be clearer, each written one commit at a time with a
rule in place that said not to. The owner then made these checks policy at every commit as well
as at every milestone (AGENTS.md rule 17), so that a lapse is caught in the commit that makes it,
not after it has spread: *"Since you're non-deterministic with prose rules, we need mechanical
checks to catch things you miss."* Before committing, check the diff against items 7, 8, 10 and
11 below, and against item 19 when the commit claims to finish a decision. A finding is fixed in the same commit; one that reaches a migration, the API or a
setting key goes to the owner first.

## The milestone

1. **Acceptance:** each item of the milestone's row in cut-a §8, with the test that proves it,
   or why it is a verification-queue item (VQ).
2. **What grew beyond the plan,** and which decisions it rests on.
3. **What is unfinished,** including anything an earlier audit left open.
4. **Components the milestone named that no acceptance test exercises** (plan-deviations
   P25: the node's byte socket was named in M2 and never built).

## The documents

5. **Contradictions between documents:** the plan, the decisions, schema.md, the package
   docs. Walk one request of the next milestone through every section it touches, and list
   each value a step needs that no step produces (plan-deviations P26).
6. **Documents that no longer say what is true:** schema.md against the migrations and
   cache.db's schema, the plan's endpoint table against the API, standards against practice.

## The code

7. **Naming:** every name added or changed since the last audit, read as someone at the use site
   who has not seen the declaration. Does it say what the value is, its quantity and its unit,
   not just its category? Includes settings keys, config keys, columns, JSON fields, query names
   and catalogue keys. Run it as parallel read-only reviews by area, and check each finding
   before renaming. (agent-failures A8; no gate can check meaning. A rename that reaches a
   migration, the API or a setting key goes to the owner.)
8. **Names that drifted from behaviour:** a name or a doc comment that promises something the
   code no longer does (the 2026-09-26 naming audit found `job_retry_limit` counting attempts,
   and `nodeproto.Path`, documented as relative, carrying an absolute path).
9. **Derived data:** for every derived table or cached value (sort keys, search alternates,
   FTS, resolved columns, caches), what are its inputs, and does every input's change refresh
   it? (agent-failures A4)
10. **One way to do each job** (AGENTS.md rule 11): helpers written more than once, such as
    pointer defaults and path joining. Only the same concept counts: before reporting, read D229
    §2's settled look-alikes and [the watch list](look-alike-watch-list.md); a doubtful pair goes
    on the watch list, not into a merge.
11. **Failure classes of calls to other hosts:** each new call names its policy for a 404, a
    429, a 5xx, a timeout and a host that keeps failing (standards/go.md §21; agent-failures A5).

## The process

12. **Gates and rules that fought correct work:** hook false positives, lint conflicts,
    revert-red flags on code no test can reach. Is each recurring, and what would fix it?
    And gates that passed wrong work: has each gate been **seen failing on every kind of file it
    claims** — Go, TypeScript, types alone, another architecture, generated code? (P29: revert-red
    passed every web file for want of vitest in its worktree.)
13. **Agent failures since the last audit:** does each still have its gate or its practice, and
    has any recurred?
14. **Plan deviations:** add one entry per deviation found, with its kind and a process lesson
    (D203).
15. **The verification queue:** what is unticked, and whether any item could become a test.

## The next milestone

16. **What it needs decided before its first line**, framed as concepts before options, after
    searching the decisions for anything already settled. When research informs one, it first lists the field's
    standard concerns and marks each covered, a gap or not relevant (agent-failures A14).
17. **Platforms** (D205): the module still builds for linux/arm64; recheck riscv64's blockers.
18. **New dependencies** since the last audit, and whether any is architecture-specific.

## Decisions

19. **Decisions claimed built:** for each decision a commit or a report calls built, each of its
    numbered clauses read back against the code and its tests, and any clause not built named
    (agent-failures A9: D218 was reported built while §4's listing and revoking of devices was
    not). A decision's **Status** line says "built" only after this read-back (plan-deviations
    P31: D217's said built while §5's eviction was not).

## The architecture map

20. **The architecture map stays true.** Re-derive
    [`design/architecture-map.md`](design/architecture-map.md) from the code, not from the last
    map (the M3 closing audit's map, whose gates and fixes the owner approved on 2026-09-30).
    `make check` does the mechanical part of steps 1, 3 and 4
    (`scripts/check_architecture_map.py`, `TestArch_EachTableHasOneWritingPackage`); the rest is
    this item's.
    1. **Packages.** `go list ./internal/... ./cmd/...` and `web/packages/*`, `web/apps/*`: every
       package appears once, with one responsibility from the map's list, taken from its doc.go or
       README. A new package without a row, or a row whose package is gone, is a finding. A package
       whose doc.go now describes a second responsibility gets a ⚑ with file:line, or is split.
    2. **Two owners.** For each responsibility, list the packages doing it. Two packages doing the
       same kind of work (a wait loop, a retry schedule, a finding type, a header policy, a sweep
       loop) is a rule 11 finding: name both sites, and fix it or record why both stay.
    3. **Tables and files.** Every table in `internal/store/migrations/` and
       `internal/store/cache/schema.sql`, every file kind under the state, cache and prepared
       directories, and what `orpheum backup` writes, each has a tier, a home and a backup answer.
       Irreplaceable or expensive-to-re-acquire data outside library.db, expensive derived data in
       a store discarded by a version bump, and cheap derived data added to library.db are findings
       unless the map cites the decision that accepts them.
    4. **Writers.** For each library.db and cache.db table, list the packages that call its
       INSERT, UPDATE or DELETE queries. More than one writer package is a finding unless the map
       names the split per column.
    5. **Seams.** Every crossing the map lists (a core package imported by the node role,
       `sqlcgen` row types in api, a decision that reads the store, policy in SQL) is still listed
       or gone. A new crossing is added with evidence. A seam that an existing depguard rule or
       `TestArch_` test could hold is proposed to the owner as a gate.
    6. Record the date, what changed since the last map, and the findings in the milestone's audit
       note (plan-deviations.md).
21. **Hardcoded values** (the owner, 2026-09-30: "keep watch for hardcoded values that ought to be
    config"). Every number added since the last audit is one of: a setting with a starting value (an
    admin's choice about behaviour: how often, how long, how much; its unit spelled out in its key);
    a config key (a fact about the deployment, or a behaviour choice that start-up or a node needs
    before any database is open); or a named constant with a comment saying why it is fixed (a
    protocol value or an internal safety bound). Each setting's starting value names its source: a
    best guess and its basis, a measurement, or the owner's tuning.
22. **Promote tuned values** (the owner, 2026-09-30: "We start with a best guess and then refine as
    we discover things"). Read the settings the owner changed on their server (`GET /settings`
    with only the modified ones); for each, promote the owner's value to the starting value with
    its source updated, or record why it is particular to that server.
