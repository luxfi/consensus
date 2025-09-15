# C++ MLX Implementation Documentation

## Overview

The C++ MLX implementation combines modern C++ features with MLX extensions for high-performance consensus. It provides object-oriented design, RAII patterns, and advanced template metaprogramming for type-safe, efficient consensus operations.

## Installation

### Prerequisites
- C++20 compatible compiler (GCC 11+, Clang 13+, MSVC 2022+)
- CMake 3.20+
- MLX Framework
- ZeroMQ 4.3+
- Boost 1.75+ (optional)

### Building with CMake

```bash
# Clone repository
git clone https://github.com/luxfi/consensus
cd consensus/cpp

# Configure with CMake
mkdir build && cd build
cmake .. -DCMAKE_BUILD_TYPE=Release -DENABLE_MLX=ON

# Build
make -j$(nproc)

# Install
sudo make install
```

### Using as Library

CMakeLists.txt:
```cmake
find_package(LuxConsensus REQUIRED)
target_link_libraries(your_app PRIVATE Lux::Consensus)
```

## Quick Start

```cpp
#include <lux/consensus.hpp>
#include <iostream>

int main() {
    using namespace lux::consensus;
    
    // Configure consensus parameters
    ConsensusParams params{
        .k = 20,
        .alpha_preference = 15,
        .alpha_confidence = 15,
        .beta = 20,
        .concurrent_polls = 10,
        .max_outstanding_items = 1000
    };
    
    // Create consensus engine with Snowball
    auto consensus = Consensus::create(EngineType::Snowball, params);
    
    // Create and add a block
    Block block{
        .id = 0x1234,
        .parent_id = 0x0000,
        .height = 1,
        .timestamp = std::chrono::system_clock::now(),
        .data = {0x01, 0x02, 0x03, 0x04}
    };
    
    consensus->add_block(block);
    
    // Process votes
    for (int i = 0; i < 20; ++i) {
        Vote vote{
            .engine_type = EngineType::Snowball,
            .node_id = static_cast<uint16_t>(i),
            .block_id = 0x1234,
            .vote_type = VoteType::Prefer
        };
        consensus->process_vote(vote);
    }
    
    // Check consensus
    if (consensus->is_accepted(0x1234)) {
        std::cout << "Block 0x" << std::hex << 0x1234 
                  << " achieved consensus!" << std::endl;
    }
    
    return 0;
}
```

## API Reference

### Core Classes

#### `Consensus` Class

```cpp
namespace lux::consensus {

class Consensus {
public:
    // Factory method
    static std::unique_ptr<Consensus> create(
        EngineType engine, 
        const ConsensusParams& params
    );
    
    // Core operations
    void add_block(const Block& block);
    void process_vote(const Vote& vote);
    bool is_accepted(uint16_t block_id) const;
    std::optional<uint16_t> get_preference() const;
    
    // Async operations
    std::future<void> add_block_async(Block block);
    std::future<void> process_vote_async(Vote vote);
    
    // Batch operations
    void process_votes_batch(std::span<const Vote> votes);
    
    // Statistics
    ConsensusStats get_stats() const;
    
    // Event handling
    template<typename Handler>
    void on_block_accepted(Handler&& handler);
};

}  // namespace lux::consensus
```

#### `ConsensusParams` Structure

```cpp
struct ConsensusParams {
    size_t k = 20;                          // Consecutive successes
    size_t alpha_preference = 15;           // Preference quorum
    size_t alpha_confidence = 15;           // Confidence quorum
    size_t beta = 20;                      // Confidence threshold
    size_t concurrent_polls = 10;          // Max concurrent polls
    size_t max_outstanding_items = 1000;   // Max outstanding items
    std::chrono::milliseconds timeout{30000}; // Processing timeout
    
    // Validation
    [[nodiscard]] bool validate() const noexcept;
};
```

#### `Block` Structure

```cpp
struct Block {
    uint16_t id;
    uint16_t parent_id;
    uint64_t height;
    std::chrono::system_clock::time_point timestamp;
    std::vector<uint8_t> data;
    
    // Serialization
    [[nodiscard]] std::vector<uint8_t> serialize() const;
    static Block deserialize(std::span<const uint8_t> data);
    
    // Hashing
    [[nodiscard]] std::array<uint8_t, 32> hash() const;
};
```

