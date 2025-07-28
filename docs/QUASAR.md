# Quasar: A Quantum-Secure Consensus Protocol Family for Lux Network

## Introduction and Motivation

Blockchain networks require robust consensus mechanisms to ensure all nodes agree on the ledger state. Classical consensus protocols (like PBFT) achieve safety through unanimous agreement but often suffer poor scalability due to high communication overhead ￼. Nakamoto-style protocols (as in Bitcoin) scale better but only offer probabilistic finality and waste energy on proof-of-work ￼ ￼. Avalanche introduced a breakthrough by combining the strengths of both approaches: it uses repeated random subsampling of validators (a gossip-based metastable mechanism) to achieve rapid, scalable consensus with high confidence ￼ ￼. Avalanche consensus blends classical and Nakamoto techniques, yielding high throughput and sub-second finality in practice ￼ ￼.

Lux Network’s Quasar consensus protocol family builds upon these ideas and extends them into the quantum computing era. The looming threat of quantum computers is that they could eventually break classical cryptography (like RSA or elliptic-curve signatures) via Shor’s algorithm ￼. With elliptic-curve schemes (including BLS12-381 used in many blockchains) estimated to require on the order of 2,330 logical qubits (and billions of operations) to crack a 128-bit curve ￼, large-scale quantum attacks are at least a decade away and currently infeasible ￼. However, Lux Network aims to be future-proof by incorporating post-quantum security now. The Quasar protocol introduces a dual-certificate finality mechanism that combines classical cryptography (BLS signatures) with post-quantum cryptography (lattice-based threshold signatures) to secure consensus even against quantum adversaries. This report provides a comprehensive overview of the Quasar consensus architecture, details of its quantum-secure design, and background on the full consensus stack developed by Lux Network.

## The Lux Network Consensus Stack Evolution

Quasar is the culmination of a series of consensus protocol stages developed for Lux Network, analogous to how Avalanche’s Snow family (Slush, Snowflake, Snowball, etc.) builds up to the Avalanche and Snowman protocols ￼ ￼. The Lux stack consists of the following stages (from simplest to most advanced):

- **Photon – Binary consensus on a single bit.** This is the foundation: a minimal consensus algorithm that decides a boolean value in a Byzantine setting. Photon is akin to Avalanche’s Snowflake protocol, which augments a basic gossip consensus with a single confidence counter ￼. Each node repeatedly queries a small random set of validators about a binary decision; if a strong majority favors a value, the node adopts that value. If the node receives the same answer α-majority repeatedly, a counter increases; once it sees β consecutive successful rounds, the decision is finalized ￼ ￼. Photon provides probabilistic agreement on binary outcomes with high confidence.

- **Wave – Multi-bit consensus.** Wave extends Photon beyond a single bit to make decisions on multiple-bit or multi-valued inputs. In practice, this means Wave can handle choices among more than two options (e.g. choosing among competing transactions or blocks) rather than a strict binary yes/no. This is comparable to Avalanche’s progression from Snowflake to Snowball, which generalizes the binary decision to multi-decree consensus using confidence counters for each choice ￼ ￼. Wave inherits Photon’s repeated sub-sampled voting but can distinguish and converge on one value among many by requiring a preference to gather a large enough sample majority (α threshold) over all others.

- **Focus – Confidence aggregation.** Focus introduces persistent counters that accumulate confidence over multiple rounds of sampling. This stage mirrors the Snowball algorithm from Avalanche, where each node not only tracks the current preferred decision but also how many successful rounds each option has gained so far ￼. Every time a query yields a majority for an option, the node increments a confidence counter for that option ￼. This mechanism makes it much harder for the network to reverse its decision once momentum builds, hence providing stronger metastability. In Focus, a node will only finalize a decision once it has seen a preferred outcome repeatedly (above some β threshold for consecutive successes) and that outcome has accumulated significant total support across rounds. The Focus stage ensures that consensus decisions are robust against sporadic network fluctuations – honest validators rapidly coalesce on one outcome and remain locked in ￼ ￼.

