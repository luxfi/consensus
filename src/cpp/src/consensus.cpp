#include <lux/consensus.hpp>
#include <unordered_map>
#include <mutex>
#include <atomic>
#include <cstring>

namespace lux::consensus {

// Parameter validation
bool ConsensusParams::validate() const noexcept {
    if (alpha_preference > k) return false;
    if (alpha_confidence > k) return false;
    if (beta < 1) return false;
    if (concurrent_polls < 1) return false;
    if (max_outstanding_items < 1) return false;
    return true;
}

// Block serialization
std::vector<uint8_t> Block::serialize() const {
    std::vector<uint8_t> result;
    result.reserve(sizeof(id) + sizeof(parent_id) + sizeof(height) + data.size());
    
    // Add fields
    const uint8_t* ptr = reinterpret_cast<const uint8_t*>(&id);
    result.insert(result.end(), ptr, ptr + sizeof(id));
    
    ptr = reinterpret_cast<const uint8_t*>(&parent_id);
    result.insert(result.end(), ptr, ptr + sizeof(parent_id));
    
    ptr = reinterpret_cast<const uint8_t*>(&height);
    result.insert(result.end(), ptr, ptr + sizeof(height));
    
    result.insert(result.end(), data.begin(), data.end());
    
    return result;
}

// Block hash (simplified)
std::array<uint8_t, 32> Block::hash() const {
    std::array<uint8_t, 32> result{};
    auto serialized = serialize();
    
    // Simple hash (should use proper crypto hash in production)
    for (size_t i = 0; i < serialized.size(); ++i) {
        result[i % 32] ^= serialized[i];
    }
    
    return result;
}

// Vote packing
std::array<uint8_t, 8> Vote::pack() const noexcept {
    std::array<uint8_t, 8> result{};
    
    result[0] = static_cast<uint8_t>(engine_type);
    result[1] = static_cast<uint8_t>(node_id >> 8);
    result[2] = static_cast<uint8_t>(node_id & 0xFF);
    result[3] = static_cast<uint8_t>(block_id >> 8);
    result[4] = static_cast<uint8_t>(block_id & 0xFF);
    result[5] = static_cast<uint8_t>(vote_type);
    result[6] = 0; // Reserved
    result[7] = 0; // Reserved
    
    return result;
}

// Vote unpacking
Vote Vote::unpack(std::span<const uint8_t, 8> data) noexcept {
    Vote vote;
    vote.engine_type = static_cast<EngineType>(data[0]);
    vote.node_id = (static_cast<uint16_t>(data[1]) << 8) | data[2];
    vote.block_id = (static_cast<uint16_t>(data[3]) << 8) | data[4];
    vote.vote_type = static_cast<VoteType>(data[5]);
    return vote;
}

// Base consensus implementation
class ConsensusImpl : public Consensus {
protected:
    ConsensusParams params_;
    mutable std::mutex mutex_;
    std::unordered_map<uint16_t, Block> blocks_;
    std::unordered_map<uint16_t, BlockStatus> block_status_;
    std::atomic<uint16_t> preference_{0};
    std::atomic<uint64_t> votes_processed_{0};
    std::atomic<uint64_t> blocks_accepted_{0};
    std::atomic<uint64_t> blocks_rejected_{0};
    BlockAcceptedHandler accepted_handler_;
    
public:
    explicit ConsensusImpl(const ConsensusParams& params)
        : params_(params) {}
    
    void add_block(const Block& block) override {
        std::lock_guard lock(mutex_);
        blocks_[block.id] = block;
        block_status_[block.id] = BlockStatus::Processing;
    }
    
