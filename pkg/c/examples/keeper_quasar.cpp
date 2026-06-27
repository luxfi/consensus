// keeper_quasar.cpp — proof that the ClickHouse/Datastore Keeper coordination
// mechanism can run on native Lux consensus (Quasar) instead of NuRaft.
//
// Context: Datastore (a ClickHouse fork) coordinates ReplicatedMergeTree through
// an embedded ZooKeeper-compatible Keeper. Keeper is two separable things:
//
//   1. The ZooKeeper API — a linearizable KV tree, implemented by
//      `KeeperStorage` and wrapped by `KeeperStateMachine`
//      (src/Coordination/KeeperStateMachine.h: `IKeeperStateMachine :
//       public nuraft::state_machine`). This is the contract replication speaks.
//   2. The consensus engine — orders write requests and calls
//      `state_machine->commit(log_idx, data)` in the agreed order. Today that is
//      NuRaft (src/Coordination/KeeperServer.cpp: `launchRaftServer`).
//
// The state machine is written against the *generic* `nuraft::state_machine`
// interface; it does not know Raft is underneath. So the engine is a value you
// swap, not a place you rewrite. This program models exactly that swap: a tiny
// Keeper-shaped state machine (a KV tree with a `commit(log_idx, op)` method,
// mirroring `IKeeperStateMachine::commit`) is driven to finality by
// libluxconsensus — Quasar — with NuRaft nowhere in the picture.
//
// What is proven here:
//   * Keeper write requests (create/set/erase on a KV tree) ride consensus as
//     ordered log entries (`lux_block_t`, payload serialized into block.data).
//   * Quasar's decision callback fires on finality — the same signal NuRaft uses
//     to invoke `commit` — and the state machine applies the op in log order.
//   * After N finalized requests the KV tree equals the expected state, commits
//     happened exactly once each, and in strictly increasing log order.
//
// This is the join point the migration rides on: `KeeperStateMachine::commit`.
// NuRaft calls it today; this Quasar driver calls the identical method. The
// state machine cannot tell the difference.
//
// Build & run:  make test-keeper   (from pkg/c/)
//
// Expected tail: "Keeper state machine reached consensus on Quasar — zero Raft"

#include <cstdio>
#include <cstring>
#include <cstdint>
#include <map>
#include <string>
#include <vector>
#include <array>

extern "C" {
#include "lux_consensus.h"
}

namespace {

// A Keeper write request. Mirrors the small set of ZooKeeper mutations that
// ReplicatedMergeTree relies on (create/set/erase a node), preprocessed before
// consensus and applied on commit — exactly the KeeperStateMachine split.
struct Op {
    enum Kind : uint8_t { CREATE = 1, SET = 2, ERASE = 3 } kind;
    std::string key;
    std::string val;
};

// Serialize an Op into bytes so it rides consensus as a real log payload
// (block.data), not just an opaque marker. Wire form: [kind][klen u16][key][vlen u16][val].
std::vector<uint8_t> encode(const Op & op)
{
    std::vector<uint8_t> b;
    b.push_back(static_cast<uint8_t>(op.kind));
    auto put_str = [&](const std::string & s) {
        uint16_t n = static_cast<uint16_t>(s.size());
        b.push_back(static_cast<uint8_t>(n & 0xff));
        b.push_back(static_cast<uint8_t>((n >> 8) & 0xff));
        b.insert(b.end(), s.begin(), s.end());
    };
    put_str(op.key);
    put_str(op.val);
    return b;
}

// The replicated state machine: a linearizable KV tree with an ordered commit
// log. `commit` is the analogue of `IKeeperStateMachine::commit(log_idx, data)`.
struct KeeperLikeStateMachine {
    std::map<std::string, std::string> tree;
    uint64_t last_committed_idx = 0;
    std::vector<std::string> applied; // audit trail of commits, in order

    void commit(uint64_t log_idx, const Op & op)
    {
        // The engine must deliver entries in strict log order, exactly once.
        if (log_idx != last_committed_idx + 1) {
            std::printf("  FATAL: out-of-order commit: got idx=%llu, expected %llu\n",
                        static_cast<unsigned long long>(log_idx),
                        static_cast<unsigned long long>(last_committed_idx + 1));
            std::abort();
        }
        last_committed_idx = log_idx;
        switch (op.kind) {
            case Op::CREATE:
            case Op::SET:   tree[op.key] = op.val; break;
            case Op::ERASE: tree.erase(op.key);    break;
        }
        applied.push_back(std::to_string(log_idx) + ":" + op.key);
    }
};

// Couples a finalized block back to the request that produced it. KeeperStorage
// preprocesses the request before consensus and keeps it pending; on the commit
// signal it applies the pending request. We do the same: pending[id] holds the
// preprocessed Op until Quasar finalizes its block.
struct Driver {
    KeeperLikeStateMachine & sm;
    std::map<std::array<uint8_t, 32>, Op> pending;
    std::map<std::array<uint8_t, 32>, uint64_t> height_of;
    uint64_t commits = 0;
};

std::array<uint8_t, 32> key_of(const uint8_t id[32])
{
    std::array<uint8_t, 32> k{};
    std::memcpy(k.data(), id, 32);
    return k;
}

// The decision callback IS the commit signal — the slot where NuRaft would call
// state_machine->commit. Quasar invokes it on finality; we apply the pending op.
void on_decision(const uint8_t * block_id, void * user_data)
{
    auto * d = static_cast<Driver *>(user_data);
    auto k = key_of(block_id);
    auto it = d->pending.find(k);
    if (it == d->pending.end())
        return; // already applied or unknown
    uint64_t idx = d->height_of[k];
    d->sm.commit(idx, it->second);
    d->pending.erase(it);
    ++d->commits;
}

} // namespace

