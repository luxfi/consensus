# C Implementation Documentation

## Overview

The C implementation provides the highest performance consensus engine with minimal overhead. It's designed for systems programming, embedded devices, and performance-critical applications.

## Installation

### Prerequisites
- GCC 9+ or Clang 10+
- ZeroMQ 4.3+
- CMake 3.15+ (optional)
- pkg-config

### Building from Source

```bash
# Install dependencies (Ubuntu/Debian)
sudo apt-get install libzmq3-dev pkg-config build-essential

# Install dependencies (macOS)
brew install zeromq pkg-config

# Build the library
cd consensus
make build-c

# Install system-wide (optional)
sudo make install-c
```

## Quick Start

```c
#include <consensus.h>
#include <stdio.h>

int main() {
    // Initialize consensus with Snowball engine
    consensus_t* consensus = consensus_new(SNOWBALL);
    if (!consensus) {
        fprintf(stderr, "Failed to create consensus\n");
        return 1;
    }
    
    // Configure parameters
    consensus_params_t params = {
        .k = 20,
        .alpha_preference = 15,
        .alpha_confidence = 15,
        .beta = 20,
        .concurrent_polls = 10
    };
    consensus_configure(consensus, &params);
    
    // Create and add a block
    block_t block = {
        .id = 0x1234,
        .parent_id = 0x0000,
        .height = 1,
        .timestamp = time(NULL),
        .data = "Hello, Consensus!",
        .data_len = 17
    };
    consensus_add_block(consensus, &block);
    
    // Process votes
    vote_msg_t vote = {
        .engine_type = SNOWBALL,
        .node_id = 0x0001,
        .block_id = 0x1234,
        .vote_type = VOTE_PREFER
    };
    consensus_process_vote(consensus, &vote);
    
    // Check consensus status
    if (consensus_is_accepted(consensus, 0x1234)) {
        printf("Block 0x1234 achieved consensus!\n");
    }
    
    // Cleanup
    consensus_free(consensus);
    return 0;
}
```

## API Reference

### Core Functions

#### `consensus_new`
```c
consensus_t* consensus_new(engine_type_t engine);
```
Creates a new consensus instance with the specified engine type.

**Parameters:**
- `engine`: Engine type (SNOWBALL, AVALANCHE, SNOWFLAKE, DAG, CHAIN, POSTQUANTUM)

**Returns:** Pointer to consensus instance or NULL on failure

#### `consensus_configure`
```c
int consensus_configure(consensus_t* c, const consensus_params_t* params);
```
Configures consensus parameters.

**Parameters:**
- `c`: Consensus instance
- `params`: Configuration parameters

**Returns:** 0 on success, -1 on failure

#### `consensus_add_block`
```c
int consensus_add_block(consensus_t* c, const block_t* block);
```
Adds a block to the consensus engine.

**Parameters:**
- `c`: Consensus instance
- `block`: Block to add

**Returns:** 0 on success, -1 on failure

#### `consensus_process_vote`
```c
int consensus_process_vote(consensus_t* c, const vote_msg_t* vote);
```
Processes an incoming vote.

**Parameters:**
- `c`: Consensus instance
- `vote`: Vote message to process

**Returns:** 0 on success, -1 on failure

#### `consensus_is_accepted`
```c
bool consensus_is_accepted(consensus_t* c, uint16_t block_id);
```
Checks if a block has been accepted by consensus.

**Parameters:**
- `c`: Consensus instance
- `block_id`: Block ID to check

**Returns:** true if accepted, false otherwise

### Network Functions

#### `network_new`
```c
network_t* network_new(const char* endpoint);
```
Creates a new network instance.

**Parameters:**
- `endpoint`: ZeroMQ endpoint (e.g., "tcp://0.0.0.0:5555")

**Returns:** Network instance or NULL on failure

#### `network_broadcast`
```c
int network_broadcast(network_t* net, const void* data, size_t len);
```
Broadcasts data to all connected nodes.

**Parameters:**
- `net`: Network instance
- `data`: Data to broadcast
- `len`: Length of data

**Returns:** Number of bytes sent or -1 on failure

#### `network_recv`
```c
int network_recv(network_t* net, void* buffer, size_t len);
```
Receives data from the network.

**Parameters:**
- `net`: Network instance
- `buffer`: Buffer to store received data
- `len`: Maximum length to receive

**Returns:** Number of bytes received or -1 on failure

## Data Structures

### consensus_params_t
```c
typedef struct {
    int k;                    // Consecutive successes needed
    int alpha_preference;     // Quorum size for preference
    int alpha_confidence;     // Quorum size for confidence
    int beta;                // Confidence threshold
    int concurrent_polls;    // Max concurrent polls
    int max_outstanding;     // Max outstanding items
} consensus_params_t;
```

### block_t
```c
typedef struct {
    uint16_t id;             // Block ID
    uint16_t parent_id;      // Parent block ID
    uint64_t height;         // Block height
    time_t timestamp;        // Unix timestamp
    void* data;              // Block data
    size_t data_len;         // Data length
} block_t;
```

