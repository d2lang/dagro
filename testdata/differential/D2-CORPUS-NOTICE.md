# D2 corpus attribution

The JSON under `d2-corpus/` and the three compatibility cases named
`regression/overlapping-edge-label/dagre`, `txtar/theme-overrides/dagre`, and
`stable/us_map/dagre` was derived from D2's Dagre adapter while replaying D2
E2E fixtures at commit `1a60d69e4df9b9557923e61bf10f9aa3aa5422e1`.
This includes the corresponding inputs and expected files; the manifests
record provenance for both these D2-derived files and the synthetic case below.
Expected files preserve D2-derived node IDs, edge names, topology, and
ordering.

D2 is Copyright 2022 Terrastruct Inc. and is licensed under the Mozilla Public
License 2.0. A verbatim copy of D2's license at that revision is included as
`D2-LICENSE.txt`. The corpus inputs were normalized to the adapter's JSON wire
format, deduplicated, and named by their SHA-256 content hash. All of this JSON
retains D2's copyright and MPL-2.0 coverage.

The synthetic `d2-profile/multiple-reversed-parallel-partners` compatibility
case was minimized from Dagro's deterministic D2-profile generator and is
licensed with Dagro under MIT; it is not copied from D2's E2E corpus.

The files under `d2-corpus/expected/` and the three D2-derived compatibility
expected files additionally contain generated layout geometry from the pinned
MIT-licensed Dagre 3.1.1 oracle or the documented compatibility patch. The
synthetic case's expected file contains only generated compatibility layout
geometry. Sources and SHA-256 hashes are recorded in the corresponding
manifests.
