// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <assert.h>
#include <time.h>
#include "../include/lux_consensus.h"

// Test helper functions
void print_test_header(const char* test_name) {
    printf("\n=== TEST: %s ===\n", test_name);
}

void print_test_result(const char* test_name, bool passed) {
    printf("[%s] %s\n", passed ? "PASS" : "FAIL", test_name);
}

// Test basic initialization
bool test_initialization() {
    print_test_header("Initialization");
    
    lux_error_t err = lux_consensus_init();
    assert(err == LUX_SUCCESS);
    
    err = lux_consensus_cleanup();
    assert(err == LUX_SUCCESS);
    
    print_test_result("Initialization", true);
    return true;
}

// Test engine creation and destruction
bool test_engine_lifecycle() {
    print_test_header("Engine Lifecycle");
    
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
    lux_error_t err = lux_consensus_engine_create(&engine, &config);
    assert(err == LUX_SUCCESS);
    assert(engine != NULL);
    
    err = lux_consensus_engine_destroy(engine);
    assert(err == LUX_SUCCESS);
    
    lux_consensus_cleanup();
    
    print_test_result("Engine Lifecycle", true);
    return true;
}

// Test block operations
bool test_block_operations() {
    print_test_header("Block Operations");
    
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
    
    // Create a test block
    lux_block_t block = {0};
    memset(block.id, 1, 32); // Set block ID to all 1s
    memset(block.parent_id, 0, 32); // Genesis parent
    block.height = 1;
    block.timestamp = (uint64_t)time(NULL);
    
    const char* block_data = "Test block data";
    block.data = (void*)block_data;
    block.data_size = strlen(block_data);
    
    // Add block to consensus
    lux_error_t err = lux_consensus_add_block(engine, &block);
    assert(err == LUX_SUCCESS);
    
    // Add same block again (should succeed due to idempotency)
    err = lux_consensus_add_block(engine, &block);
    assert(err == LUX_SUCCESS);
    
    // Check if block is accepted (should not be yet)
    bool is_accepted = false;
    err = lux_consensus_is_accepted(engine, block.id, &is_accepted);
    assert(err == LUX_SUCCESS);
    assert(is_accepted == false);
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    
    print_test_result("Block Operations", true);
    return true;
}

// Test voting
bool test_voting() {
    print_test_header("Voting");
    
    lux_consensus_init();
    
    lux_consensus_config_t config = {
        .k = 20,
        .alpha_preference = 2,
        .alpha_confidence = 2,
        .beta = 3,
        .concurrent_polls = 1,
        .optimal_processing = 1,
        .max_outstanding_items = 1024,
        .max_item_processing_time_ns = 2000000000,
        .engine_type = LUX_ENGINE_DAG
    };
    
    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);
    
    // Add a block
    lux_block_t block = {0};
    memset(block.id, 2, 32);
    memset(block.parent_id, 0, 32);
    block.height = 1;
    block.timestamp = (uint64_t)time(NULL);
    
    lux_error_t err = lux_consensus_add_block(engine, &block);
    assert(err == LUX_SUCCESS);
    
    // Cast votes
    for (int i = 0; i < 3; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i, 32);
        memcpy(vote.block_id, block.id, 32);
        vote.is_preference = false;
        
        err = lux_consensus_process_vote(engine, &vote);
        assert(err == LUX_SUCCESS);
    }
    
    // Check statistics
    lux_consensus_stats_t stats;
    err = lux_consensus_get_stats(engine, &stats);
    assert(err == LUX_SUCCESS);
    assert(stats.votes_processed == 3);
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    
    print_test_result("Voting", true);
    return true;
}

// Test preference
bool test_preference() {
    print_test_header("Preference");
    
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
    
    // Get initial preference (should be genesis)
    uint8_t pref_id[32];
    lux_error_t err = lux_consensus_get_preference(engine, pref_id);
    assert(err == LUX_SUCCESS);
    
    // Preference should be all zeros initially (genesis)
    bool is_genesis = true;
    for (int i = 0; i < 32; i++) {
        if (pref_id[i] != 0) {
            is_genesis = false;
            break;
        }
    }
    assert(is_genesis);
    
    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    
    print_test_result("Preference", true);
    return true;
}

// Test error handling
bool test_error_handling() {
    print_test_header("Error Handling");
    
    // Test invalid parameters
    lux_error_t err = lux_consensus_engine_create(NULL, NULL);
    assert(err == LUX_ERROR_INVALID_PARAMS);
    
    err = lux_consensus_engine_destroy(NULL);
    assert(err == LUX_ERROR_INVALID_PARAMS);
    
    // Test error strings
    const char* err_str = lux_error_string(LUX_SUCCESS);
    assert(strcmp(err_str, "Success") == 0);
    
    err_str = lux_error_string(LUX_ERROR_INVALID_PARAMS);
    assert(strcmp(err_str, "Invalid parameters") == 0);
    
    // Test engine type strings
    const char* type_str = lux_engine_type_string(LUX_ENGINE_CHAIN);
    assert(strcmp(type_str, "Chain") == 0);
    
    type_str = lux_engine_type_string(LUX_ENGINE_DAG);
    assert(strcmp(type_str, "DAG") == 0);
    
    print_test_result("Error Handling", true);
    return true;
}

// Main test runner
int main() {
    printf("=== Lux Consensus C Library Tests ===\n");
    printf("=====================================\n");
    
    int tests_passed = 0;
    int tests_failed = 0;
    
    // Run all tests
    if (test_initialization()) tests_passed++; else tests_failed++;
    if (test_engine_lifecycle()) tests_passed++; else tests_failed++;
    if (test_block_operations()) tests_passed++; else tests_failed++;
    if (test_voting()) tests_passed++; else tests_failed++;
    if (test_preference()) tests_passed++; else tests_failed++;
    if (test_error_handling()) tests_passed++; else tests_failed++;
    
    // Print summary
    printf("\n=====================================\n");
    printf("SUMMARY: %d passed, %d failed\n", tests_passed, tests_failed);
    
    if (tests_failed == 0) {
        printf("✅ ALL TESTS PASSED!\n");
        return 0;
    } else {
        printf("❌ SOME TESTS FAILED!\n");
        return 1;
    }
}