// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <sys/time.h>
#include "include/lux_consensus.h"

#define BENCHMARK_ITERATIONS 100000
#define BATCH_SIZE 100
#define BLOCK_DATA_SIZE 1024

// High-precision timing
typedef struct {
    struct timespec start;
    struct timespec end;
} benchmark_timer_t;

static inline void timer_start(benchmark_timer_t* timer) {
    clock_gettime(CLOCK_MONOTONIC, &timer->start);
}

static inline void timer_end(benchmark_timer_t* timer) {
    clock_gettime(CLOCK_MONOTONIC, &timer->end);
}

static inline uint64_t timer_elapsed_ns(benchmark_timer_t* timer) {
    return (timer->end.tv_sec - timer->start.tv_sec) * 1000000000ULL +
           (timer->end.tv_nsec - timer->start.tv_nsec);
}

// Generate random block ID
static void generate_block_id(uint8_t* id) {
    for (int i = 0; i < 32; i++) {
        id[i] = rand() % 256;
    }
}

// Memory usage tracking
typedef struct {
    size_t current_usage;
    size_t peak_usage;
} memory_tracker_t;

static memory_tracker_t mem_tracker = {0, 0};

// Track memory allocations (simplified - would need to wrap malloc/free in production)
static void track_memory(size_t size) {
    mem_tracker.current_usage += size;
    if (mem_tracker.current_usage > mem_tracker.peak_usage) {
        mem_tracker.peak_usage = mem_tracker.current_usage;
    }
}

static void untrack_memory(size_t size) {
    mem_tracker.current_usage -= size;
}

// Benchmark result structure
typedef struct {
    const char* name;
    uint64_t total_ns;
    uint32_t iterations;
    uint64_t min_ns;
    uint64_t max_ns;
    double avg_ns;
    double ops_per_sec;
} benchmark_result_t;

static void print_result(const benchmark_result_t* result) {
    printf("%-40s: %12.2f ns/op | %12.0f ops/sec | min: %8llu ns | max: %8llu ns\n",
           result->name,
           result->avg_ns,
           result->ops_per_sec,
           result->min_ns,
           result->max_ns);
}

// Benchmark 1: Single block addition
static benchmark_result_t benchmark_single_block_add(lux_consensus_engine_t* engine) {
    benchmark_result_t result = {
        .name = "Single Block Addition",
        .iterations = BENCHMARK_ITERATIONS,
        .min_ns = UINT64_MAX,
        .max_ns = 0,
        .total_ns = 0
    };

    lux_block_t block;
    uint8_t parent_id[32] = {0};
    uint8_t data[BLOCK_DATA_SIZE];
    memset(data, 0xAA, BLOCK_DATA_SIZE);

    benchmark_timer_t timer;

    for (uint32_t i = 0; i < result.iterations; i++) {
        // Generate unique block
        generate_block_id(block.id);
        memcpy(block.parent_id, parent_id, 32);
        block.height = i;
        block.timestamp = time(NULL);
        block.data = data;
        block.data_size = BLOCK_DATA_SIZE;

        timer_start(&timer);
        lux_consensus_add_block(engine, &block);
        timer_end(&timer);

        uint64_t elapsed = timer_elapsed_ns(&timer);
        result.total_ns += elapsed;
        if (elapsed < result.min_ns) result.min_ns = elapsed;
        if (elapsed > result.max_ns) result.max_ns = elapsed;

        // Use this block as parent for next (create chain)
        memcpy(parent_id, block.id, 32);
    }

    result.avg_ns = (double)result.total_ns / result.iterations;
    result.ops_per_sec = 1000000000.0 / result.avg_ns;

    return result;
}