### vote_msg_t
```c
typedef struct {
    uint8_t engine_type;     // Consensus engine type
    uint16_t node_id;        // Voting node ID
    uint16_t block_id;       // Block being voted on
    uint8_t vote_type;       // Vote type (PREFER/ACCEPT/REJECT)
    uint16_t reserved;       // Reserved for future use
} vote_msg_t;
```

## Advanced Usage

### Custom Vote Handler
```c
void custom_vote_handler(consensus_t* c, const vote_msg_t* vote, void* userdata) {
    printf("Received vote from node %u for block %u\n", 
           vote->node_id, vote->block_id);
    
    // Custom logic here
    MyState* state = (MyState*)userdata;
    state->vote_count++;
}

// Register handler
consensus_set_vote_handler(consensus, custom_vote_handler, &my_state);
```

### Byzantine Fault Tolerance
```c
// Enable Byzantine fault detection
consensus_enable_byzantine_detection(consensus, true);

// Set Byzantine threshold (f < n/3)
consensus_set_byzantine_threshold(consensus, num_nodes / 3 - 1);

// Check for Byzantine behavior
if (consensus_detect_byzantine(consensus, node_id)) {
    printf("Node %u exhibiting Byzantine behavior\n", node_id);
    consensus_blacklist_node(consensus, node_id);
}
```

### Performance Optimization
```c
// Use batch processing for votes
vote_msg_t votes[100];
int count = network_recv_batch(net, votes, 100);
consensus_process_votes_batch(consensus, votes, count);

// Enable vote caching
consensus_enable_cache(consensus, true);
consensus_set_cache_size(consensus, 10000);

// Use memory pools for zero-copy
memory_pool_t* pool = memory_pool_new(1024 * 1024);
consensus_set_memory_pool(consensus, pool);
```

## Benchmarking

```c
#include <consensus_bench.h>

// Run built-in benchmarks
benchmark_results_t results;
consensus_benchmark(consensus, &results);

printf("Throughput: %lu votes/sec\n", results.votes_per_second);
printf("Latency: %.2f ms\n", results.avg_latency_ms);
printf("Memory: %lu KB\n", results.memory_usage_kb);
```

## Error Handling

```c
// Check for errors
if (consensus_has_error(consensus)) {
    const char* error = consensus_get_error(consensus);
    fprintf(stderr, "Consensus error: %s\n", error);
    consensus_clear_error(consensus);
}

// Set custom error handler
void error_handler(const char* error, void* userdata) {
    log_error("Consensus error: %s", error);
}
consensus_set_error_handler(consensus, error_handler, NULL);
```

## Thread Safety

The C implementation is thread-safe with proper locking:

```c
// Multi-threaded usage
#pragma omp parallel for
for (int i = 0; i < num_votes; i++) {
    consensus_process_vote(consensus, &votes[i]);
}

// Or use explicit locking
consensus_lock(consensus);
// Critical section
consensus_unlock(consensus);
```

## Memory Management

```c
// Get memory statistics
memory_stats_t stats;
consensus_get_memory_stats(consensus, &stats);
printf("Allocated: %lu bytes\n", stats.allocated);
printf("Peak: %lu bytes\n", stats.peak);

// Set memory limits
consensus_set_memory_limit(consensus, 100 * 1024 * 1024); // 100MB

// Custom allocator
consensus_set_allocator(consensus, my_malloc, my_free, my_realloc);
```

## Debugging

```c
// Enable debug logging
consensus_set_log_level(consensus, LOG_DEBUG);

// Dump internal state
consensus_dump_state(consensus, "consensus_state.txt");

// Validate integrity
if (!consensus_validate(consensus)) {
    fprintf(stderr, "Consensus state corrupted!\n");
}
```

## Examples

See the [`examples/`](../../examples/c/) directory for complete examples:
- `simple_consensus.c` - Basic consensus usage
- `network_consensus.c` - Network integration
- `byzantine_test.c` - Byzantine fault tolerance
- `benchmark.c` - Performance benchmarking
- `multi_engine.c` - Using multiple engines

## Troubleshooting

### Common Issues

1. **Compilation errors with ZeroMQ**
   ```bash
   export PKG_CONFIG_PATH=/usr/local/lib/pkgconfig:$PKG_CONFIG_PATH
   ```

2. **Segmentation faults**
   - Check that consensus is initialized before use
   - Verify block IDs are unique
   - Ensure proper cleanup with `consensus_free()`

3. **Poor performance**
   - Enable vote batching
   - Use memory pools
   - Check network latency
   - Profile with `perf` or `valgrind`

4. **Memory leaks**
   ```bash
   valgrind --leak-check=full ./your_program
   ```

## Performance Tips

1. **Batch Operations**: Process votes in batches of 100-1000
2. **Memory Pools**: Use pre-allocated memory pools
3. **CPU Affinity**: Pin threads to specific cores
4. **NUMA Awareness**: Allocate memory on local NUMA nodes
5. **Compiler Optimization**: Use `-O3 -march=native`

## License

Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.