# D2 corpus attribution

The JSON under `d2-corpus/` and `compatibility/` was derived from D2's Dagre
adapter while replaying D2 E2E fixtures at commit
`1a60d69e4df9b9557923e61bf10f9aa3aa5422e1`. This includes inputs, expected
files, and manifests: expected files preserve D2-derived node IDs, edge names,
topology, and ordering.

D2 is Copyright 2022 Terrastruct Inc. and is licensed under the Mozilla Public
License 2.0. A verbatim copy of D2's license at that revision is included as
`D2-LICENSE.txt`. The corpus inputs were normalized to the adapter's JSON wire
format, deduplicated, and named by their SHA-256 content hash. All of this JSON
retains D2's copyright and MPL-2.0 coverage.

The files under `d2-corpus/expected/` and `compatibility/expected/`
additionally contain generated layout geometry from the pinned MIT-licensed
Dagre 3.1.1 oracle or the documented compatibility patch. Their source and
SHA-256 hashes are recorded in the corresponding manifests.
