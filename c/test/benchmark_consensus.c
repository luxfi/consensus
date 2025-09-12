// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <sys/time.h>
#include <pthread.h>
#include "../include/lux_consensus.h"

// Benchmark configuration
#define WARMUP_ITERATIONS 100
#define BENCHMARK_ITERATIONS 1000
#define NUM_BLOCKS_SMALL 100
#define NUM_BLOCKS_MEDIUM 1000
#define NUM_BLOCKS_LARGE 10000
#define NUM_VOTES_SMALL 1000
#define NUM_VOTES_MEDIUM 10000
#define NUM_VOTES_LARGE 100000

// Timing utilities
double get_time_ms() {
    struct timeval tv;
    gettimeofday(&tv, NULL);
    return (tv.tv_sec * 1000.0) + (tv.tv_usec / 1000.0);
}

typedef struct {
    double min;
    double max;
    double avg;
    double median;
    double p95;
    double p99;
    double total;
    int count;
} benchmark_stats_t;

void calculate_stats(double* times, int count, benchmark_stats_t* stats) {
    if (count == 0) return;
    
    // Sort times for percentiles
    for (int i = 0; i < count - 1; i++) {
        for (int j = 0; j < count - i - 1; j++) {
            if (times[j] > times[j + 1]) {
                double temp = times[j];
                times[j] = times[j + 1];
                times[j + 1] = temp;
            }
        }
    }
    
    stats->min = times[0];
    stats->max = times[count - 1];
    stats->median = times[count / 2];
    stats->p95 = times[(int)(count * 0.95)];
    stats->p99 = times[(int)(count * 0.99)];
    
    stats->total = 0;
    for (int i = 0; i < count; i++) {
        stats->total += times[i];
    }
    stats->avg = stats->total / count;
    stats->count = count;
}

void print_benchmark_result(const char* name, benchmark_stats_t* stats, const char* unit) {
    printf("%-30s: avg=%.3f%s min=%.3f%s max=%.3f%s p95=%.3f%s p99=%.3f%s\n",
           name, stats->avg, unit, stats->min, unit, stats->max, unit,
           stats->p95, unit, stats->p99, unit);
}

// Benchmark: Engine creation
void benchmark_engine_creation() {
    printf("\n=== BENCHMARK: Engine Creation ===\n");
    
    double times[BENCHMARK_ITERATIONS];
    lux_consensus_init();
    
    lux_consensus_config_t config = {
        .k = 20,
        .alpha_preference = 15,
        .alpha_confidence = 15,
        .beta = 20,
        .concurrent_polls = 1,
        .optimal_processing = 1,
        .max_outstanding_items = 1024,
        .max_item_processing_time_ns = 2000000000,
        .engine_type = LUX_ENGINE_DAG
    };
    
    // Warmup
    for (int i = 0; i < WARMUP_ITERATIONS; i++) {
        lux_consensus_engine_t* engine = NULL;
        lux_consensus_engine_create(&engine, &config);
        lux_consensus_engine_destroy(engine);
    }
    
    // Benchmark
    for (int i = 0; i < BENCHMARK_ITERATIONS; i++) {
        double start = get_time_ms();
        
        lux_consensus_engine_t* engine = NULL;
        lux_consensus_engine_create(&engine, &config);
        
        double end = get_time_ms();
        times[i] = end - start;
        
        lux_consensus_engine_destroy(engine);
    }
    
    benchmark_stats_t stats;
    calculate_stats(times, BENCHMARK_ITERATIONS, &stats);
    print_benchmark_result("Engine Creation", &stats, "ms");
    
    lux_consensus_cleanup();
}