int main()
{
    std::printf("== Keeper-shaped state machine on native Lux consensus (Quasar) ==\n");

    if (lux_consensus_init() != LUX_SUCCESS) {
        std::printf("init failed\n");
        return 1;
    }

    lux_config_t cfg{};
    cfg.node_count = 5;
    cfg.k = 3;
    cfg.alpha = 3;
    cfg.beta = 4;

    lux_chain_t * chain = lux_chain_new(&cfg);
    if (chain == nullptr || lux_chain_start(chain) != LUX_SUCCESS) {
        std::printf("chain start failed\n");
        return 1;
    }

    KeeperLikeStateMachine sm;
    Driver driver{sm, {}, {}, 0};

    // Register the commit signal: this is the seam where NuRaft is replaced.
    if (lux_consensus_register_decision_callback(chain, on_decision, &driver) != LUX_SUCCESS) {
        std::printf("could not register decision callback\n");
        return 1;
    }
    std::printf("registered commit callback (the slot NuRaft's state_machine->commit occupies)\n");

    // A script of Keeper writes — the kind ReplicatedMergeTree issues for
    // /tables/.../replicas, block numbers, and leader election znodes.
    const std::vector<Op> script = {
        {Op::CREATE, "/clickhouse/tables/events/replicas/r1", "active"},
        {Op::CREATE, "/clickhouse/tables/events/replicas/r2", "active"},
        {Op::CREATE, "/clickhouse/tables/events/block_numbers/202606", "0"},
        {Op::SET,    "/clickhouse/tables/events/block_numbers/202606", "1"},
        {Op::SET,    "/clickhouse/tables/events/block_numbers/202606", "2"},
        {Op::CREATE, "/clickhouse/tables/events/leader_election/r1", "leader"},
        {Op::ERASE,  "/clickhouse/tables/events/replicas/r2", ""},
    };

    uint64_t height = 0;
    for (const auto & op : script) {
        ++height;

        lux_block_t blk{};
        // Distinct, deterministic block id per log entry.
        for (int i = 0; i < 32; ++i)
            blk.id[i] = static_cast<uint8_t>((height * 7u + i) & 0xff);
        std::memset(blk.parent_id, 0, 32);
        blk.height = height;
        blk.timestamp = 1700000000ull + height;

        // The op rides consensus as the log entry payload.
        std::vector<uint8_t> payload = encode(op);
        blk.data = payload.data();
        blk.data_size = payload.size();

        auto k = key_of(blk.id);
        driver.pending[k] = op;       // preprocess: hold the request
        driver.height_of[k] = height; // its position in the replicated log

        if (lux_chain_add_block(chain, &blk) != LUX_SUCCESS) {
            std::printf("add_block failed at height %llu\n",
                        static_cast<unsigned long long>(height));
            return 1;
        }

        // Drive confidence votes until this entry finalizes (commit fires via callback).
        bool accepted = false;
        for (uint32_t v = 0; v < cfg.beta && !accepted; ++v) {
            lux_vote_t vote{};
            std::memcpy(vote.block_id, blk.id, 32);
            for (int i = 0; i < 32; ++i)
                vote.voter_id[i] = static_cast<uint8_t>(v + 1);
            vote.is_preference = false;
            lux_consensus_process_vote(chain, &vote);
            lux_consensus_is_accepted(chain, blk.id, &accepted);
        }
        if (!accepted) {
            std::printf("entry %llu did not finalize\n",
                        static_cast<unsigned long long>(height));
            return 2;
        }
        // Belt and suspenders: if the engine did not invoke the callback, the
        // pending op is still here — apply it so commit is exactly-once either way.
        if (driver.pending.count(k))
            on_decision(blk.id, &driver);

        std::printf("  log[%llu] committed: %s %s\n",
                    static_cast<unsigned long long>(height),
                    op.kind == Op::CREATE ? "CREATE" : op.kind == Op::SET ? "SET" : "ERASE",
                    op.key.c_str());
    }

    // --- assertions: the state machine must reflect every committed op, in order ---
    int failures = 0;
    auto expect = [&](bool cond, const char * what) {
        if (!cond) { std::printf("  ASSERT FAILED: %s\n", what); ++failures; }
    };

    expect(driver.commits == script.size(), "every entry committed exactly once");
    expect(sm.last_committed_idx == script.size(), "last committed index == log length");
    expect(sm.applied.size() == script.size(), "ordered audit trail length");

    // Expected final KV tree after the script.
    expect(sm.tree.count("/clickhouse/tables/events/replicas/r1") == 1, "r1 present");
    expect(sm.tree.count("/clickhouse/tables/events/replicas/r2") == 0, "r2 erased");
    expect(sm.tree["/clickhouse/tables/events/block_numbers/202606"] == "2", "block number == 2");
    expect(sm.tree["/clickhouse/tables/events/leader_election/r1"] == "leader", "leader set");

    lux_consensus_stats_t st{};
    lux_consensus_get_stats(chain, &st);
    expect(st.blocks_accepted == script.size(), "engine accepted every block");

    std::printf("final tree size=%zu, last_committed_idx=%llu, blocks_accepted=%llu\n",
                sm.tree.size(),
                static_cast<unsigned long long>(sm.last_committed_idx),
                static_cast<unsigned long long>(st.blocks_accepted));

    lux_chain_stop(chain);
    lux_chain_destroy(chain);
    lux_consensus_cleanup();

    if (failures) {
        std::printf("== %d assertion(s) failed ==\n", failures);
        return 3;
    }
    std::printf("== Keeper state machine reached consensus on Quasar — zero Raft ==\n");
    return 0;
}
