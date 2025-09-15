#pragma once

#include <cstdint>
#include <memory>
#include <vector>
#include <optional>
#include <chrono>
#include <functional>
#include <span>
#include <array>

namespace lux::consensus {

// Forward declarations
class ConsensusImpl;

// Engine types
enum class EngineType : uint8_t {
    Snowball = 0,
    Avalanche = 1,
    Snowflake = 2,
    DAG = 3,
    Chain = 4,
    PostQuantum = 5
};

// Vote types
enum class VoteType : uint8_t {
    Prefer = 1,
    Accept = 2,
    Reject = 3
};

// Block status
enum class BlockStatus : uint8_t {
    Unknown = 0,
    Processing = 1,
    Accepted = 2,
    Rejected = 3
};

// Consensus parameters
struct ConsensusParams {
    size_t k = 20;                          // Consecutive successes
    size_t alpha_preference = 15;           // Preference quorum
    size_t alpha_confidence = 15;           // Confidence quorum
    size_t beta = 20;                      // Confidence threshold
    size_t concurrent_polls = 10;          // Max concurrent polls
    size_t max_outstanding_items = 1000;   // Max outstanding items
    std::chrono::milliseconds timeout{30000}; // Processing timeout
    
    [[nodiscard]] bool validate() const noexcept;
};

// Block structure
struct Block {
    uint16_t id;
    uint16_t parent_id;
    uint64_t height;
    std::chrono::system_clock::time_point timestamp;
    std::vector<uint8_t> data;
    
    [[nodiscard]] std::vector<uint8_t> serialize() const;
    [[nodiscard]] std::array<uint8_t, 32> hash() const;
    
    static Block deserialize(std::span<const uint8_t> data);
};

// Vote structure
struct Vote {
    EngineType engine_type;
    uint16_t node_id;
    uint16_t block_id;
    VoteType vote_type;
    
    // Binary protocol (8 bytes)
    [[nodiscard]] std::array<uint8_t, 8> pack() const noexcept;
    static Vote unpack(std::span<const uint8_t, 8> data) noexcept;
};

// Consensus statistics
struct ConsensusStats {
    uint64_t votes_processed = 0;
    uint64_t blocks_accepted = 0;
    uint64_t blocks_rejected = 0;
    std::chrono::milliseconds avg_latency{0};
    size_t memory_usage_bytes = 0;
};

// Main consensus class
class Consensus {
public:
    // Factory method
    static std::unique_ptr<Consensus> create(
        EngineType engine,
        const ConsensusParams& params
    );
    
    // Destructor
    virtual ~Consensus() = default;
    
    // Core operations
    virtual void add_block(const Block& block) = 0;
    virtual void process_vote(const Vote& vote) = 0;
    virtual bool is_accepted(uint16_t block_id) const = 0;
    virtual std::optional<uint16_t> get_preference() const = 0;
    
    // Batch operations
    virtual void process_votes_batch(std::span<const Vote> votes) = 0;
    
    // Statistics
    virtual ConsensusStats get_stats() const = 0;
    
    // Event handling
    using BlockAcceptedHandler = std::function<void(uint16_t)>;
    virtual void on_block_accepted(BlockAcceptedHandler handler) = 0;
    
    // Health check
    virtual bool health_check() const = 0;
};

// Engine trait for custom implementations
class Engine {
public:
    virtual ~Engine() = default;
    
    virtual void process_vote(const Vote& vote) = 0;
    virtual bool is_accepted(uint16_t block_id) const = 0;
    virtual std::optional<uint16_t> get_preference() const = 0;
    virtual std::vector<Vote> poll(uint16_t block_id) = 0;
};

} // namespace lux::consensus