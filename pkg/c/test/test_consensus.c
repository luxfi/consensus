// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

#include <lux_consensus.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <assert.h>

// ANSI color codes
#define COLOR_RESET   "\033[0m"
#define COLOR_RED     "\033[0;31m"
#define COLOR_GREEN   "\033[0;32m"
#define COLOR_YELLOW  "\033[1;33m"

// Test tracking
typedef struct {
    int passed;
    int failed;
    int skipped;
} test_results_t;

static test_results_t results = {0, 0, 0};

// Test helper macros
#define PRINT_HEADER(category, name) \
    printf("\n%s=== %s: %s ===%s\n", COLOR_YELLOW, category, name, COLOR_RESET)

#define ASSERT_TEST(condition, test_name) do { \
    if (condition) { \
        printf("%s[PASS]%s %s\n", COLOR_GREEN, COLOR_RESET, test_name); \
        results.passed++; \
    } else { \
        printf("%s[FAIL]%s %s\n", COLOR_RED, COLOR_RESET, test_name); \
        results.failed++; \
        return 1; \
    } \
} while(0)

// Helper to create default config
static void create_default_config(lux_consensus_config_t* config) {
    config->k = 20;
    config->alpha_preference = 15;
    config->alpha_confidence = 15;
    config->beta = 20;
    config->concurrent_polls = 1;
    config->optimal_processing = 1;
    config->max_outstanding_items = 1024;
    config->max_item_processing_time_ns = 2000000000;
    config->engine_type = LUX_ENGINE_DAG;
}

// 1. INITIALIZATION TESTS
static int test_initialization_suite(void) {
    PRINT_HEADER("INITIALIZATION", "Library Lifecycle");

    // Test multiple init/cleanup cycles
    for (int i = 0; i < 3; i++) {
        lux_error_t err = lux_consensus_init();
        char msg[100];
        snprintf(msg, sizeof(msg), "Initialize library cycle %d", i);
        ASSERT_TEST(err == LUX_SUCCESS, msg);

        err = lux_consensus_cleanup();
        snprintf(msg, sizeof(msg), "Cleanup library cycle %d", i);
        ASSERT_TEST(err == LUX_SUCCESS, msg);
    }

    // Test error strings
    const char* str = lux_error_string(LUX_SUCCESS);
    ASSERT_TEST(strcmp(str, "Success") == 0, "Error string for SUCCESS");

    str = lux_error_string(LUX_ERROR_INVALID_PARAMS);
    ASSERT_TEST(strcmp(str, "Invalid parameters") == 0, "Error string for INVALID_PARAMS");

    return 0;
}

// 2. ENGINE CREATION TESTS
static int test_engine_creation_suite(void) {
    PRINT_HEADER("ENGINE", "Creation and Configuration");

    lux_consensus_init();

    // Test various configurations
    lux_consensus_config_t configs[] = {
        {20, 15, 15, 20, 1, 1, 1024, 2000000000, LUX_ENGINE_CHAIN},
        {30, 20, 20, 25, 2, 2, 2048, 3000000000, LUX_ENGINE_DAG},
        {10, 7, 7, 10, 1, 1, 512, 1000000000, LUX_ENGINE_PQ},
    };

    for (size_t i = 0; i < sizeof(configs)/sizeof(configs[0]); i++) {
        lux_consensus_engine_t* engine = NULL;
        lux_error_t err = lux_consensus_engine_create(&engine, &configs[i]);

        char msg[100];
        snprintf(msg, sizeof(msg), "Create engine with config %zu", i);
        ASSERT_TEST(err == LUX_SUCCESS && engine != NULL, msg);

        lux_consensus_engine_destroy(engine);
    }

    lux_consensus_cleanup();
    return 0;
}