// Benchmark: Block operations
void benchmark_block_operations() {
    printf("\n=== BENCHMARK: Block Operations ===\n");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {
        .k = 20,
        .alpha_preference = 15,
        .alpha_confidence = 15,
        .beta = 20,
        .concurrent_polls = 1,
        .optimal_processing = 1,
        .max_outstanding_items = 10000,
        .max_item_processing_time_ns = 2000000000,
        .engine_type = LUX_ENGINE_DAG
    };
    
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Benchmark single block addition
    double times_single[BENCHMARK_ITERATIONS];
    for (int i = 0; i < BENCHMARK_ITERATIONS; i++) {
        lux_block_t block = {0};
        memset(block.id, i & 0xFF, 32);
        block.id[1] = (i >> 8) & 0xFF;
        memset(block.parent_id, 0, 32);
        block.height = i;
        block.timestamp = time(NULL);
        
        double start = get_time_ms();
        lux_consensus_add_block(engine, &block);
        double end = get_time_ms();
        
        times_single[i] = (end - start) * 1000; // Convert to microseconds
    }
    
    benchmark_stats_t stats_single;
    calculate_stats(times_single, BENCHMARK_ITERATIONS, &stats_single);
    print_benchmark_result("Single Block Add", &stats_single, "μs");
    
    // Benchmark batch operations
    lux_consensus_engine_destroy(engine);
    lux_consensus_engine_create(&engine, &config);
    
    int batch_sizes[] = {NUM_BLOCKS_SMALL, NUM_BLOCKS_MEDIUM, NUM_BLOCKS_LARGE};
    const char* batch_names[] = {"100 Blocks", "1000 Blocks", "10000 Blocks"};
    
    for (int b = 0; b < 3; b++) {
        double start = get_time_ms();
        
        for (int i = 0; i < batch_sizes[b]; i++) {
            lux_block_t block = {0};
            memset(block.id, i & 0xFF, 32);
            block.id[1] = (i >> 8) & 0xFF;
            block.id[2] = (i >> 16) & 0xFF;
            memset(block.parent_id, 0, 32);
            block.height = i;
            block.timestamp = time(NULL);
            lux_consensus_add_block(engine, &block);
        }
        
        double end = get_time_ms();
        double total_ms = end - start;
        double per_block_us = (total_ms * 1000) / batch_sizes[b];
        
        printf("%-30s: total=%.2fms per_block=%.2fμs throughput=%.0f blocks/s\n",
               batch_names[b], total_ms, per_block_us, 
               (batch_sizes[b] * 1000.0) / total_ms);
        
        // Reset for next batch
        lux_consensus_engine_destroy(engine);
        lux_consensus_engine_create(&engine, &config);
    }
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// Benchmark: Vote processing
void benchmark_vote_processing() {
    printf("\n=== BENCHMARK: Vote Processing ===\n");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {
        .k = 20,
        .alpha_preference = 15,
        .alpha_confidence = 15,
        .beta = 20,
        .concurrent_polls = 1,
        .optimal_processing = 1,
        .max_outstanding_items = 1024,
        .max_item_processing_time_ns = 2000000000,
        .engine_type = LUX_ENGINE_DAG
    };
    
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Add some blocks to vote on
    for (int i = 0; i < 100; i++) {
        lux_block_t block = {0};
        memset(block.id, i, 32);
        memset(block.parent_id, 0, 32);
        block.height = i;
        block.timestamp = time(NULL);
        lux_consensus_add_block(engine, &block);
    }
    
    // Benchmark single vote
    double times_single[BENCHMARK_ITERATIONS];
    for (int i = 0; i < BENCHMARK_ITERATIONS; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i & 0xFF, 32);
        vote.voter_id[1] = (i >> 8) & 0xFF;
        memset(vote.block_id, i % 100, 32);
        vote.is_preference = (i % 2 == 0);
        
        double start = get_time_ms();
        lux_consensus_process_vote(engine, &vote);
        double end = get_time_ms();
        
        times_single[i] = (end - start) * 1000; // Convert to microseconds
    }
    
    benchmark_stats_t stats_single;
    calculate_stats(times_single, BENCHMARK_ITERATIONS, &stats_single);
    print_benchmark_result("Single Vote Process", &stats_single, "μs");
    
    // Benchmark batch vote processing
    int vote_batches[] = {NUM_VOTES_SMALL, NUM_VOTES_MEDIUM, NUM_VOTES_LARGE};
    const char* vote_names[] = {"1000 Votes", "10000 Votes", "100000 Votes"};
    
    for (int b = 0; b < 3; b++) {
        double start = get_time_ms();
        
        for (int i = 0; i < vote_batches[b]; i++) {
            lux_vote_t vote = {0};
            memset(vote.voter_id, i & 0xFF, 32);
            vote.voter_id[1] = (i >> 8) & 0xFF;
            vote.voter_id[2] = (i >> 16) & 0xFF;
            memset(vote.block_id, i % 100, 32);
            vote.is_preference = (i % 2 == 0);
            lux_consensus_process_vote(engine, &vote);
        }
        
        double end = get_time_ms();
        double total_ms = end - start;
        double per_vote_us = (total_ms * 1000) / vote_batches[b];
        
        printf("%-30s: total=%.2fms per_vote=%.2fμs throughput=%.0f votes/s\n",
               vote_names[b], total_ms, per_vote_us,
               (vote_batches[b] * 1000.0) / total_ms);
    }
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// Benchmark: Query operations
void benchmark_query_operations() {
    printf("\n=== BENCHMARK: Query Operations ===\n");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {
        .k = 20,
        .alpha_preference = 15,
        .alpha_confidence = 15,
        .beta = 20,
        .concurrent_polls = 1,
        .optimal_processing = 1,
        .max_outstanding_items = 1024,
        .max_item_processing_time_ns = 2000000000,
        .engine_type = LUX_ENGINE_DAG
    };
    
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Add blocks
    uint8_t block_ids[1000][32];
    for (int i = 0; i < 1000; i++) {
        lux_block_t block = {0};
        memset(block.id, i & 0xFF, 32);
        block.id[1] = (i >> 8) & 0xFF;
        memcpy(block_ids[i], block.id, 32);
        memset(block.parent_id, 0, 32);
        block.height = i;
        block.timestamp = time(NULL);
        lux_consensus_add_block(engine, &block);
    }
    
    // Benchmark is_accepted queries
    double times_accepted[BENCHMARK_ITERATIONS];
    for (int i = 0; i < BENCHMARK_ITERATIONS; i++) {
        bool is_accepted;
        int block_idx = i % 1000;
        
        double start = get_time_ms();
        lux_consensus_is_accepted(engine, block_ids[block_idx], &is_accepted);
        double end = get_time_ms();
        
        times_accepted[i] = (end - start) * 1000; // Convert to microseconds
    }
    
    benchmark_stats_t stats_accepted;
    calculate_stats(times_accepted, BENCHMARK_ITERATIONS, &stats_accepted);
    print_benchmark_result("Is Accepted Query", &stats_accepted, "μs");
    
    // Benchmark get_preference
    double times_pref[BENCHMARK_ITERATIONS];
    for (int i = 0; i < BENCHMARK_ITERATIONS; i++) {
        uint8_t pref_id[32];
        
        double start = get_time_ms();
        lux_consensus_get_preference(engine, pref_id);
        double end = get_time_ms();
        
        times_pref[i] = (end - start) * 1000; // Convert to microseconds
    }
    
    benchmark_stats_t stats_pref;
    calculate_stats(times_pref, BENCHMARK_ITERATIONS, &stats_pref);
    print_benchmark_result("Get Preference", &stats_pref, "μs");
    
    // Benchmark get_stats
    double times_stats[BENCHMARK_ITERATIONS];
    for (int i = 0; i < BENCHMARK_ITERATIONS; i++) {
        lux_consensus_stats_t stats;
        
        double start = get_time_ms();
        lux_consensus_get_stats(engine, &stats);
        double end = get_time_ms();
        
        times_stats[i] = (end - start) * 1000; // Convert to microseconds
    }
    
    benchmark_stats_t stats_stats;
    calculate_stats(times_stats, BENCHMARK_ITERATIONS, &stats_stats);
    print_benchmark_result("Get Stats", &stats_stats, "μs");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// Benchmark: Concurrent operations
typedef struct {
    lux_consensus_engine_t* engine;
    int thread_id;
    int num_operations;
    double elapsed_ms;
} thread_benchmark_data_t;

void* concurrent_add_blocks(void* arg) {
    thread_benchmark_data_t* data = (thread_benchmark_data_t*)arg;
    
    double start = get_time_ms();
    
    for (int i = 0; i < data->num_operations; i++) {
        lux_block_t block = {0};
        block.id[0] = data->thread_id;
        block.id[1] = i & 0xFF;
        block.id[2] = (i >> 8) & 0xFF;
        memset(block.parent_id, 0, 32);
        block.height = i;
        block.timestamp = time(NULL);
        lux_consensus_add_block(data->engine, &block);
    }
    
    double end = get_time_ms();
    data->elapsed_ms = end - start;
    
    return NULL;
}

void benchmark_concurrent_operations() {
    printf("\n=== BENCHMARK: Concurrent Operations ===\n");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {
        .k = 20,
        .alpha_preference = 15,
        .alpha_confidence = 15,
        .beta = 20,
        .concurrent_polls = 1,
        .optimal_processing = 1,
        .max_outstanding_items = 10000,
        .max_item_processing_time_ns = 2000000000,
        .engine_type = LUX_ENGINE_DAG
    };
    
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    int thread_counts[] = {1, 2, 4, 8};
    int operations_per_thread = 1000;
    
    for (int t = 0; t < 4; t++) {
        int num_threads = thread_counts[t];
        pthread_t threads[num_threads];
        thread_benchmark_data_t thread_data[num_threads];
        
        double start = get_time_ms();
        
        // Start threads
        for (int i = 0; i < num_threads; i++) {
            thread_data[i].engine = engine;
            thread_data[i].thread_id = i;
            thread_data[i].num_operations = operations_per_thread;
            pthread_create(&threads[i], NULL, concurrent_add_blocks, &thread_data[i]);
        }
        
        // Wait for threads
        for (int i = 0; i < num_threads; i++) {
            pthread_join(threads[i], NULL);
        }
        
        double end = get_time_ms();
        double total_ms = end - start;
        int total_ops = num_threads * operations_per_thread;
        double throughput = (total_ops * 1000.0) / total_ms;
        
        printf("%d Threads (1000 ops/thread): total=%.2fms throughput=%.0f ops/s speedup=%.2fx\n",
               num_threads, total_ms, throughput, 
               (num_threads == 1) ? 1.0 : throughput / (1000000.0 / total_ms));
        
        // Reset engine for next test
        lux_consensus_engine_destroy(engine);
        lux_consensus_engine_create(&engine, &config);
    }
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// Benchmark: Memory usage
void benchmark_memory_usage() {
    printf("\n=== BENCHMARK: Memory Usage ===\n");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {
        .k = 20,
        .alpha_preference = 15,
        .alpha_confidence = 15,
        .beta = 20,
        .concurrent_polls = 1,
        .optimal_processing = 1,
        .max_outstanding_items = 100000,
        .max_item_processing_time_ns = 2000000000,
        .engine_type = LUX_ENGINE_DAG
    };
    
    int block_counts[] = {1000, 10000, 100000};
    const char* block_names[] = {"1K Blocks", "10K Blocks", "100K Blocks"};
    
    for (int b = 0; b < 3; b++) {
        lux_consensus_engine_t* engine = NULL;
        lux_consensus_engine_create(&engine, &config);
        
        double start = get_time_ms();
        
        for (int i = 0; i < block_counts[b]; i++) {
            lux_block_t block = {0};
            block.id[0] = i & 0xFF;
            block.id[1] = (i >> 8) & 0xFF;
            block.id[2] = (i >> 16) & 0xFF;
            block.id[3] = (i >> 24) & 0xFF;
            memset(block.parent_id, 0, 32);
            block.height = i;
            block.timestamp = time(NULL);
            
            // Add some data to blocks
            char data[64];
            sprintf(data, "Block data %d", i);
            block.data = data;
            block.data_size = strlen(data);
            
            lux_consensus_add_block(engine, &block);
        }
        
        double end = get_time_ms();
        
        // Get stats to verify blocks were added
        lux_consensus_stats_t stats;
        lux_consensus_get_stats(engine, &stats);
        
        printf("%-30s: time=%.2fms blocks_stored=%d avg_time=%.3fμs/block\n",
               block_names[b], end - start, block_counts[b],
               ((end - start) * 1000) / block_counts[b]);
        
        lux_consensus_engine_destroy(engine);
    }
    
    lux_consensus_cleanup();
}

// Main benchmark runner
int main() {
    printf("=====================================\n");
    printf("=== LUX CONSENSUS C BENCHMARKS ===\n");
    printf("=====================================\n");
    printf("Iterations: %d\n", BENCHMARK_ITERATIONS);
    printf("Warmup: %d iterations\n", WARMUP_ITERATIONS);
    
    benchmark_engine_creation();
    benchmark_block_operations();
    benchmark_vote_processing();
    benchmark_query_operations();
    benchmark_concurrent_operations();
    benchmark_memory_usage();
    
    printf("\n=====================================\n");
    printf("=== BENCHMARK COMPLETE ===\n");
    printf("=====================================\n");
    
    return 0;
}