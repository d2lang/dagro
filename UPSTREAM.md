# Upstream provenance and compatibility boundary

Dagro implements the default Dagre layout path used by D2. The version
constants identify behavioral source revisions, not complete API coverage.

## Pinned sources

| Source | Revision | Test artifact |
| --- | --- | --- |
| `@dagrejs/dagre` 3.1.1 | `c3ed0802cd98de74c21cff1f754689ebbb0f8dae` | npm integrity `sha512-zroZB1dFOFiGgv4Xcrn1DckB1o4aOikPqD2NDQPV0WM//CXGcS6xiD0rNkqHmw6FEg4tabt4nxPLwgCWT+Vb2A==`; `dist/dagre.cjs` SHA-256 `70b9a4367932dd436075d98892a7968d65cf66ae83263f995e0531823b59b671` |
| `@dagrejs/graphlib` 4.0.5 | `d3a0cf36f55ebd75f28b6acf7a436a54e1b990dc` | npm integrity `sha512-7xrBTqIts3o+PMUZX97wSc+7TUbW+/rULzGNCTP6yooNVDXbzw4Wutg/H/xOutTB/c/k0YqOAavgPh4/Zk9PFA==`; `dist/graphlib.cjs` SHA-256 `271f39d50dbcf2f795808cb4f5b90fb42a096b5f84b4dd6bb672487b454011e7` |

The test-only `package.json` and lockfile pin those exact packages. CI pins
Node 24.19.0 and uses `npm ci --ignore-scripts`; neither JavaScript package is
a production dependency.

The compatibility oracle is rebuilt from the exact Dagre commit by
`testdata/differential/build-dagre-3.1.1-d2-compat.sh`. The builder verifies
upstream `package-lock.json` SHA-256
`9f5e1e7a40667dcffc12e35ea5d4db96f346dadf943e97c1ecc2b4dc21afbb2d`,
applies the pinned source patch, runs the upstream build with esbuild 0.27.3,
tsx 4.21.0, TypeScript 5.9.3, and Graphlib 4.0.5, and verifies patched
`dist/dagre.js` SHA-256
`9b91fccee8e70a74299cf47eaf8100c46a900fb1f334a11424ff3682c1019585`.
The generated test-only CommonJS oracle has SHA-256
`8e34c25ed53dbccca2fa206780b0b46974b285c74e0cd7b34d0d1fafa5506cab`;
it is generated in CI and is not checked in or shipped.

## Source map

| Upstream source | Dagro implementation |
| --- | --- |
| `lib/layout.ts`, `lib/coordinate-system.ts`, `lib/position/index.ts` | `layout.go`, `coordinate_system.go`, `position.go` |
| `lib/acyclic.ts`, `lib/greedy-fas.ts` | `acyclic.go`, `greedy_fas.go` |
| `lib/nesting-graph.ts`, `lib/normalize.ts`, `lib/parent-dummy-chains.ts`, `lib/add-border-segments.ts` | `nesting_graph.go`, `normalize.go`, `parent_dummy_chains.go`, `add_border_segments.go` |
| `lib/rank/*.ts` | `rank.go`, `rank_util.go`, `feasible_tree.go`, `network_simplex.go` |
| `lib/order/*.ts` | `order.go` and `order_*.go` |
| `lib/position/bk.ts` | `position_bk.go` |
| `lib/util.ts`, `lib/data/list.ts` | `util.go`, `numeric_semantics.go`, `list.go` |
| Graphlib graph operations used by D2 and Dagre's default path | `graph.go`, `callback.go`, and ordered collection helpers |

## Verified D2 profile

The release contract is the directed, compound, named-multiedge profile used
by D2: graph `rankdir`, `nodesep`, `edgesep`, and `ranksep`; node IDs,
parentage, width, and height; and named edges with width, height, and
`labelpos`. The observable outputs are graph bounds, node and edge-label
coordinates, and ordered edge points.