// 3. BLOCK MANAGEMENT TESTS
static int test_block_management_suite(void) {
    PRINT_HEADER("BLOCKS", "Add, Query, and Hierarchy");

    lux_consensus_init();

    lux_consensus_config_t config;
    create_default_config(&config);

    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);

    // Create block hierarchy
    uint8_t genesis_id[32] = {0};

    lux_block_t block1 = {0};
    memset(block1.id, 1, 32);
    memcpy(block1.parent_id, genesis_id, 32);
    block1.height = 1;
    block1.timestamp = (uint64_t)time(NULL);

    lux_block_t block2 = {0};
    memset(block2.id, 2, 32);
    memcpy(block2.parent_id, block1.id, 32);
    block2.height = 2;
    block2.timestamp = (uint64_t)time(NULL);

    // Test adding blocks
    lux_error_t err = lux_consensus_add_block(engine, &block1);
    ASSERT_TEST(err == LUX_SUCCESS, "Add block 1");

    err = lux_consensus_add_block(engine, &block2);
    ASSERT_TEST(err == LUX_SUCCESS, "Add block 2");

    // Test idempotency
    err = lux_consensus_add_block(engine, &block1);
    ASSERT_TEST(err == LUX_SUCCESS, "Add duplicate block (idempotent)");

    // Test with block data
    const char* data = "Important block data";
    lux_block_t block3 = {0};
    memset(block3.id, 3, 32);
    memcpy(block3.parent_id, block2.id, 32);
    block3.height = 3;
    block3.timestamp = (uint64_t)time(NULL);
    block3.data = (void*)data;
    block3.data_size = strlen(data);

    err = lux_consensus_add_block(engine, &block3);
    ASSERT_TEST(err == LUX_SUCCESS, "Add block with data");

    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    return 0;
}

// 4. VOTING TESTS
static int test_voting_suite(void) {
    PRINT_HEADER("VOTING", "Preference and Confidence");

    lux_consensus_init();

    lux_consensus_config_t config;
    create_default_config(&config);
    config.alpha_preference = 3;
    config.alpha_confidence = 3;
    config.beta = 5;

    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);

    // Add test block
    lux_block_t block = {0};
    memset(block.id, 0x0A, 32);
    memset(block.parent_id, 0, 32);
    block.height = 1;
    block.timestamp = (uint64_t)time(NULL);
    lux_consensus_add_block(engine, &block);

    // Test preference votes
    for (int i = 0; i < 3; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i, 32);
        memcpy(vote.block_id, block.id, 32);
        vote.is_preference = true;

        lux_error_t err = lux_consensus_process_vote(engine, &vote);
        char msg[100];
        snprintf(msg, sizeof(msg), "Process preference vote %d", i);
        ASSERT_TEST(err == LUX_SUCCESS, msg);
    }

    // Test confidence votes
    for (int i = 3; i < 6; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i, 32);
        memcpy(vote.block_id, block.id, 32);
        vote.is_preference = false;

        lux_error_t err = lux_consensus_process_vote(engine, &vote);
        char msg[100];
        snprintf(msg, sizeof(msg), "Process confidence vote %d", i);
        ASSERT_TEST(err == LUX_SUCCESS, msg);
    }

    // Check statistics
    lux_consensus_stats_t stats;
    lux_consensus_get_stats(engine, &stats);
    ASSERT_TEST(stats.votes_processed == 6, "Vote count tracking");

    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    return 0;
}

// 5. ACCEPTANCE TESTS
static int test_acceptance_suite(void) {
    PRINT_HEADER("ACCEPTANCE", "Decision Thresholds");

    lux_consensus_init();

    lux_consensus_config_t config;
    create_default_config(&config);
    config.alpha_preference = 2;
    config.alpha_confidence = 2;
    config.beta = 3;

    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);

    // Add competing blocks
    lux_block_t block_a = {0};
    memset(block_a.id, 0xAA, 32);
    memset(block_a.parent_id, 0, 32);
    block_a.height = 1;
    block_a.timestamp = (uint64_t)time(NULL);
    lux_consensus_add_block(engine, &block_a);

    lux_block_t block_b = {0};
    memset(block_b.id, 0xBB, 32);
    memset(block_b.parent_id, 0, 32);
    block_b.height = 1;
    block_b.timestamp = (uint64_t)time(NULL);
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
    ASSERT_TEST(is_accepted == true, "Block A accepted after threshold");

    is_accepted = false;
    lux_consensus_is_accepted(engine, block_b.id, &is_accepted);
    ASSERT_TEST(is_accepted == false, "Block B not accepted");

    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    return 0;
}

// 6. PREFERENCE TESTS
static int test_preference_suite(void) {
    PRINT_HEADER("PREFERENCE", "Preferred Block Selection");

    lux_consensus_init();

    lux_consensus_config_t config;
    create_default_config(&config);

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
    ASSERT_TEST(is_genesis, "Initial preference is genesis");

    // Add and accept a block
    lux_block_t block = {0};
    memset(block.id, 0xFF, 32);
    memset(block.parent_id, 0, 32);
    block.height = 1;
    block.timestamp = (uint64_t)time(NULL);
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
    ASSERT_TEST(memcmp(pref_id, block.id, 32) == 0, "Preference updated to accepted block");

    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    return 0;
}

