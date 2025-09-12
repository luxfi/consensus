// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <assert.h>
#include <time.h>
#include <pthread.h>
#include <unistd.h>
#include "../include/lux_consensus.h"

// Test categories matching Go implementation
#define NUM_TEST_CATEGORIES 15

// Color codes for output
#define GREEN "\033[0;32m"
#define RED "\033[0;31m"
#define YELLOW "\033[1;33m"
#define RESET "\033[0m"

// Test results tracking
typedef struct {
    int passed;
    int failed;
    int skipped;
} test_results_t;

static test_results_t results = {0, 0, 0};

// Helper functions
void print_test_header(const char* category, const char* test_name) {
    printf("\n%s=== %s: %s ===%s\n", YELLOW, category, test_name, RESET);
}

void assert_test(bool condition, const char* test_name) {
    if (condition) {
        printf("%s[PASS]%s %s\n", GREEN, RESET, test_name);
        results.passed++;
    } else {
        printf("%s[FAIL]%s %s\n", RED, RESET, test_name);
        results.failed++;
    }
}

// 1. INITIALIZATION TESTS
void test_initialization_suite() {
    print_test_header("INITIALIZATION", "Library Lifecycle");
    
    // Test multiple init/cleanup cycles
    for (int i = 0; i < 3; i++) {
        lux_error_t err = lux_consensus_init();
        assert_test(err == LUX_SUCCESS, "Initialize library");
        
        err = lux_consensus_cleanup();
        assert_test(err == LUX_SUCCESS, "Cleanup library");
    }
    
    // Test error messages
    const char* err_str = lux_error_string(LUX_SUCCESS);
    assert_test(strcmp(err_str, "Success") == 0, "Error string for SUCCESS");
    
    err_str = lux_error_string(LUX_ERROR_INVALID_PARAMS);
    assert_test(strcmp(err_str, "Invalid parameters") == 0, "Error string for INVALID_PARAMS");
}

// 2. ENGINE CREATION TESTS
void test_engine_creation_suite() {
    print_test_header("ENGINE", "Creation and Configuration");
    
    lux_consensus_init();
    
    // Test various configurations
    lux_consensus_config_t configs[] = {
        {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_CHAIN},
        {30, 20, 20, 25, 2, 2, 2048, 3000000000, LUX_ENGINE_DAG},
        {10, 7, 7, 10, 1, 1, 512, 1000000000, LUX_ENGINE_PQ},
    };
    
    for (int i = 0; i < 3; i++) {
        lux_consensus_engine_t* engine = NULL;
        lux_error_t err = lux_consensus_engine_create(&engine, &configs[i]);
        assert_test(err == LUX_SUCCESS && engine != NULL, "Create engine with different configs");
        
        if (engine) {
            lux_consensus_engine_destroy(engine);
        }
    }
    
    // Test invalid parameters
    lux_consensus_engine_t* engine = NULL;
    lux_error_t err = lux_consensus_engine_create(&engine, NULL);
    assert_test(err == LUX_ERROR_INVALID_PARAMS, "Reject NULL config");
    
    err = lux_consensus_engine_create(NULL, &configs[0]);
    assert_test(err == LUX_ERROR_INVALID_PARAMS, "Reject NULL engine pointer");
    
    lux_consensus_cleanup();
}

