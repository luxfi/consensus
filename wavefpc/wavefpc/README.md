# wavefpc — fast path certification ("wave")

This package adds a vote counter that rides inside ordinary blocks:
- Proposers bundle `FPCVotes` (tx refs) + optional `EpochBit`.
- Validators tally votes per tx, one vote per validator per owned object.
- `2f+1` votes ⇒ `Executable`.
- `Final` once covered by an accepted anchor (ancestry) or by a counted certificate in the DAG.
- Optionally, kick PQ signatures (Ringtail) in parallel when `Executable`.

No new RPCs, no changes to your sampling/threshold loop.
