package chain

// Proposal-identity stability — the 8-gate verification matrix for the fix in
// buildBlocksLocked + ownUndecidedCandidateLocked (branch fix/proposal-identity-stability).
//
// ROOT CAUSE (owner-diagnosed, PROVEN live on mainnet 2026-07-03): luxd-0 rebuilt block
// 1082880 with a CHANGING blkID every ~16s — proven with the heartbeat PAUSED + a clean
// mempool + one tx, so it is NOT gas-escalator/mempool-specific — because proposervm
// re-wraps the height into a fresh envelope ID each build tick off the stale 1082879
// parent. buildBlocksLocked's exact-ID dedup misses the re-wrap, adds a NEW sibling at
// (parent,height) every tick, votes scatter across siblings, none reaches α=4 → no cert
// → frozen. The fix stabilizes proposal identity: while our own proposal at (parent,
// height) is undecided, re-solicit THAT candidate instead of adding a replacement.
//
// These 8 gates MUST all pass before the fix ships to mainnet (owner-required matrix).
// Each is a real test, not a placeholder — Blue implements the bodies against the live
// Transitive + a mock BlockBuilder whose BuildBlock returns a DIFFERENT envelope ID each
// call for the same inner (parent,height) (the proposervm re-wrap the fix targets), and
// RED adversarially re-verifies. WITHOUT the fix each churn test fails (siblings grow,
// no cert); WITH the fix one candidate reaches α and the height decides.
//
// Gate 1 — Churn repro (the core liveness proof).
//   n=5, α=4, stable parent P at height H. BuildBlock returns a fresh envelope ID on
//   every call for the same inner (P,H+1). PRE-fix: len(pendingBlocks) grows one sibling
//   per build tick, no candidate reaches α, no cert. POST-fix: exactly ONE own candidate
//   is tracked at (P,H+1), its votes accumulate to α=4, the α-of-K cert forms, H+1 decides.
//   TestReproposalChurn_StableCandidateReachesAlpha
//
// Gate 2 — No false collapse across parent (the highest-risk correctness gate).
//   Same height H+1, DIFFERENT parent (P vs P'). ownUndecidedCandidateLocked(P',H+1) must
//   NOT return the candidate built on P. A genuine new-parent proposal builds normally.
//   TestProposalStability_DifferentParentNotCollapsed
//
// Gate 3 — No false collapse after decision.
//   A candidate at (P,H+1) that has DECIDED (Decided=true, or finalized+pruned) must not
//   be reused; a subsequent build at (P,H+1) is a no-op/normal, never re-solicits a
//   decided block. TestProposalStability_DecidedCandidateNotReused
//
// Gate 4 — No false collapse of a remote proposal.
//   A GOSSIPED (IsOwnProposal=false) block at (P,H+1) must NOT be returned by
//   ownUndecidedCandidateLocked — only our own undecided proposal gets stability reuse
//   (a peer's block keeps the rePoll-attempt cap). TestProposalStability_RemoteNotCollapsed
//
// Gate 5 — View/slot replacement allowed.
//   If the proposer VIEW/SLOT changes (round advance / view-change), a NEW candidate is
//   permitted per the protocol rule — stability holds a candidate only within the same
//   view. Assert a view change does not wedge us onto a stale candidate.
//   TestProposalStability_ViewChangeAllowsReplacement
//
// Gate 6 — No double-finalize (the safety gate).
//   Old sibling votes cannot be mixed into a new candidate; two candidates at the same
//   height cannot both decide. Even under churn + an equivocating/lagging voter, at most
//   ONE block finalizes at H+1 (decided-floor + one-precommit-per-round hold; the fix
//   adds no vote and lowers no threshold). TestProposalStability_NoDoubleFinalize
//
// Gate 7 — n-matrix quorum unchanged.
//   Strict quorum behavior at 1/1, 2/2, 3/3, 3/4, 4/5, 5/6 is unchanged by the fix
//   (α = ⌊2k/3⌋+1 per row; K==1 finalizes inline with no re-solicit).
//   TestProposalStability_QuorumMatrixUnchanged
//
// Gate 8 — Mainnet repro model (the end-to-end shape of the live incident).
//   n=5, α=4, envelope churn every build tick off a stale parent (the exact 1082880
//   condition). POST-fix: one stable blkID reaches α=4, H+1 decides, and the NEXT height
//   continues (sustained, not "one block then hang"). TestProposalStability_MainnetReproModel