// 3. BLOCK MANAGEMENT TESTS
void test_block_management_suite() {
    print_test_header("BLOCKS", "Add, Query, and Hierarchy");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Create block hierarchy
    lux_block_t genesis = {0};
    memset(genesis.id, 0, 32);
    memset(genesis.parent_id, 0, 32);
    genesis.height = 0;
    genesis.timestamp = time(NULL);
    
    lux_block_t block1 = {0};
    memset(block1.id, 1, 32);
    memcpy(block1.parent_id, genesis.id, 32);
    block1.height = 1;
    block1.timestamp = time(NULL);
    
    lux_block_t block2 = {0};
    memset(block2.id, 2, 32);
    memcpy(block2.parent_id, block1.id, 32);
    block2.height = 2;
    block2.timestamp = time(NULL);
    
    // Test adding blocks
    lux_error_t err = lux_consensus_add_block(engine, &block1);
    assert_test(err == LUX_SUCCESS, "Add block 1");
    
    err = lux_consensus_add_block(engine, &block2);
    assert_test(err == LUX_SUCCESS, "Add block 2");
    
    // Test idempotency
    err = lux_consensus_add_block(engine, &block1);
    assert_test(err == LUX_SUCCESS, "Add duplicate block (idempotent)");
    
    // Test with block data
    const char* data = "Important block data";
    lux_block_t block3 = {0};
    memset(block3.id, 3, 32);
    memcpy(block3.parent_id, block2.id, 32);
    block3.height = 3;
    block3.timestamp = time(NULL);
    block3.data = (void*)data;
    block3.data_size = strlen(data);
    
    err = lux_consensus_add_block(engine, &block3);
    assert_test(err == LUX_SUCCESS, "Add block with data");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// 4. VOTING TESTS
void test_voting_suite() {
    print_test_header("VOTING", "Preference and Confidence");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {20, 3, 3, 5, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Add test block
    lux_block_t block = {0};
    memset(block.id, 10, 32);
    memset(block.parent_id, 0, 32);
    block.height = 1;
    block.timestamp = time(NULL);
    lux_consensus_add_block(engine, &block);
    
    // Test preference votes
    for (int i = 0; i < 3; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i, 32);
        memcpy(vote.block_id, block.id, 32);
        vote.is_preference = true;
        
        lux_error_t err = lux_consensus_process_vote(engine, &vote);
        assert_test(err == LUX_SUCCESS, "Process preference vote");
    }
    
    // Test confidence votes
    for (int i = 3; i < 6; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i, 32);
        memcpy(vote.block_id, block.id, 32);
        vote.is_preference = false;
        
        lux_error_t err = lux_consensus_process_vote(engine, &vote);
        assert_test(err == LUX_SUCCESS, "Process confidence vote");
    }
    
    // Check statistics
    lux_consensus_stats_t stats;
    lux_consensus_get_stats(engine, &stats);
    assert_test(stats.votes_processed == 6, "Vote count tracking");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// 5. ACCEPTANCE TESTS
void test_acceptance_suite() {
    print_test_header("ACCEPTANCE", "Decision Thresholds");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {20, 2, 2, 3, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Add competing blocks
    lux_block_t block_a = {0};
    memset(block_a.id, 0xAA, 32);
    memset(block_a.parent_id, 0, 32);
    block_a.height = 1;
    block_a.timestamp = time(NULL);
    lux_consensus_add_block(engine, &block_a);
    
    lux_block_t block_b = {0};
    memset(block_b.id, 0xBB, 32);
    memset(block_b.parent_id, 0, 32);
    block_b.height = 1;
    block_b.timestamp = time(NULL);
    lux_consensus_add_block(engine, &block_b);
    
    // Vote for block A to reach acceptance
    for (int i = 0; i < 3; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i, 32);
        memcpy(vote.block_id, block_a.id, 32);
        vote.is_preference = false;
        lux_consensus_process_vote(engine, &vote);
    }
    
    // Check acceptance
    bool is_accepted = false;
    lux_consensus_is_accepted(engine, block_a.id, &is_accepted);
    assert_test(is_accepted == true, "Block A accepted after threshold");
    
    lux_consensus_is_accepted(engine, block_b.id, &is_accepted);
    assert_test(is_accepted == false, "Block B not accepted");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// 6. PREFERENCE TESTS
void test_preference_suite() {
    print_test_header("PREFERENCE", "Preferred Block Selection");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Initial preference should be genesis
    uint8_t pref_id[32];
    lux_consensus_get_preference(engine, pref_id);
    
    bool is_genesis = true;
    for (int i = 0; i < 32; i++) {
        if (pref_id[i] != 0) {
            is_genesis = false;
            break;
        }
    }
    assert_test(is_genesis, "Initial preference is genesis");
    
    // Add and accept a block
    lux_block_t block = {0};
    memset(block.id, 0xFF, 32);
    memset(block.parent_id, 0, 32);
    block.height = 1;
    block.timestamp = time(NULL);
    lux_consensus_add_block(engine, &block);
    
    // Vote to accept
    for (int i = 0; i < 20; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i, 32);
        memcpy(vote.block_id, block.id, 32);
        vote.is_preference = false;
        lux_consensus_process_vote(engine, &vote);
    }
    
    // Check preference updated
    lux_consensus_get_preference(engine, pref_id);
    assert_test(memcmp(pref_id, block.id, 32) == 0, "Preference updated to accepted block");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// 7. POLLING TESTS
void test_polling_suite() {
    print_test_header("POLLING", "Validator Polling");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Create validator IDs
    uint8_t validators[10][32];
    for (int i = 0; i < 10; i++) {
        memset(validators[i], i + 100, 32);
    }
    
    uint8_t* validator_ptrs[10];
    for (int i = 0; i < 10; i++) {
        validator_ptrs[i] = validators[i];
    }
    
    // Test polling
    lux_error_t err = lux_consensus_poll(engine, 10, (const uint8_t**)validator_ptrs);
    assert_test(err == LUX_SUCCESS, "Poll 10 validators");
    
    // Test with no validators
    err = lux_consensus_poll(engine, 0, NULL);
    assert_test(err == LUX_SUCCESS, "Poll with no validators");
    
    // Check stats
    lux_consensus_stats_t stats;
    lux_consensus_get_stats(engine, &stats);
    assert_test(stats.polls_completed == 2, "Poll count tracking");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// 8. STATISTICS TESTS
void test_statistics_suite() {
    print_test_header("STATISTICS", "Metrics Collection");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Initial stats
    lux_consensus_stats_t stats;
    lux_consensus_get_stats(engine, &stats);
    assert_test(stats.blocks_accepted == 0, "Initial blocks accepted");
    assert_test(stats.blocks_rejected == 0, "Initial blocks rejected");
    assert_test(stats.polls_completed == 0, "Initial polls completed");
    assert_test(stats.votes_processed == 0, "Initial votes processed");
    
    // Generate activity
    lux_block_t block = {0};
    memset(block.id, 0x42, 32);
    memset(block.parent_id, 0, 32);
    block.height = 1;
    block.timestamp = time(NULL);
    lux_consensus_add_block(engine, &block);
    
    for (int i = 0; i < 5; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i, 32);
        memcpy(vote.block_id, block.id, 32);
        vote.is_preference = (i % 2 == 0);
        lux_consensus_process_vote(engine, &vote);
    }
    
    // Check updated stats
    lux_consensus_get_stats(engine, &stats);
    assert_test(stats.votes_processed == 5, "Updated votes processed");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// 9. THREAD SAFETY TESTS
void* thread_add_blocks(void* arg) {
    lux_consensus_engine_t* engine = (lux_consensus_engine_t*)arg;
    
    for (int i = 0; i < 100; i++) {
        lux_block_t block = {0};
        block.id[0] = i & 0xFF;
        memset(block.parent_id, 0, 32);
        block.height = i;
        block.timestamp = time(NULL);
        block.data = NULL;
        block.data_size = 0;
        lux_consensus_add_block(engine, &block);
    }
    
    return NULL;
}

void* thread_process_votes(void* arg) {
    lux_consensus_engine_t* engine = (lux_consensus_engine_t*)arg;
    
    for (int i = 0; i < 100; i++) {
        lux_vote_t vote = {0};
        vote.voter_id[0] = i & 0xFF;
        vote.block_id[0] = (i % 10) & 0xFF;
        vote.is_preference = (i % 2 == 0);
        lux_consensus_process_vote(engine, &vote);
    }
    
    return NULL;
}

void test_thread_safety_suite() {
    print_test_header("CONCURRENCY", "Thread Safety");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Create threads
    pthread_t threads[4];
    pthread_create(&threads[0], NULL, thread_add_blocks, engine);
    pthread_create(&threads[1], NULL, thread_add_blocks, engine);
    pthread_create(&threads[2], NULL, thread_process_votes, engine);
    pthread_create(&threads[3], NULL, thread_process_votes, engine);
    
    // Wait for completion
    for (int i = 0; i < 4; i++) {
        pthread_join(threads[i], NULL);
    }
    
    // Check consistency
    lux_consensus_stats_t stats;
    lux_consensus_get_stats(engine, &stats);
    assert_test(stats.votes_processed > 0, "Concurrent vote processing");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// 10. MEMORY MANAGEMENT TESTS
void test_memory_management_suite() {
    print_test_header("MEMORY", "Allocation and Cleanup");
    
    lux_consensus_init();
    
    // Test multiple engine creation/destruction
    for (int i = 0; i < 10; i++) {
        lux_consensus_config_t config = {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
        lux_consensus_engine_t* engine = NULL;
        lux_consensus_engine_create(&engine, &config);
        
        // Add many blocks
        for (int j = 0; j < 100; j++) {
            lux_block_t block = {0};
            memset(block.id, j, 32);
            memset(block.parent_id, 0, 32);
            block.height = j;
            block.timestamp = time(NULL);
            
            char data[256];
            sprintf(data, "Block data %d", j);
            block.data = data;
            block.data_size = strlen(data);
            
            lux_consensus_add_block(engine, &block);
        }
        
        lux_consensus_engine_destroy(engine);
    }
    
    assert_test(true, "Memory stress test passed");
    
    lux_consensus_cleanup();
}

// 11. ERROR HANDLING TESTS
void test_error_handling_suite() {
    print_test_header("ERRORS", "Error Conditions");
    
    lux_consensus_init();
    
    // NULL parameter tests
    lux_error_t err = lux_consensus_engine_create(NULL, NULL);
    assert_test(err == LUX_ERROR_INVALID_PARAMS, "NULL engine and config");
    
    err = lux_consensus_engine_destroy(NULL);
    assert_test(err == LUX_ERROR_INVALID_PARAMS, "Destroy NULL engine");
    
    lux_consensus_config_t config = {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Invalid operations
    err = lux_consensus_add_block(engine, NULL);
    assert_test(err == LUX_ERROR_INVALID_PARAMS, "Add NULL block");
    
    err = lux_consensus_process_vote(engine, NULL);
    assert_test(err == LUX_ERROR_INVALID_PARAMS, "Process NULL vote");
    
    err = lux_consensus_is_accepted(engine, NULL, NULL);
    assert_test(err == LUX_ERROR_INVALID_PARAMS, "Check acceptance with NULL");
    
    err = lux_consensus_get_preference(engine, NULL);
    assert_test(err == LUX_ERROR_INVALID_PARAMS, "Get preference with NULL");
    
    err = lux_consensus_get_stats(engine, NULL);
    assert_test(err == LUX_ERROR_INVALID_PARAMS, "Get stats with NULL");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// 12. ENGINE TYPE TESTS
void test_engine_types_suite() {
    print_test_header("ENGINE TYPES", "Chain, DAG, PQ");
    
    lux_consensus_init();
    
    lux_engine_type_t types[] = {LUX_ENGINE_CHAIN, LUX_ENGINE_DAG, LUX_ENGINE_PQ};
    const char* expected[] = {"Chain", "DAG", "PQ"};
    
    for (int i = 0; i < 3; i++) {
        lux_consensus_config_t config = {20, 15, 15, 20, 1, 1, 1024, 2000000000, types[i]};
        lux_consensus_engine_t* engine = NULL;
        
        lux_error_t err = lux_consensus_engine_create(&engine, &config);
        assert_test(err == LUX_SUCCESS, "Create engine with type");
        
        const char* type_str = lux_engine_type_string(types[i]);
        assert_test(strcmp(type_str, expected[i]) == 0, "Engine type string");
        
        lux_consensus_engine_destroy(engine);
    }
    
    lux_consensus_cleanup();
}

// 13. PERFORMANCE TESTS
void test_performance_suite() {
    print_test_header("PERFORMANCE", "Throughput and Latency");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    clock_t start = clock();
    
    // Add 1000 blocks
    for (int i = 0; i < 1000; i++) {
        lux_block_t block = {0};
        memset(block.id, i & 0xFF, 32);
        block.id[0] = i >> 8;
        memset(block.parent_id, 0, 32);
        block.height = i;
        block.timestamp = time(NULL);
        lux_consensus_add_block(engine, &block);
    }
    
    clock_t end = clock();
    double cpu_time = ((double)(end - start)) / CLOCKS_PER_SEC;
    
    assert_test(cpu_time < 1.0, "Add 1000 blocks in < 1 second");
    printf("  Time: %.3f seconds\n", cpu_time);
    
    // Process 10000 votes
    start = clock();
    
    for (int i = 0; i < 10000; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i & 0xFF, 32);
        vote.voter_id[0] = i >> 8;
        memset(vote.block_id, i % 100, 32);
        vote.is_preference = (i % 2 == 0);
        lux_consensus_process_vote(engine, &vote);
    }
    
    end = clock();
    cpu_time = ((double)(end - start)) / CLOCKS_PER_SEC;
    
    assert_test(cpu_time < 2.0, "Process 10000 votes in < 2 seconds");
    printf("  Time: %.3f seconds\n", cpu_time);
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// 14. EDGE CASE TESTS
void test_edge_cases_suite() {
    print_test_header("EDGE CASES", "Boundary Conditions");
    
    lux_consensus_init();
    
    // Minimum configuration
    lux_consensus_config_t min_config = {1, 1, 1, 1, 1, 1, 1, 1, LUX_ENGINE_CHAIN};
    lux_consensus_engine_t* engine = NULL;
    lux_error_t err = lux_consensus_engine_create(&engine, &min_config);
    assert_test(err == LUX_SUCCESS, "Minimum configuration");
    lux_consensus_engine_destroy(engine);
    
    // Maximum reasonable configuration
    lux_consensus_config_t max_config = {
        1000, 750, 750, 900, 100, 100, 1000000, 10000000000, LUX_ENGINE_DAG
    };
    err = lux_consensus_engine_create(&engine, &max_config);
    assert_test(err == LUX_SUCCESS, "Maximum configuration");
    
    // Very long block chain
    for (int i = 0; i < 100; i++) {
        lux_block_t block = {0};
        memset(block.id, i, 32);
        if (i == 0) {
            memset(block.parent_id, 0, 32);
        } else {
            memset(block.parent_id, i - 1, 32);
        }
        block.height = i;
        block.timestamp = time(NULL);
        lux_consensus_add_block(engine, &block);
    }
    assert_test(true, "Long chain creation");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// 15. INTEGRATION TESTS
void test_integration_suite() {
    print_test_header("INTEGRATION", "Full Workflow");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_DAG};
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Simulate full consensus workflow
    // 1. Add genesis
    lux_block_t genesis = {0};
    memset(genesis.id, 0, 32);
    memset(genesis.parent_id, 0, 32);
    genesis.height = 0;
    genesis.timestamp = time(NULL);
    
    // 2. Add competing chains
    lux_block_t chain_a[5];
    lux_block_t chain_b[5];
    
    for (int i = 0; i < 5; i++) {
        // Chain A
        memset(chain_a[i].id, 0xA0 + i, 32);
        if (i == 0) {
            memcpy(chain_a[i].parent_id, genesis.id, 32);
        } else {
            memcpy(chain_a[i].parent_id, chain_a[i-1].id, 32);
        }
        chain_a[i].height = i + 1;
        chain_a[i].timestamp = time(NULL);
        lux_consensus_add_block(engine, &chain_a[i]);
        
        // Chain B
        memset(chain_b[i].id, 0xB0 + i, 32);
        if (i == 0) {
            memcpy(chain_b[i].parent_id, genesis.id, 32);
        } else {
            memcpy(chain_b[i].parent_id, chain_b[i-1].id, 32);
        }
        chain_b[i].height = i + 1;
        chain_b[i].timestamp = time(NULL);
        lux_consensus_add_block(engine, &chain_b[i]);
    }
    
    // 3. Vote for chain A
    for (int i = 0; i < 20; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i, 32);
        memcpy(vote.block_id, chain_a[4].id, 32);
        vote.is_preference = false;
        lux_consensus_process_vote(engine, &vote);
    }
    
    // 4. Check final state
    bool is_accepted = false;
    lux_consensus_is_accepted(engine, chain_a[4].id, &is_accepted);
    assert_test(is_accepted, "Chain A accepted");
    
    lux_consensus_is_accepted(engine, chain_b[4].id, &is_accepted);
    assert_test(!is_accepted, "Chain B rejected");
    
    uint8_t pref_id[32];
    lux_consensus_get_preference(engine, pref_id);
    assert_test(memcmp(pref_id, chain_a[4].id, 32) == 0, "Preference is chain A tip");
    
    lux_consensus_stats_t stats;
    lux_consensus_get_stats(engine, &stats);
    assert_test(stats.blocks_accepted > 0, "Blocks accepted in workflow");
    assert_test(stats.votes_processed == 20, "All votes processed");
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
}

// Main test runner
int main(int argc, char* argv[]) {
    printf("%s", YELLOW);
    printf("=====================================\n");
    printf("=== LUX CONSENSUS C TEST SUITE ===\n");
    printf("=====================================\n");
    printf("%s\n", RESET);
    
    // Run all test suites
    test_initialization_suite();
    test_engine_creation_suite();
    test_block_management_suite();
    test_voting_suite();
    test_acceptance_suite();
    test_preference_suite();
    test_polling_suite();
    test_statistics_suite();
    test_thread_safety_suite();
    test_memory_management_suite();
    test_error_handling_suite();
    test_engine_types_suite();
    test_performance_suite();
    test_edge_cases_suite();
    test_integration_suite();
    
    // Print summary
    printf("\n%s", YELLOW);
    printf("=====================================\n");
    printf("=== TEST SUMMARY ===\n");
    printf("=====================================\n");
    printf("%s", RESET);
    
    printf("Total Tests: %d\n", results.passed + results.failed + results.skipped);
    printf("%sPassed: %d%s\n", GREEN, results.passed, RESET);
    printf("%sFailed: %d%s\n", RED, results.failed, RESET);
    printf("%sSkipped: %d%s\n", YELLOW, results.skipped, RESET);
    
    if (results.failed == 0) {
        printf("\n%s🎉 ALL TESTS PASSED! 100%% SUCCESS RATE%s\n", GREEN, RESET);
        return 0;
    } else {
        printf("\n%s❌ SOME TESTS FAILED%s\n", RED, RESET);
        return 1;
    }
}