#### `Vote` Structure

```cpp
struct Vote {
    EngineType engine_type;
    uint16_t node_id;
    uint16_t block_id;
    VoteType vote_type;
    
    // Binary protocol (8 bytes)
    [[nodiscard]] std::array<uint8_t, 8> pack() const noexcept;
    static Vote unpack(std::span<const uint8_t, 8> data) noexcept;
};
```

### MLX Extensions

#### SIMD Operations

```cpp
#include <lux/consensus/mlx.hpp>

// MLX SIMD vote counting
class MLXConsensus : public Consensus {
public:
    void process_votes_simd(const mlx::array& votes) {
        // Use MLX for vectorized operations
        auto prefer_mask = mlx::equal(votes, VOTE_PREFER);
        auto prefer_count = mlx::sum(prefer_mask);
        
        // Update consensus state
        update_state_vectorized(prefer_count);
    }
    
private:
    void update_state_vectorized(const mlx::array& counts);
};
```

#### GPU Acceleration

```cpp
// GPU-accelerated consensus
class GPUConsensus : public Consensus {
public:
    GPUConsensus(const ConsensusParams& params) 
        : Consensus(params), 
          device_(mlx::gpu::device(0)) {}
    
    void process_votes_gpu(std::span<const Vote> votes) {
        // Transfer to GPU
        auto gpu_votes = mlx::array(votes.data(), {votes.size(), 8}, device_);
        
        // Process on GPU
        auto results = mlx::ops::consensus_kernel(gpu_votes);
        
        // Update state
        apply_results(results.to_host());
    }
    
private:
    mlx::Device device_;
};
```

### Template Metaprogramming

#### Compile-Time Engine Selection

```cpp
template<EngineType Engine>
class StaticConsensus {
public:
    static constexpr EngineType engine = Engine;
    
    template<typename Block>
    void add_block(Block&& block) {
        if constexpr (Engine == EngineType::Snowball) {
            snowball_add(std::forward<Block>(block));
        } else if constexpr (Engine == EngineType::Avalanche) {
            avalanche_add(std::forward<Block>(block));
        }
        // ... other engines
    }
    
private:
    void snowball_add(const Block& block);
    void avalanche_add(const Block& block);
};

// Usage - engine type known at compile time
StaticConsensus<EngineType::Snowball> consensus;
```

#### Concept-Based Constraints

```cpp
template<typename T>
concept Votable = requires(T t) {
    { t.id() } -> std::convertible_to<uint16_t>;
    { t.vote_type() } -> std::same_as<VoteType>;
};

template<typename T>
concept Blockable = requires(T t) {
    { t.id() } -> std::convertible_to<uint16_t>;
    { t.parent_id() } -> std::convertible_to<uint16_t>;
    { t.height() } -> std::convertible_to<uint64_t>;
    { t.serialize() } -> std::convertible_to<std::vector<uint8_t>>;
};

// Constrained templates
template<Blockable B>
void process_block(B&& block) {
    // Type-safe block processing
}
```

### Network Integration

#### ZeroMQ Network

```cpp
#include <lux/consensus/network.hpp>

class NetworkNode {
public:
    NetworkNode(const std::string& bind_addr) 
        : context_(1), 
          publisher_(context_, zmq::socket_type::pub),
          subscriber_(context_, zmq::socket_type::sub) {
        
        publisher_.bind(bind_addr);
        subscriber_.connect("tcp://peer1:5555");
        subscriber_.set(zmq::sockopt::subscribe, "");
    }
    
    void broadcast(const Vote& vote) {
        auto packed = vote.pack();
        zmq::message_t msg(packed.data(), packed.size());
        publisher_.send(msg, zmq::send_flags::none);
    }
    
    std::optional<Vote> receive() {
        zmq::message_t msg;
        if (subscriber_.recv(msg, zmq::recv_flags::dontwait)) {
            return Vote::unpack({
                static_cast<uint8_t*>(msg.data()), 
                msg.size()
            });
        }
        return std::nullopt;
    }
    
private:
    zmq::context_t context_;
    zmq::socket_t publisher_;
    zmq::socket_t subscriber_;
};
```

