// cpp_consumer.cpp — proof that a C++ translation unit drives the native Lux
// consensus engine directly through the C ABI, with no Go runtime, no gRPC, and
// no zero-knowledge layer.
//
// Why this exists: a columnar engine such as the ClickHouse-derived Hanzo
// Datastore is C++. Today it coordinates its ReplicatedMergeTree log through an
// embedded Raft/Keeper (ZooKeeper API). The goal is to replace that Raft backend
// with the leaderless Lux consensus (Quasar) *natively* — i.e. by linking
// libluxconsensus into the C++ process, not by porting Go to C++. This example
// is the smallest end-to-end demonstration that the seam holds: a C++ program
// links the static library, opens a chain, appends a block (a stand-in for a
// replication-log entry / part manifest), drives confidence votes from a
// validator set, and observes the engine finalize the block.
//
// Build & run:  make test-cpp   (from pkg/c/)
//
// Expected tail:  "C++ reached consensus finality — zero Raft, zero ZK"

#include <cstdio>
#include <cstring>
#include <cstdint>

extern "C" {
#include "lux_consensus.h"
}

int main() {
    std::printf("== C++ consumer of native Lux consensus (libluxconsensus) ==\n");

    if (lux_consensus_init() != LUX_SUCCESS) {
        std::printf("init failed\n");
        return 1;
    }

    // node_count/k/alpha/beta are the Snow-family sampling parameters; beta is
    // the confidence threshold for finalization (see check_decision_threshold).
    lux_config_t cfg{};
    cfg.node_count = 5;
    cfg.k = 3;
    cfg.alpha = 3;
    cfg.beta = 4;

    lux_chain_t* chain = lux_chain_new(&cfg);
    if (chain == nullptr || lux_chain_start(chain) != LUX_SUCCESS) {
        std::printf("chain start failed\n");
        return 1;
    }
    std::printf("chain started (k=%u alpha=%u beta=%u) — this is the slot Raft/Keeper occupies today\n",
                cfg.k, cfg.alpha, cfg.beta);

    // A block here models one ordered replication-log entry (e.g. a datastore
    // part-manifest commitment) — the small, ordered datum that must reach
    // consensus, while the bulk columnar data stays off-chain.
    lux_block_t blk{};
    for (int i = 0; i < 32; ++i) {
        blk.id[i] = static_cast<uint8_t>(0xA0 + i);
        blk.parent_id[i] = 0;
    }
    blk.height = 1;
    blk.timestamp = 1700000000;
    blk.data = nullptr;
    blk.data_size = 0;

    if (lux_chain_add_block(chain, &blk) != LUX_SUCCESS) {
        std::printf("add_block failed\n");
        return 1;
    }
    std::printf("appended log entry (height=1)\n");

    // Drive confidence votes from the validator set until the engine finalizes.
    bool accepted = false;
    for (uint32_t v = 0; v < cfg.beta && !accepted; ++v) {
        lux_vote_t vote{};
        std::memcpy(vote.block_id, blk.id, 32);
        for (int i = 0; i < 32; ++i) {
            vote.voter_id[i] = static_cast<uint8_t>(v + 1);
        }
        vote.is_preference = false;  // confidence vote
        lux_consensus_process_vote(chain, &vote);
        lux_consensus_is_accepted(chain, blk.id, &accepted);
        std::printf("  validator %u voted -> accepted=%s\n", v + 1, accepted ? "true" : "false");
    }

    lux_consensus_stats_t st{};
    lux_consensus_get_stats(chain, &st);
    std::printf("finalized: accepted=%s blocks_accepted=%llu votes_processed=%llu\n",
                accepted ? "YES" : "no",
                static_cast<unsigned long long>(st.blocks_accepted),
                static_cast<unsigned long long>(st.votes_processed));

    lux_chain_stop(chain);
    lux_chain_destroy(chain);
    lux_consensus_cleanup();

    if (!accepted) {
        std::printf("== did NOT finalize ==\n");
        return 2;
    }
    std::printf("== C++ reached consensus finality — zero Raft, zero ZK ==\n");
    return 0;
}
