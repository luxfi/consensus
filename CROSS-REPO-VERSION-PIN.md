# Cross-Repo Version Pin — March 3, 2026 PQ Consensus Architecture Freeze

This document is the canonical record of which commit SHAs the
`v0.1.0-rc1-pq-consensus-freeze` tag points at in every repo that
participates in the Lux post-quantum consensus stack.

## Tag

Every repo below is tagged annotated with:

```
git tag -a v0.1.0-rc1-pq-consensus-freeze \
  -m "Mar 3 2026 Lux PQ Consensus Architecture Freeze (locked scope)"
```

## Repos and SHAs

| Repo | Path | Tag | Commit SHA |
|------|------|-----|------------|
| pulsar | `~/work/lux/pulsar` | `v0.1.0-rc1-pq-consensus-freeze` | 7b3a53f0ba569a7092fe969c759bfbb28f5341ec |
| lens | `~/work/lux/lens` | `v0.1.0-rc1-pq-consensus-freeze` | 72d0dd6c470caab0689ca1a1e59881c044cc5cb6 |
| threshold | `~/work/lux/threshold` | `v0.1.0-rc1-pq-consensus-freeze` | e4e6be547b7db23726f0b18474bd00ea9801592c |
| consensus | `~/work/lux/consensus` | `v0.1.0-rc1-pq-consensus-freeze` | 853c48cb3149fd93b9517a7818a6200b4d9557f4 |
| warp | `~/work/lux/warp` | `v0.1.0-rc1-pq-consensus-freeze` | d4237f861b6b28aca6a6ccb8693a65a769205b18 |
| proofs | `~/work/lux/proofs` | `v0.1.0-rc1-pq-consensus-freeze` | cf682ab2fb1b233331f0880d6496f05883ff6583 |
| papers | `~/work/lux/papers` | `v0.1.0-rc1-pq-consensus-freeze` | 3529523e60c0f03550705cdd526bd2258db71c58 |
| lps | `~/work/lux/lps` | `v0.1.0-rc1-pq-consensus-freeze` | c582b6d3e3c2dc0e3904d211a13c6b6e0a962c42 |

These SHAs are produced by amending each repo's existing 2026-03-03
HEAD with the gate-1/2/5 deliverables and tagging that amended
commit. The annotated tag message is:
"Mar 3 2026 Lux PQ Consensus Architecture Freeze (locked scope)".

The consensus row records the SHA of the amend that immediately
precedes the final SHA-table commit (since this file LIVES IN that
final commit and would otherwise need to record its own SHA, which
is a chicken-and-egg). Run `git log -1 --format=%H v0.1.0-rc1-pq-consensus-freeze`
inside `~/work/lux/consensus` to see the actual tagged commit.

## go.mod Pinning

The pulled-in repos (consensus, threshold, warp) declare the tag as
their `require` version *and* keep the local-dev `replace` directive
pointing at the sister checkout. Replace lines carry a comment:

```
// pinned to v0.1.0-rc1-pq-consensus-freeze; see go-mod-pin.md
```

This lets local development proceed against the working tree without
relying on a published tagged module proxy. CI builds without the
neighbouring checkouts use the tagged version directly.

### consensus/go.mod

```
require (
    github.com/luxfi/pulsar    v0.1.0-rc1-pq-consensus-freeze
    github.com/luxfi/threshold v0.1.0-rc1-pq-consensus-freeze
    github.com/luxfi/lens      v0.1.0-rc1-pq-consensus-freeze // indirect
)
replace github.com/luxfi/pulsar    => ../pulsar    // pinned to v0.1.0-rc1-pq-consensus-freeze; see go-mod-pin.md
replace github.com/luxfi/threshold => ../threshold // pinned to v0.1.0-rc1-pq-consensus-freeze; see go-mod-pin.md
replace github.com/luxfi/lens      => ../lens      // pinned to v0.1.0-rc1-pq-consensus-freeze; see go-mod-pin.md
```

### threshold/go.mod

```
require (
    github.com/luxfi/lens   v0.1.0-rc1-pq-consensus-freeze
    github.com/luxfi/pulsar v0.1.0-rc1-pq-consensus-freeze
)
replace (
    github.com/luxfi/lens   => ../lens   // pinned to v0.1.0-rc1-pq-consensus-freeze; see go-mod-pin.md
    github.com/luxfi/pulsar => ../pulsar // pinned to v0.1.0-rc1-pq-consensus-freeze; see go-mod-pin.md
)
```

### warp/go.mod

```
require github.com/luxfi/pulsar v0.1.0-rc1-pq-consensus-freeze
replace github.com/luxfi/pulsar => ../pulsar // pinned to v0.1.0-rc1-pq-consensus-freeze; see go-mod-pin.md
```

## Verifying the Pin

A canonical proof point is whether `~/work/lux/scripts/regen-all-kats.sh
--verify` passes against the tagged commit on each repo. That script
runs:

- `pulsar/scripts/regen-kats.sh --verify`
- `lens/scripts/regen-kats.sh --verify`
- `warp/scripts/regen-kats.sh --verify`
- `threshold/scripts/regen-kats.sh --verify`

and asserts byte-equality of all KAT outputs against the manifest
each repo's regen-kats.sh writes.

## Forward Path

The next freeze should bump the tag (e.g. `v0.1.0-rc2-...`) and
update both the `require` lines and the comments on the `replace`
lines. The `replace` directives stay pointing at the local working
tree — they're the local-dev affordance and never go away.