// Benchmark 2: Batch block addition
static benchmark_result_t benchmark_batch_block_add(lux_consensus_engine_t* engine) {
    benchmark_result_t result = {
        .name = "Batch Block Addition (100 blocks)",
        .iterations = BENCHMARK_ITERATIONS / BATCH_SIZE,
        .min_ns = UINT64_MAX,
        .max_ns = 0,
        .total_ns = 0
    };

    lux_block_t blocks[BATCH_SIZE];
    uint8_t parent_id[32] = {0};
    uint8_t data[BLOCK_DATA_SIZE];
    memset(data, 0xBB, BLOCK_DATA_SIZE);

    benchmark_timer_t timer;

    for (uint32_t batch = 0; batch < result.iterations; batch++) {
        // Prepare batch
        for (int i = 0; i < BATCH_SIZE; i++) {
            generate_block_id(blocks[i].id);
            memcpy(blocks[i].parent_id, parent_id, 32);
            blocks[i].height = batch * BATCH_SIZE + i;
            blocks[i].timestamp = time(NULL);
            blocks[i].data = data;
            blocks[i].data_size = BLOCK_DATA_SIZE;
        }

        timer_start(&timer);
        for (int i = 0; i < BATCH_SIZE; i++) {
            lux_consensus_add_block(engine, &blocks[i]);
        }
        timer_end(&timer);

        uint64_t elapsed = timer_elapsed_ns(&timer);
        result.total_ns += elapsed;
        if (elapsed < result.min_ns) result.min_ns = elapsed;
        if (elapsed > result.max_ns) result.max_ns = elapsed;
    }

    result.avg_ns = (double)result.total_ns / result.iterations;
    result.ops_per_sec = 1000000000.0 / result.avg_ns;

    return result;
}

// Benchmark 3: Single vote processing
static benchmark_result_t benchmark_single_vote(lux_consensus_engine_t* engine) {
    benchmark_result_t result = {
        .name = "Single Vote Processing",
        .iterations = BENCHMARK_ITERATIONS,
        .min_ns = UINT64_MAX,
        .max_ns = 0,
        .total_ns = 0
    };

    // First add a block to vote on
    lux_block_t block;
    generate_block_id(block.id);
    memset(block.parent_id, 0, 32);
    block.height = 1;
    block.timestamp = time(NULL);
    block.data = NULL;
    block.data_size = 0;
    lux_consensus_add_block(engine, &block);

    lux_vote_t vote;
    memcpy(vote.block_id, block.id, 32);
    vote.is_preference = true;

    benchmark_timer_t timer;

    for (uint32_t i = 0; i < result.iterations; i++) {
        // Generate unique voter ID
        generate_block_id(vote.voter_id);

        timer_start(&timer);
        lux_consensus_process_vote(engine, &vote);
        timer_end(&timer);

        uint64_t elapsed = timer_elapsed_ns(&timer);
        result.total_ns += elapsed;
        if (elapsed < result.min_ns) result.min_ns = elapsed;
        if (elapsed > result.max_ns) result.max_ns = elapsed;

        // Alternate between preference and confidence votes
        vote.is_preference = !vote.is_preference;
    }

    result.avg_ns = (double)result.total_ns / result.iterations;
    result.ops_per_sec = 1000000000.0 / result.avg_ns;

    return result;
}