// 7. ENGINE TYPE TESTS
static int test_engine_types_suite(void) {
    PRINT_HEADER("ENGINE TYPES", "Chain, DAG, PQ");

    lux_consensus_init();

    struct {
        lux_engine_type_t type;
        const char* name;
    } types[] = {
        {LUX_ENGINE_CHAIN, "Chain"},
        {LUX_ENGINE_DAG, "DAG"},
        {LUX_ENGINE_PQ, "PQ"},
    };

    for (size_t i = 0; i < sizeof(types)/sizeof(types[0]); i++) {
        lux_consensus_config_t config;
        create_default_config(&config);
        config.engine_type = types[i].type;

        lux_consensus_engine_t* engine = NULL;
        lux_error_t err = lux_consensus_engine_create(&engine, &config);

        char msg[100];
        snprintf(msg, sizeof(msg), "Create %s engine", types[i].name);
        ASSERT_TEST(err == LUX_SUCCESS, msg);

        const char* type_str = lux_engine_type_string(types[i].type);
        snprintf(msg, sizeof(msg), "Engine type string for %s", types[i].name);
        ASSERT_TEST(strcmp(type_str, types[i].name) == 0, msg);

        lux_consensus_engine_destroy(engine);
    }

    lux_consensus_cleanup();
    return 0;
}

// 8. PERFORMANCE TESTS
static int test_performance_suite(void) {
    PRINT_HEADER("PERFORMANCE", "Throughput and Latency");

    lux_consensus_init();

    lux_consensus_config_t config;
    create_default_config(&config);

    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);

    // Add 1000 blocks
    clock_t start = clock();

    for (int i = 0; i < 1000; i++) {
        lux_block_t block = {0};
        block.id[0] = (i >> 8) & 0xFF;
        block.id[1] = i & 0xFF;
        memset(block.parent_id, 0, 32);
        block.height = i;
        block.timestamp = (uint64_t)time(NULL);
        lux_consensus_add_block(engine, &block);
    }

    double elapsed = (double)(clock() - start) / CLOCKS_PER_SEC;
    char msg[200];
    snprintf(msg, sizeof(msg), "Add 1000 blocks in < 1 second (took %.3fs)", elapsed);
    ASSERT_TEST(elapsed < 1.0, msg);

    printf("  Time: %.3f seconds\n", elapsed);

    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    return 0;
}

// Main test runner
int main(void) {
    printf("%s=====================================%s\n", COLOR_YELLOW, COLOR_RESET);
    printf("%s=== LUX CONSENSUS C TEST SUITE ===%s\n", COLOR_YELLOW, COLOR_RESET);
    printf("%s=====================================%s\n\n", COLOR_YELLOW, COLOR_RESET);

    // Run all test suites
    typedef int (*test_suite_func)(void);
    test_suite_func test_suites[] = {
        test_initialization_suite,
        test_engine_creation_suite,
        test_block_management_suite,
        test_voting_suite,
        test_acceptance_suite,
        test_preference_suite,
        test_engine_types_suite,
        test_performance_suite,
    };

    for (size_t i = 0; i < sizeof(test_suites)/sizeof(test_suites[0]); i++) {
        if (test_suites[i]() != 0) {
            // Test suite failed, but continue running other suites
        }
    }

    // Print summary
    printf("\n%s=====================================%s\n", COLOR_YELLOW, COLOR_RESET);
    printf("%s=== TEST SUMMARY ===%s\n", COLOR_YELLOW, COLOR_RESET);
    printf("%s=====================================%s\n", COLOR_YELLOW, COLOR_RESET);

    int total_tests = results.passed + results.failed + results.skipped;
    printf("Total Tests: %d\n", total_tests);
    printf("%sPassed: %d%s\n", COLOR_GREEN, results.passed, COLOR_RESET);
    printf("%sFailed: %d%s\n", COLOR_RED, results.failed, COLOR_RESET);
    printf("%sSkipped: %d%s\n", COLOR_YELLOW, results.skipped, COLOR_RESET);

    if (results.failed == 0) {
        printf("\n%s🎉 ALL TESTS PASSED! 100%% SUCCESS RATE%s\n", COLOR_GREEN, COLOR_RESET);
        return 0;
    } else {
        printf("\n%s❌ SOME TESTS FAILED%s\n", COLOR_RED, COLOR_RESET);
        return 1;
    }
}