#### Async Networking

```cpp
#include <boost/asio.hpp>

class AsyncConsensusNode {
public:
    AsyncConsensusNode(boost::asio::io_context& io)
        : io_(io), 
          acceptor_(io, tcp::endpoint(tcp::v4(), 9650)),
          consensus_(Consensus::create(EngineType::Avalanche, {})) {
        
        start_accept();
    }
    
private:
    void start_accept() {
        auto socket = std::make_shared<tcp::socket>(io_);
        
        acceptor_.async_accept(*socket,
            [this, socket](const boost::system::error_code& error) {
                if (!error) {
                    handle_connection(socket);
                }
                start_accept();
            });
    }
    
    void handle_connection(std::shared_ptr<tcp::socket> socket) {
        auto buffer = std::make_shared<std::array<uint8_t, 8>>();
        
        boost::asio::async_read(*socket, boost::asio::buffer(*buffer),
            [this, buffer](const boost::system::error_code& error, size_t) {
                if (!error) {
                    auto vote = Vote::unpack(*buffer);
                    consensus_->process_vote(vote);
                }
            });
    }
    
    boost::asio::io_context& io_;
    tcp::acceptor acceptor_;
    std::unique_ptr<Consensus> consensus_;
};
```

### Advanced Features

#### Coroutines (C++20)

```cpp
#include <coroutine>

class CoroutineConsensus {
public:
    struct VoteAwaitable {
        bool await_ready() const noexcept { return false; }
        void await_suspend(std::coroutine_handle<> h) { handle = h; }
        Vote await_resume() { return vote; }
        
        std::coroutine_handle<> handle;
        Vote vote;
    };
    
    task<void> process_votes() {
        while (true) {
            Vote vote = co_await receive_vote();
            consensus_->process_vote(vote);
            
            if (consensus_->is_accepted(vote.block_id)) {
                co_await notify_accepted(vote.block_id);
            }
        }
    }
    
private:
    VoteAwaitable receive_vote();
    task<void> notify_accepted(uint16_t block_id);
};
```

#### Memory Management

```cpp
// Custom allocator for vote processing
template<typename T>
class ConsensusAllocator {
public:
    using value_type = T;
    
    ConsensusAllocator() : pool_(1024 * 1024) {}  // 1MB pool
    
    T* allocate(size_t n) {
        return static_cast<T*>(pool_.allocate(n * sizeof(T)));
    }
    
    void deallocate(T* p, size_t n) {
        pool_.deallocate(p, n * sizeof(T));
    }
    
private:
    MemoryPool pool_;
};

// Usage
using VoteVector = std::vector<Vote, ConsensusAllocator<Vote>>;
VoteVector votes(allocator);
```

## Testing

### Unit Tests (Google Test)

```cpp
#include <gtest/gtest.h>
#include <lux/consensus.hpp>

class ConsensusTest : public ::testing::Test {
protected:
    void SetUp() override {
        params_.k = 20;
        params_.alpha_preference = 15;
        consensus_ = Consensus::create(EngineType::Snowball, params_);
    }
    
    ConsensusParams params_;
    std::unique_ptr<Consensus> consensus_;
};

TEST_F(ConsensusTest, AcceptsBlockAfterQuorum) {
    Block block{.id = 1, .parent_id = 0, .height = 1};
    consensus_->add_block(block);
    
    for (int i = 0; i < 20; ++i) {
        Vote vote{
            .engine_type = EngineType::Snowball,
            .node_id = static_cast<uint16_t>(i),
            .block_id = 1,
            .vote_type = VoteType::Prefer
        };
        consensus_->process_vote(vote);
    }
    
    EXPECT_TRUE(consensus_->is_accepted(1));
}
```

### Benchmarks (Google Benchmark)