// Benchmark 4: Batch vote processing
static benchmark_result_t benchmark_batch_vote(lux_consensus_engine_t* engine) {
    benchmark_result_t result = {
        .name = "Batch Vote Processing (100 votes)",
        .iterations = BENCHMARK_ITERATIONS / BATCH_SIZE,
        .min_ns = UINT64_MAX,
        .max_ns = 0,
        .total_ns = 0
    };

    // Add a block to vote on
    lux_block_t block;
    generate_block_id(block.id);
    memset(block.parent_id, 0, 32);
    block.height = 2;
    block.timestamp = time(NULL);
    block.data = NULL;
    block.data_size = 0;
    lux_consensus_add_block(engine, &block);

    lux_vote_t votes[BATCH_SIZE];
    for (int i = 0; i < BATCH_SIZE; i++) {
        memcpy(votes[i].block_id, block.id, 32);
        votes[i].is_preference = (i % 2 == 0);
    }

    benchmark_timer_t timer;

    for (uint32_t batch = 0; batch < result.iterations; batch++) {
        // Generate unique voter IDs for this batch
        for (int i = 0; i < BATCH_SIZE; i++) {
            generate_block_id(votes[i].voter_id);
        }

        timer_start(&timer);
        for (int i = 0; i < BATCH_SIZE; i++) {
            lux_consensus_process_vote(engine, &votes[i]);
        }
        timer_end(&timer);

        uint64_t elapsed = timer_elapsed_ns(&timer);
        result.total_ns += elapsed;
        if (elapsed < result.min_ns) result.min_ns = elapsed;
        if (elapsed > result.max_ns) result.max_ns = elapsed;
    }

    result.avg_ns = (double)result.total_ns / result.iterations;
    result.ops_per_sec = 1000000000.0 / result.avg_ns;

    return result;
}

// Benchmark 5: Finalization checking (is_accepted)
static benchmark_result_t benchmark_finalization_check(lux_consensus_engine_t* engine) {
    benchmark_result_t result = {
        .name = "Finalization Check (is_accepted)",
        .iterations = BENCHMARK_ITERATIONS,
        .min_ns = UINT64_MAX,
        .max_ns = 0,
        .total_ns = 0
    };

    // Add multiple blocks
    lux_block_t block;
    uint8_t block_ids[10][32];
    for (int i = 0; i < 10; i++) {
        generate_block_id(block.id);
        memcpy(block_ids[i], block.id, 32);
        memset(block.parent_id, 0, 32);
        block.height = i + 100;
        block.timestamp = time(NULL);
        block.data = NULL;
        block.data_size = 0;
        lux_consensus_add_block(engine, &block);
    }

    benchmark_timer_t timer;
    bool is_accepted;

    for (uint32_t i = 0; i < result.iterations; i++) {
        uint8_t* block_id = block_ids[i % 10];

        timer_start(&timer);
        lux_consensus_is_accepted(engine, block_id, &is_accepted);
        timer_end(&timer);

        uint64_t elapsed = timer_elapsed_ns(&timer);
        result.total_ns += elapsed;
        if (elapsed < result.min_ns) result.min_ns = elapsed;
        if (elapsed > result.max_ns) result.max_ns = elapsed;
    }

    result.avg_ns = (double)result.total_ns / result.iterations;
    result.ops_per_sec = 1000000000.0 / result.avg_ns;

    return result;
}

// Benchmark 6: Get preference
static benchmark_result_t benchmark_get_preference(lux_consensus_engine_t* engine) {
    benchmark_result_t result = {
        .name = "Get Preference",
        .iterations = BENCHMARK_ITERATIONS,
        .min_ns = UINT64_MAX,
        .max_ns = 0,
        .total_ns = 0
    };

    uint8_t block_id[32];
    benchmark_timer_t timer;

    for (uint32_t i = 0; i < result.iterations; i++) {
        timer_start(&timer);
        lux_consensus_get_preference(engine, block_id);
        timer_end(&timer);

        uint64_t elapsed = timer_elapsed_ns(&timer);
        result.total_ns += elapsed;
        if (elapsed < result.min_ns) result.min_ns = elapsed;
        if (elapsed > result.max_ns) result.max_ns = elapsed;
    }

    result.avg_ns = (double)result.total_ns / result.iterations;
    result.ops_per_sec = 1000000000.0 / result.avg_ns;

    return result;
}