    void process_vote(const Vote& vote) override {
        votes_processed_.fetch_add(1, std::memory_order_relaxed);
        
        // Process based on vote type
        if (vote.vote_type == VoteType::Prefer) {
            preference_.store(vote.block_id, std::memory_order_relaxed);
        } else if (vote.vote_type == VoteType::Accept) {
            std::lock_guard lock(mutex_);
            if (block_status_[vote.block_id] == BlockStatus::Processing) {
                block_status_[vote.block_id] = BlockStatus::Accepted;
                blocks_accepted_.fetch_add(1, std::memory_order_relaxed);
                
                if (accepted_handler_) {
                    accepted_handler_(vote.block_id);
                }
            }
        } else if (vote.vote_type == VoteType::Reject) {
            std::lock_guard lock(mutex_);
            if (block_status_[vote.block_id] == BlockStatus::Processing) {
                block_status_[vote.block_id] = BlockStatus::Rejected;
                blocks_rejected_.fetch_add(1, std::memory_order_relaxed);
            }
        }
    }
    
    bool is_accepted(uint16_t block_id) const override {
        std::lock_guard lock(mutex_);
        auto it = block_status_.find(block_id);
        return it != block_status_.end() && it->second == BlockStatus::Accepted;
    }
    
    std::optional<uint16_t> get_preference() const override {
        auto pref = preference_.load(std::memory_order_relaxed);
        return pref > 0 ? std::optional<uint16_t>(pref) : std::nullopt;
    }
    
    void process_votes_batch(std::span<const Vote> votes) override {
        for (const auto& vote : votes) {
            process_vote(vote);
        }
    }
    
    ConsensusStats get_stats() const override {
        return {
            .votes_processed = votes_processed_.load(std::memory_order_relaxed),
            .blocks_accepted = blocks_accepted_.load(std::memory_order_relaxed),
            .blocks_rejected = blocks_rejected_.load(std::memory_order_relaxed),
            .avg_latency = std::chrono::milliseconds(10), // Mock value
            .memory_usage_bytes = blocks_.size() * sizeof(Block)
        };
    }
    
    void on_block_accepted(BlockAcceptedHandler handler) override {
        accepted_handler_ = std::move(handler);
    }
    
    bool health_check() const override {
        return true;
    }
};

// Snowball engine implementation
class SnowballConsensus : public ConsensusImpl {
private:
    std::unordered_map<uint16_t, size_t> confidence_;
    std::unordered_map<uint16_t, size_t> consecutive_successes_;
    
public:
    explicit SnowballConsensus(const ConsensusParams& params)
        : ConsensusImpl(params) {}
    
    void process_vote(const Vote& vote) override {
        ConsensusImpl::process_vote(vote);
        
        if (vote.vote_type == VoteType::Prefer) {
            std::lock_guard lock(mutex_);
            
            // Increment confidence
            confidence_[vote.block_id]++;
            
            // Check for consecutive successes
            if (confidence_[vote.block_id] >= params_.alpha_preference) {
                consecutive_successes_[vote.block_id]++;
                
                // Check for acceptance
                if (consecutive_successes_[vote.block_id] >= params_.k) {
                    if (block_status_[vote.block_id] == BlockStatus::Processing) {
                        block_status_[vote.block_id] = BlockStatus::Accepted;
                        blocks_accepted_.fetch_add(1, std::memory_order_relaxed);
                        
                        if (accepted_handler_) {
                            accepted_handler_(vote.block_id);
                        }
                    }
                }
            }
        }
    }
};

// Factory method implementation
std::unique_ptr<Consensus> Consensus::create(
    EngineType engine,
    const ConsensusParams& params) {
    
    if (!params.validate()) {
        return nullptr;
    }
    
    switch (engine) {
        case EngineType::Snowball:
            return std::make_unique<SnowballConsensus>(params);
        case EngineType::Avalanche:
            // TODO: Implement Avalanche
            return std::make_unique<ConsensusImpl>(params);
        case EngineType::Snowflake:
            // TODO: Implement Snowflake
            return std::make_unique<ConsensusImpl>(params);
        case EngineType::DAG:
            // TODO: Implement DAG
            return std::make_unique<ConsensusImpl>(params);
        case EngineType::Chain:
            // TODO: Implement Chain
            return std::make_unique<ConsensusImpl>(params);
        case EngineType::PostQuantum:
            // TODO: Implement PostQuantum
            return std::make_unique<ConsensusImpl>(params);
        default:
            return nullptr;
    }
}

} // namespace lux::consensus