```cpp
#include <benchmark/benchmark.h>

static void BM_VoteProcessing(benchmark::State& state) {
    auto consensus = Consensus::create(EngineType::Snowball, {});
    Vote vote{.engine_type = EngineType::Snowball, .node_id = 1, 
              .block_id = 1, .vote_type = VoteType::Prefer};
    
    for (auto _ : state) {
        consensus->process_vote(vote);
    }
    
    state.SetItemsProcessed(state.iterations());
}
BENCHMARK(BM_VoteProcessing);

static void BM_MLXVoteProcessing(benchmark::State& state) {
    MLXConsensus consensus({});
    auto votes = mlx::random::uniform({1000, 8});
    
    for (auto _ : state) {
        consensus.process_votes_simd(votes);
    }
    
    state.SetItemsProcessed(state.iterations() * 1000);
}
BENCHMARK(BM_MLXVoteProcessing);
```

### Fuzzing

```cpp
extern "C" int LLVMFuzzerTestOneInput(const uint8_t* data, size_t size) {
    if (size < 8) return 0;
    
    auto consensus = Consensus::create(EngineType::Snowball, {});
    
    // Process as vote
    std::array<uint8_t, 8> vote_data;
    std::copy_n(data, 8, vote_data.begin());
    
    try {
        auto vote = Vote::unpack(vote_data);
        consensus->process_vote(vote);
    } catch (...) {
        // Ignore exceptions in fuzzing
    }
    
    return 0;
}
```

## Performance Optimization

### Lock-Free Data Structures

```cpp
#include <atomic>

template<typename T, size_t Size>
class LockFreeRingBuffer {
public:
    bool push(T value) {
        auto current_write = write_index_.load();
        auto next_write = (current_write + 1) % Size;
        
        if (next_write == read_index_.load()) {
            return false;  // Buffer full
        }
        
        buffer_[current_write] = std::move(value);
        write_index_.store(next_write);
        return true;
    }
    
    std::optional<T> pop() {
        auto current_read = read_index_.load();
        
        if (current_read == write_index_.load()) {
            return std::nullopt;  // Buffer empty
        }
        
        T value = std::move(buffer_[current_read]);
        read_index_.store((current_read + 1) % Size);
        return value;
    }
    
private:
    std::array<T, Size> buffer_;
    std::atomic<size_t> write_index_{0};
    std::atomic<size_t> read_index_{0};
};

// Use in consensus
LockFreeRingBuffer<Vote, 10000> vote_queue;
```

### Cache Optimization

```cpp
// Cache-aligned structures
struct alignas(64) CacheAlignedVote {
    Vote vote;
    char padding[64 - sizeof(Vote)];
};

// Prefetching
void process_votes_prefetch(std::span<const Vote> votes) {
    constexpr size_t prefetch_distance = 4;
    
    for (size_t i = 0; i < votes.size(); ++i) {
        if (i + prefetch_distance < votes.size()) {
            __builtin_prefetch(&votes[i + prefetch_distance], 0, 1);
        }
        consensus_->process_vote(votes[i]);
    }
}
```

## Examples

Complete examples in [`examples/`](../../examples/cpp/):
- `basic_consensus.cpp` - Basic usage
- `mlx_consensus.cpp` - MLX extensions
- `network_node.cpp` - Network implementation
- `async_consensus.cpp` - Async operations
- `benchmark.cpp` - Performance testing

## Best Practices

1. **RAII**: Always use RAII for resource management
2. **Move Semantics**: Prefer move over copy for large objects
3. **Const Correctness**: Mark const wherever possible
4. **noexcept**: Mark functions noexcept when they don't throw
5. **Concepts**: Use concepts for template constraints

## Troubleshooting

### Common Issues

1. **Compilation errors**
   ```bash
   # Check C++ standard
   cmake .. -DCMAKE_CXX_STANDARD=20
   
   # Enable verbose output
   make VERBOSE=1
   ```

2. **Linker errors**
   ```bash
   # Check library paths
   export LD_LIBRARY_PATH=/usr/local/lib:$LD_LIBRARY_PATH
   
   # Use ldd to check dependencies
   ldd your_binary
   ```

3. **Performance issues**
   - Enable optimizations: `-O3 -march=native`
   - Use PGO (Profile-Guided Optimization)
   - Enable LTO (Link-Time Optimization)

4. **Memory issues**
   - Use AddressSanitizer: `-fsanitize=address`
   - Use Valgrind for leak detection
   - Enable STL debug mode

## License

Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.