// Benchmark 7: Poll operation
static benchmark_result_t benchmark_poll(lux_consensus_engine_t* engine) {
    benchmark_result_t result = {
        .name = "Poll Operation (10 validators)",
        .iterations = BENCHMARK_ITERATIONS / 100, // Less iterations for polling
        .min_ns = UINT64_MAX,
        .max_ns = 0,
        .total_ns = 0
    };

    // Create validator IDs
    #define NUM_VALIDATORS 10
    uint8_t validator_ids[NUM_VALIDATORS][32];
    const uint8_t* validator_ptrs[NUM_VALIDATORS];

    for (int i = 0; i < NUM_VALIDATORS; i++) {
        generate_block_id(validator_ids[i]);
        validator_ptrs[i] = validator_ids[i];
    }

    benchmark_timer_t timer;

    for (uint32_t i = 0; i < result.iterations; i++) {
        timer_start(&timer);
        lux_consensus_poll(engine, NUM_VALIDATORS, validator_ptrs);
        timer_end(&timer);

        uint64_t elapsed = timer_elapsed_ns(&timer);
        result.total_ns += elapsed;
        if (elapsed < result.min_ns) result.min_ns = elapsed;
        if (elapsed > result.max_ns) result.max_ns = elapsed;
    }

    result.avg_ns = (double)result.total_ns / result.iterations;
    result.ops_per_sec = 1000000000.0 / result.avg_ns;

    return result;
}

// Benchmark 8: Get statistics
static benchmark_result_t benchmark_get_stats(lux_consensus_engine_t* engine) {
    benchmark_result_t result = {
        .name = "Get Statistics",
        .iterations = BENCHMARK_ITERATIONS,
        .min_ns = UINT64_MAX,
        .max_ns = 0,
        .total_ns = 0
    };

    lux_consensus_stats_t stats;
    benchmark_timer_t timer;

    for (uint32_t i = 0; i < result.iterations; i++) {
        timer_start(&timer);
        lux_consensus_get_stats(engine, &stats);
        timer_end(&timer);

        uint64_t elapsed = timer_elapsed_ns(&timer);
        result.total_ns += elapsed;
        if (elapsed < result.min_ns) result.min_ns = elapsed;
        if (elapsed > result.max_ns) result.max_ns = elapsed;
    }

    result.avg_ns = (double)result.total_ns / result.iterations;
    result.ops_per_sec = 1000000000.0 / result.avg_ns;

    return result;
}

// Memory usage benchmark
static void benchmark_memory_usage(void) {
    printf("\n=== Memory Usage Benchmark ===\n");

    // Create multiple engines with different sizes
    const int engine_counts[] = {1, 10, 100};
    const int block_counts[] = {100, 1000, 10000};

    for (int e = 0; e < 3; e++) {
        for (int b = 0; b < 3; b++) {
            mem_tracker.current_usage = 0;
            mem_tracker.peak_usage = 0;

            // Track approximate memory (simplified)
            // Estimate engine size (struct + hash table + mutexes)
            size_t base_memory = (2048 + 1024 * sizeof(void*)) * engine_counts[e];
            size_t block_memory = (sizeof(lux_block_t) + 100) * block_counts[b]; // Assume 100 bytes overhead per block

            track_memory(base_memory);
            track_memory(block_memory);

            printf("Engines: %3d, Blocks: %5d => Estimated Memory: %8zu bytes (%.2f MB)\n",
                   engine_counts[e],
                   block_counts[b],
                   mem_tracker.peak_usage,
                   mem_tracker.peak_usage / (1024.0 * 1024.0));

            untrack_memory(block_memory);
            untrack_memory(base_memory);
        }
    }
}