The checked corpus was captured from D2 commit
`1a60d69e4df9b9557923e61bf10f9aa3aa5422e1`. Across 313 D2 Dagre E2E cases,
270 invoked the adapter, producing 349 calls and 311 unique layout inputs.
Official Dagre 3.1.1 returned for 309 inputs, but only 308 results were fully
finite. The exact expected-output partition is therefore:

- 308 bit-for-bit official 3.1.1 outputs after D2's JSON bridge
  normalization;
- three named compatibility outputs: one replaces a successful but
  non-finite official result, and two replace official errors.

The 311-input corpus and the first three named compatibility cases are derived
from D2's MPL-2.0 E2E fixtures and retain D2's copyright and license coverage;
see `testdata/differential/D2-CORPUS-NOTICE.md` and `D2-LICENSE.txt`. Their
expected JSON additionally contains generated layout geometry from the pinned
MIT oracle or documented compatibility patch. The synthetic fourth case is
generated under Dagro's MIT license. Sources and hashes are recorded in the
manifests.

The compatibility cases are:

| D2 case | Content-addressed input ID | Official behavior |
| --- | --- | --- |
| `regression/overlapping-edge-label/dagre` | `7cfd90e29056db3a1a4d2b45690869ff537734eb5d809ab5f1bb832e59a0bc67` | returns non-finite geometry |
| `txtar/theme-overrides/dagre` | `e052b4c21cba3edb2df9b001d3f64058bf66eb4b30aeec321afa063278915d88` | throws during rectangle intersection |
| `stable/us_map/dagre` | `e2cfd977b7a3bf293fced2080851d9ce8e6bf5425153799b0f3efed58ac27853` | throws during rectangle intersection |
| `d2-profile/multiple-reversed-parallel-partners` | `297220aa20dc2b11460f16bd2c6387f96a4fba40497a7ce71296b45bae600cc4` | returns a finite but different layout; synthetic MIT-licensed regression that pins the composition of the two ordering corrections, not a captured D2 fixture |

`TestLayoutMatchesD2Corpus` verifies the partition, every input and expected
output hash, finiteness, and exact `float64` bits. The pinned JavaScript oracle
also checks ordinary fixtures directly and confirms all four documented
upstream behaviors rather than silently skipping them.

## Compatibility corrections

The reviewable patch
`testdata/differential/dagre-3.1.1-d2-compat.patch` has SHA-256
`d510c474e1f291c38c14c276f6bc498dbbd0dc7132e3b9694e31b47650356d19`
and applies to the pinned Dagre commit. It makes three changes:

1. The reversed-parallel-edge ordering special case applies only when exactly
   one dummy represents a reversed edge. Official 3.1.1 can otherwise drop a
   same-direction dummy from a group of three and propagate non-finite
   geometry.
2. Every reversed partner for the same forward dummy is retained in encounter
   order. With correction 1 applied, upstream's single-valued object table can
   overwrite an earlier opposite-direction partner and leave a valid
   D2-profile graph without coordinates.
3. A rectangle intersection at the exact center of a zero-width or
   zero-height node returns the center, and axis-aligned intersections avoid
   division by zero.

Dagro has one additional public-API safety correction: generated `revN` names
are collision-checked before an internal reversed edge is inserted. This
prevents a caller's named multiedge from being overwritten and removes that
collision's dependence on global dummy-ID history. Ordinary non-colliding
names preserve the official first candidate.

The raw seven-point self-loop route intentionally remains identical to Dagre
3.1.1. Any D2 curve or endpoint-chopping normalization belongs in the D2
adapter and is not hidden in Dagro.

## Out of scope

No parity claim is made for dynamic remembered layout state, per-cluster
direction, `customOrder`, ordering constraints, manual rank APIs, `rankalign`,
or other Dagre 3.x surfaces D2 does not use. Graphlib coverage is likewise the
D2-used public graph operations plus the operations required internally by
the verified layout path, not all of Graphlib 4.0.5.