- **Beam – Linear chain consensus.** Beam is the Lux counterpart to Avalanche’s Snowman protocol ￼. Snowman is essentially the Avalanche consensus adapted to a totally-ordered chain of blocks, appropriate for smart contract chains that require sequential block production ￼ ￼. Similarly, Beam operates on a linear sequence of blocks (one block after another in a chain). It leverages the earlier stages (Photon/Wave/Focus) to decide on each block with the same subsampled voting approach, but constrains votes to maintain a single, growing chain (no forks). Beam can be thought of as Snowman++, an enhanced linear consensus incorporating Lux’s improvements. It provides total ordering of transactions with high throughput and fast finality, making it suitable for chains where every block has a unique predecessor (e.g. Lux’s contract chain). Beam retains the safety and liveness properties of Avalanche’s Snowman: it achieves deterministic ordering and finality of blocks typically within a few rounds of voting, thanks to the positive feedback mechanism ￼.

- **Flare – State synchronization.** Flare is a specialized stage focusing on bootstrapping and synchronizing node state, rather than deciding block contents. In a distributed network, nodes joining or recovering need to catch up to the latest state of the blockchain. Flare handles state sync and bootstrapping: it ensures that a node can quickly download and verify the current chain or DAG state and start participating in consensus. While not a consensus protocol per se, Flare integrates with Beam and Nova (the next stage) to guarantee that all validators begin from a consistent state before they start voting. This involves efficient state transfer protocols, snapshot awareness, and possibly backward syncing of recent blocks (like Avalanche’s bootstrapping procedure on P-Chain/X-Chain). By providing a rapid state synchronization mechanism, Flare minimizes downtime and ensures new or lagging nodes do not weaken consensus due to outdated views.

- **Nova – DAG-based consensus.** Nova is the Lux Network’s directed-acyclic-graph consensus stage, analogous to Avalanche’s Avalanche (DAG) protocol. In Avalanche, the original protocol (sometimes just called Avalanche consensus) works on a DAG of dependent transactions, allowing parallel decisions and no global linear chain ￼. Nova similarly generalizes the consensus to a DAG of vertices/blocks, meaning multiple blocks or transactions can be decided concurrently as long as they do not conflict. Nova builds on Focus (Snowball) but in a multi-node DAG context: each vertex in the DAG (representing a transaction or a batch of transactions) is voted on via repeated random subsampling. Edges in the DAG represent dependencies (e.g. transaction A must be decided before transaction B if B spends an output from A). Nova achieves consistency by ensuring that if any conflict exists (like two transactions spending the same input), at most one will become finalized – the metastable voting will cause one branch of the conflict to garner more support, and the other to be abandoned ￼. Nova’s DAG consensus yields extremely high throughput (many decisions per round) and retains Avalanche’s property that honest nodes will irreversibly commit to the same outcomes with very high probability ￼ ￼.

- **Quasar – Quantum-secure consensus overlay.** Quasar is the pinnacle of the stack, adding an overlay of quantum security on top of the classical consensus stages. It introduces a dual-certificate finality mechanism to the consensus decisions made by Beam or Nova. In other words, Quasar wraps around the outcome of the earlier protocols (whether it’s a linear chain block from Beam or a DAG vertex from Nova) and requires two distinct cryptographic certificates for a block/transaction to be considered finalized. One certificate is produced using classical cryptography (BLS signatures) and the other using a post-quantum scheme (a lattice-based threshold signature, referred to as Ringtail in Lux’s implementation). A block is final if and only if both certificates are present and valid:

```go
// Block is final IFF both certificates are valid
isFinal = verifyBLS(blsAgg, Q) && verifyRT(rtCert, Q)
```

In the above logic, blsAgg is an aggregated BLS signature from a supermajority of validators, and rtCert is a Ringtail threshold signature from a (possibly overlapping) threshold of validators. Quasar thus ensures that even a powerful adversary equipped with a quantum computer cannot single-handedly forge finality on a block, since they would need to break both signature schemes (one of which is quantum-resistant by design).

## Core Innovation: Dual-Certificate Finality for Quantum Security

The defining innovation of Quasar is its dual-certificate finality mechanism. Finality in Quasar requires two cryptographic certificates on each decided block:
1. **BLS Aggregated Signature Certificate (Classical Security):** The first certificate is an aggregated BLS signature from the validator set. Each validator signs the block using their BLS12-381 private key, and these signatures are combined into a single compact aggregate. This aggregate guarantees classical 128-bit security and efficient verification.
2. **Ringtail Threshold Signature Certificate (Post-Quantum Security):** The second certificate is a Ringtail threshold signature, providing lattice-based post-quantum security against quantum adversaries. Validators hold key shares and collectively produce a threshold signature on the block.