// Stress test: Maximum throughput
static void benchmark_max_throughput(lux_consensus_engine_t* engine) {
    printf("\n=== Maximum Throughput Test (1 second) ===\n");

    lux_block_t block;
    lux_vote_t vote;
    uint8_t parent_id[32] = {0};

    uint32_t blocks_added = 0;
    uint32_t votes_processed = 0;

    struct timespec start, current;
    clock_gettime(CLOCK_MONOTONIC, &start);

    // Run for 1 second
    while (1) {
        clock_gettime(CLOCK_MONOTONIC, &current);
        if (current.tv_sec > start.tv_sec) break;

        // Add a block
        generate_block_id(block.id);
        memcpy(block.parent_id, parent_id, 32);
        block.height = blocks_added;
        block.timestamp = time(NULL);
        block.data = NULL;
        block.data_size = 0;

        if (lux_consensus_add_block(engine, &block) == LUX_SUCCESS) {
            blocks_added++;
            memcpy(parent_id, block.id, 32);

            // Process some votes for this block
            for (int v = 0; v < 10; v++) {
                generate_block_id(vote.voter_id);
                memcpy(vote.block_id, block.id, 32);
                vote.is_preference = (v % 2 == 0);

                if (lux_consensus_process_vote(engine, &vote) == LUX_SUCCESS) {
                    votes_processed++;
                }
            }
        }
    }

    printf("Blocks added:     %8u blocks/sec\n", blocks_added);
    printf("Votes processed:  %8u votes/sec\n", votes_processed);
    printf("Combined ops:     %8u ops/sec\n", blocks_added + votes_processed);
}

int main(void) {
    printf("========================================\n");
    printf("    Lux Consensus C Library Benchmarks\n");
    printf("========================================\n\n");

    // Seed random number generator
    srand(time(NULL));

    // Initialize consensus library
    if (lux_consensus_init() != LUX_SUCCESS) {
        fprintf(stderr, "Failed to initialize consensus library\n");
        return 1;
    }

    // Create consensus engine with default config
    lux_consensus_config_t config = {
        .k = 20,
        .alpha_preference = 14,
        .alpha_confidence = 14,
        .beta = 20,
        .concurrent_polls = 10,
        .optimal_processing = 50,
        .max_outstanding_items = 1024,
        .max_item_processing_time_ns = 1000000000, // 1 second
        .engine_type = LUX_ENGINE_CHAIN
    };

    lux_consensus_engine_t* engine = NULL;
    if (lux_consensus_engine_create(&engine, &config) != LUX_SUCCESS) {
        fprintf(stderr, "Failed to create consensus engine\n");
        lux_consensus_cleanup();
        return 1;
    }

    printf("Configuration:\n");
    printf("  Engine Type: %s\n", lux_engine_type_string(config.engine_type));
    printf("  k=%u, α_pref=%u, α_conf=%u, β=%u\n",
           config.k, config.alpha_preference, config.alpha_confidence, config.beta);
    printf("  Iterations: %d\n", BENCHMARK_ITERATIONS);
    printf("  Batch Size: %d\n\n", BATCH_SIZE);

    printf("=== Operation Benchmarks ===\n");

    // Run benchmarks
    benchmark_result_t results[] = {
        benchmark_single_block_add(engine),
        benchmark_batch_block_add(engine),
        benchmark_single_vote(engine),
        benchmark_batch_vote(engine),
        benchmark_finalization_check(engine),
        benchmark_get_preference(engine),
        benchmark_poll(engine),
        benchmark_get_stats(engine)
    };

    // Print results
    for (size_t i = 0; i < sizeof(results) / sizeof(results[0]); i++) {
        print_result(&results[i]);
    }

    // Get final statistics
    lux_consensus_stats_t stats;
    lux_consensus_get_stats(engine, &stats);

    printf("\n=== Final Statistics ===\n");
    printf("Blocks Accepted:  %llu\n", stats.blocks_accepted);
    printf("Blocks Rejected:  %llu\n", stats.blocks_rejected);
    printf("Votes Processed:  %llu\n", stats.votes_processed);
    printf("Polls Completed:  %llu\n", stats.polls_completed);
    printf("Avg Decision Time: %.2f ms\n", stats.average_decision_time_ms);

    // Memory usage benchmark
    benchmark_memory_usage();

    // Maximum throughput test
    benchmark_max_throughput(engine);

    // Cleanup
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();

    printf("\n========================================\n");
    printf("          Benchmark Complete\n");
    printf("========================================\n");

    return 0;
}