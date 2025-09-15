#include <lux/consensus.hpp>
#include <iostream>
#include <cassert>

using namespace lux::consensus;

void test_basic_consensus() {
    ConsensusParams params{
        .k = 20,
        .alpha_preference = 15,
        .alpha_confidence = 15,
        .beta = 20
    };
    
    auto consensus = Consensus::create(EngineType::Snowball, params);
    assert(consensus != nullptr);
    
    Block block{
        .id = 1,
        .parent_id = 0,
        .height = 1,
        .timestamp = std::chrono::system_clock::now(),
        .data = {0x01, 0x02, 0x03}
    };
    
    consensus->add_block(block);
    
    // Simulate votes
    for (int i = 0; i < 20; ++i) {
        Vote vote{
            .engine_type = EngineType::Snowball,
            .node_id = static_cast<uint16_t>(i),
            .block_id = 1,
            .vote_type = VoteType::Prefer
        };
        consensus->process_vote(vote);
    }
    
    assert(consensus->is_accepted(1));
    std::cout << "✅ Basic consensus test passed\n";
}

int main() {
    test_basic_consensus();
    std::cout << "All tests passed!\n";
    return 0;
}