A block in Quasar is considered finalized only when both the BLS and the Ringtail certificates are present and valid. This dual requirement means that an attacker must defeat both cryptographic defenses to forge a final block.

## System Architecture and Components

Quasar is implemented as a modular system within the Lux Network node software (`luxd`). The directory structure highlights key components:

```
/quasar/
├── choices/          # Consensus decision states and interfaces
├── consensus/        # Core consensus algorithms (beam, nova)
├── crypto/           # Cryptographic primitives (bls, ringtail)
├── engine/           # Consensus engines integrating logic and networking
├── networking/       # P2P network layer (handler, router, sender)
├── validators/       # Validator set management and key shares
└── uptime/           # Validator uptime tracking and monitoring
```

This architecture cleanly separates consensus logic from networking, allowing each layer to evolve independently and plugins to be added with minimal friction.

## Consensus Flow: From Transaction to Finalization

1. **Transaction Submission:** Clients submit transactions to node mempools and gossip them to peers.
2. **Block Proposal:** A rotating proposer forms a block (Beam) or vertex (Nova), signs it with BLS, and initiates Ringtail share creation.
3. **Share Collection:** Validators verify the proposal, create Ringtail signature shares, and send them to the proposer.
4. **Certificate Aggregation:** The proposer aggregates BLS signatures and Ringtail shares into dual certificates attached to the block.
5. **Consensus Voting:** Validators verify both certificates and finalize the block via Avalanche-style sampling polls.
6. **Finalization:** A block is finalized once it carries valid BLS and Ringtail certificates and consensus votes exceed safety thresholds.

## Performance Characteristics

Lux mainnet (21 validators) metrics:
- **Block Time:** ~500 ms
- **Dual-Cert Finality Latency:** < 350 ms (295 ms for BLS, ~7 ms for Ringtail share aggregation, ~50 ms network overhead)

These results demonstrate that post-quantum finality can be achieved with negligible performance impact.

## Network and Security Considerations

- **Classical BFT Safety:** Avalanche-style subsampled voting with BLS provides safety under f ≤ ⌊(αₚ−1)/2⌋ Byzantine faults.
- **Post-Quantum Security:** Ringtail threshold signatures resist quantum attacks under standard lattice assumptions (Module-LWE).
- **Dual Validation:** Both certificates required for finality protect against single-scheme failures.
- **Slashing:** Validators that misbehave (double-signing, missing PQ shares) are economically penalized.
- **Liveness:** Threshold Q < total validators ensures progress even if some validators are offline.

## Deployment and Configuration

Consensus parameters for mainnet vs testnet:

| Network  | K  | αₚ | α𝚌 | β | QThreshold | Timeout |
|----------|----|----|----|---|------------|---------|
| Mainnet  | 21 | 13 | 18 | 8 | 15         | 50 ms   |
| Testnet  | 11 | 7  | 9  | 6 | 8          | 100 ms  |

These values balance safety, liveness, and performance for each environment.

## Future Directions and Enhancements

- Dynamic validator set reconfiguration with rapid DKG.
- Cross-subnet atomic swaps leveraging fast finality.
- Light client proofs of dual-certificate finality.
- Mobile validator support via optimized cryptography.
- HSM and secure enclave integration for key protection.

## Conclusion

Quasar unifies Avalanche-style consensus with post-quantum threshold cryptography. By requiring dual certificates, Lux Network ensures fast, final, and quantum-secure consensus. Its modular architecture supports both linear and DAG chain topologies and paves the way for future innovations in blockchain security.

## References

1. NTT Research, “Ringtail: World’s first two-round post-quantum threshold signature scheme,” Press Release, May 2025.
2. Avalanche Builder Docs, “Avalanche Consensus,” build.avax.network/docs/quick-start/avalanche-consensus.
3. Medium (0xbarchitect), “BLS12-381 and BLS Signatures,” Sep 2023.
4. Meri.Garden (Wikipedia excerpt), “Elliptic Curve Cryptography – Quantum Computing Attack,” Aug 2023.
5. Binance Academy, “Slashing,” glossary.
6. Team Rocket (Anonymous), “Snowflake to Avalanche: A Novel Metastable Consensus Protocol Family,” May 2018.