// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

#include "../include/lux_consensus.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <pthread.h>
#include <time.h>

// Internal data structures optimized for C

typedef struct block_node {
    lux_block_t block;
    struct block_node* parent;
    struct block_node** children;
    size_t children_count;
    size_t children_capacity;
    
    // Consensus state
    uint32_t preference_count;
    uint32_t confidence_count;
    bool is_accepted;
    bool is_rejected;
    bool is_processing;
    
    // Performance optimization
    uint64_t last_poll_time;
    uint32_t poll_count;
} block_node_t;

typedef struct vote_cache {
    uint8_t voter_id[32];
    uint8_t block_id[32];
    uint64_t timestamp;
    struct vote_cache* next;
} vote_cache_t;

// Hash table for fast block lookup
#define HASH_TABLE_SIZE 1024

typedef struct hash_entry {
    uint8_t block_id[32];
    block_node_t* node;
    struct hash_entry* next;
} hash_entry_t;

struct lux_chain {
    lux_config_t config;
    
    // Block storage
    hash_entry_t* block_table[HASH_TABLE_SIZE];
    block_node_t* genesis_block;
    block_node_t* preferred_block;
    
    // Vote tracking
    vote_cache_t* vote_cache;
    size_t vote_cache_size;
    
    // Thread safety
    pthread_mutex_t mutex;
    pthread_rwlock_t rwlock;
    
    // Callbacks
    lux_callback_decision decision_callback;
    lux_callback_verify verify_callback;
    lux_callback_notify notify_callback;
    void* callback_user_data;
    
    // Statistics
    lux_consensus_stats_t stats;
    uint64_t start_time;
};

// Fast hash function for block IDs
static uint32_t hash_block_id(const uint8_t* block_id) {
    uint32_t hash = 5381;
    for (int i = 0; i < 32; i++) {
        hash = ((hash << 5) + hash) + block_id[i];
    }
    return hash % HASH_TABLE_SIZE;
}

// Find block in hash table
static block_node_t* find_block(lux_chain_t* engine, const uint8_t* block_id) {
    uint32_t index = hash_block_id(block_id);
    hash_entry_t* entry = engine->block_table[index];
    
    while (entry) {
        if (memcmp(entry->block_id, block_id, 32) == 0) {
            return entry->node;
        }
        entry = entry->next;
    }
    return NULL;
}

// Add block to hash table
static lux_error_t add_block_to_table(lux_chain_t* engine, block_node_t* node) {
    uint32_t index = hash_block_id(node->block.id);
    
    hash_entry_t* new_entry = (hash_entry_t*)calloc(1, sizeof(hash_entry_t));
    if (!new_entry) return LUX_ERROR_OUT_OF_MEMORY;
    
    memcpy(new_entry->block_id, node->block.id, 32);
    new_entry->node = node;
    new_entry->next = engine->block_table[index];
    engine->block_table[index] = new_entry;
    
    return LUX_SUCCESS;
}

// Lux Consensus algorithm implementation
static bool check_confidence(lux_chain_t* engine, block_node_t* node) {
    return node->confidence_count >= engine->config.alpha;
}

static bool check_preference(lux_chain_t* engine, block_node_t* node) {
    return node->preference_count >= engine->config.alpha;
}

static bool check_decision_threshold(lux_chain_t* engine, block_node_t* node) {
    return node->confidence_count >= engine->config.beta;
}

// Process consensus decision
static void process_decision(lux_chain_t* engine, block_node_t* node) {
    if (node->is_accepted || node->is_rejected) {
        return;
    }
    
    if (check_decision_threshold(engine, node)) {
        node->is_accepted = true;
        engine->stats.blocks_accepted++;
        
        // Update preferred block
        engine->preferred_block = node;
        
        // Notify via callback
        if (engine->decision_callback) {
            engine->decision_callback(node->block.id, engine->callback_user_data);
        }
        
        // Reject conflicting blocks
        for (size_t i = 0; i < node->parent->children_count; i++) {
            block_node_t* sibling = node->parent->children[i];
            if (sibling != node && !sibling->is_rejected) {
                sibling->is_rejected = true;
                engine->stats.blocks_rejected++;
            }
        }
    }
}

// Library initialization
lux_error_t lux_consensus_init(void) {
    // Initialize any global state if needed
    return LUX_SUCCESS;
}

lux_error_t lux_consensus_cleanup(void) {
    // Cleanup global state
    return LUX_SUCCESS;
}

// Engine creation and destruction
lux_error_t lux_consensus_engine_create(
    lux_chain_t** engine_ptr,
    const lux_config_t* config
) {
    if (!engine_ptr || !config) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    lux_chain_t* engine = (lux_chain_t*)calloc(1, sizeof(lux_chain_t));
    if (!engine) {
        return LUX_ERROR_OUT_OF_MEMORY;
    }
    
    // Copy configuration
    memcpy(&engine->config, config, sizeof(lux_config_t));
    
    // Initialize synchronization
    pthread_mutex_init(&engine->mutex, NULL);
    pthread_rwlock_init(&engine->rwlock, NULL);
    
    // Initialize statistics
    engine->start_time = (uint64_t)time(NULL);
    
    // Create genesis block
    engine->genesis_block = (block_node_t*)calloc(1, sizeof(block_node_t));
    if (!engine->genesis_block) {
        free(engine);
        return LUX_ERROR_OUT_OF_MEMORY;
    }
    
    // Genesis block is always accepted
    engine->genesis_block->is_accepted = true;
    engine->preferred_block = engine->genesis_block;
    
    *engine_ptr = engine;
    return LUX_SUCCESS;
}

lux_error_t lux_consensus_engine_destroy(lux_chain_t* engine) {
    if (!engine) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    pthread_mutex_lock(&engine->mutex);
    
    // Clean up hash table
    for (int i = 0; i < HASH_TABLE_SIZE; i++) {
        hash_entry_t* entry = engine->block_table[i];
        while (entry) {
            hash_entry_t* next = entry->next;
            free(entry);
            entry = next;
        }
    }
    
    // Clean up vote cache
    vote_cache_t* vote = engine->vote_cache;
    while (vote) {
        vote_cache_t* next = vote->next;
        free(vote);
        vote = next;
    }
    
    pthread_mutex_unlock(&engine->mutex);
    pthread_mutex_destroy(&engine->mutex);
    pthread_rwlock_destroy(&engine->rwlock);
    
    free(engine);
    return LUX_SUCCESS;
}

// Block operations
lux_error_t lux_consensus_add_block(
    lux_chain_t* engine,
    const lux_block_t* block
) {
    if (!engine || !block) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    pthread_mutex_lock(&engine->mutex);
    
    // Check if block already exists
    if (find_block(engine, block->id)) {
        pthread_mutex_unlock(&engine->mutex);
        return LUX_SUCCESS;
    }
    
    // Verify block if callback is set
    if (engine->verify_callback) {
        if (!engine->verify_callback(block, engine->callback_user_data)) {
            pthread_mutex_unlock(&engine->mutex);
            return LUX_ERROR_CONSENSUS_FAILED;
        }
    }
    
    // Create new block node
    block_node_t* node = (block_node_t*)calloc(1, sizeof(block_node_t));
    if (!node) {
        pthread_mutex_unlock(&engine->mutex);
        return LUX_ERROR_OUT_OF_MEMORY;
    }
    
    // Copy block data
    memcpy(&node->block, block, sizeof(lux_block_t));
    if (block->data_size > 0 && block->data) {
        node->block.data = malloc(block->data_size);
        if (!node->block.data) {
            free(node);
            pthread_mutex_unlock(&engine->mutex);
            return LUX_ERROR_OUT_OF_MEMORY;
        }
        memcpy(node->block.data, block->data, block->data_size);
    }
    
    // Find parent
    node->parent = find_block(engine, block->parent_id);
    if (!node->parent) {
        node->parent = engine->genesis_block;
    }
    
    // Add to parent's children
    if (node->parent->children_count >= node->parent->children_capacity) {
        size_t new_capacity = node->parent->children_capacity ? 
                             node->parent->children_capacity * 2 : 4;
        block_node_t** new_children = (block_node_t**)realloc(
            node->parent->children,
            new_capacity * sizeof(block_node_t*)
        );
        if (!new_children) {
            free(node->block.data);
            free(node);
            pthread_mutex_unlock(&engine->mutex);
            return LUX_ERROR_OUT_OF_MEMORY;
        }
        node->parent->children = new_children;
        node->parent->children_capacity = new_capacity;
    }
    node->parent->children[node->parent->children_count++] = node;
    
    // Add to hash table
    lux_error_t err = add_block_to_table(engine, node);
    if (err != LUX_SUCCESS) {
        free(node->block.data);
        free(node);
        pthread_mutex_unlock(&engine->mutex);
        return err;
    }
    
    pthread_mutex_unlock(&engine->mutex);
    return LUX_SUCCESS;
}

// Vote processing
lux_error_t lux_consensus_process_vote(
    lux_chain_t* engine,
    const lux_vote_t* vote
) {
    if (!engine || !vote) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    pthread_mutex_lock(&engine->mutex);
    
    block_node_t* node = find_block(engine, vote->block_id);
    if (!node) {
        pthread_mutex_unlock(&engine->mutex);
        return LUX_ERROR_INVALID_STATE;
    }
    
    // Update vote counts
    if (vote->is_preference) {
        node->preference_count++;
    } else {
        node->confidence_count++;
    }
    
    // Cache vote for analytics
    vote_cache_t* cached_vote = (vote_cache_t*)malloc(sizeof(vote_cache_t));
    if (cached_vote) {
        memcpy(cached_vote->voter_id, vote->voter_id, 32);
        memcpy(cached_vote->block_id, vote->block_id, 32);
        cached_vote->timestamp = (uint64_t)time(NULL);
        cached_vote->next = engine->vote_cache;
        engine->vote_cache = cached_vote;
        engine->vote_cache_size++;
        
        // Limit cache size
        if (engine->vote_cache_size > 10000) {
            vote_cache_t* old = engine->vote_cache;
            while (old->next && old->next->next) {
                old = old->next;
            }
            if (old->next) {
                free(old->next);
                old->next = NULL;
                engine->vote_cache_size--;
            }
        }
    }
    
    engine->stats.votes_processed++;
    
    // Check for consensus decision
    process_decision(engine, node);
    
    pthread_mutex_unlock(&engine->mutex);
    return LUX_SUCCESS;
}

// Query operations
lux_error_t lux_consensus_is_accepted(
    lux_chain_t* engine,
    const uint8_t* block_id,
    bool* is_accepted
) {
    if (!engine || !block_id || !is_accepted) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    pthread_rwlock_rdlock(&engine->rwlock);
    
    block_node_t* node = find_block(engine, block_id);
    if (!node) {
        pthread_rwlock_unlock(&engine->rwlock);
        return LUX_ERROR_INVALID_STATE;
    }
    
    *is_accepted = node->is_accepted;
    
    pthread_rwlock_unlock(&engine->rwlock);
    return LUX_SUCCESS;
}

lux_error_t lux_consensus_get_preference(
    lux_chain_t* engine,
    uint8_t* block_id
) {
    if (!engine || !block_id) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    pthread_rwlock_rdlock(&engine->rwlock);
    
    if (engine->preferred_block) {
        memcpy(block_id, engine->preferred_block->block.id, 32);
    } else {
        memset(block_id, 0, 32);
    }
    
    pthread_rwlock_unlock(&engine->rwlock);
    return LUX_SUCCESS;
}

// Polling
lux_error_t lux_consensus_poll(
    lux_chain_t* engine,
    uint32_t num_validators,
    const uint8_t** validator_ids
) {
    if (!engine || !validator_ids) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    pthread_mutex_lock(&engine->mutex);
    
    // Simulate polling validators (in real implementation, would do network calls)
    // For now, just increment poll count
    engine->stats.polls_completed++;
    
    pthread_mutex_unlock(&engine->mutex);
    return LUX_SUCCESS;
}

// Callback registration
lux_error_t lux_consensus_register_decision_callback(
    lux_chain_t* engine,
    lux_callback_decision callback,
    void* user_data
) {
    if (!engine) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    pthread_mutex_lock(&engine->mutex);
    engine->decision_callback = callback;
    engine->callback_user_data = user_data;
    pthread_mutex_unlock(&engine->mutex);
    
    return LUX_SUCCESS;
}

lux_error_t lux_consensus_register_verify_callback(
    lux_chain_t* engine,
    lux_callback_verify callback,
    void* user_data
) {
    if (!engine) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    pthread_mutex_lock(&engine->mutex);
    engine->verify_callback = callback;
    engine->callback_user_data = user_data;
    pthread_mutex_unlock(&engine->mutex);
    
    return LUX_SUCCESS;
}

lux_error_t lux_consensus_register_notify_callback(
    lux_chain_t* engine,
    lux_callback_notify callback,
    void* user_data
) {
    if (!engine) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    pthread_mutex_lock(&engine->mutex);
    engine->notify_callback = callback;
    engine->callback_user_data = user_data;
    pthread_mutex_unlock(&engine->mutex);
    
    return LUX_SUCCESS;
}

// Statistics
lux_error_t lux_consensus_get_stats(
    lux_chain_t* engine,
    lux_consensus_stats_t* stats
) {
    if (!engine || !stats) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    pthread_rwlock_rdlock(&engine->rwlock);
    memcpy(stats, &engine->stats, sizeof(lux_consensus_stats_t));
    
    // Calculate average decision time
    uint64_t current_time = (uint64_t)time(NULL);
    uint64_t elapsed = current_time - engine->start_time;
    if (engine->stats.blocks_accepted > 0) {
        stats->average_decision_time_ms = (double)(elapsed * 1000) / engine->stats.blocks_accepted;
    }
    
    pthread_rwlock_unlock(&engine->rwlock);
    return LUX_SUCCESS;
}

// Utility functions
const char* lux_error_string(lux_error_t error) {
    switch (error) {
        case LUX_SUCCESS: return "Success";
        case LUX_ERROR_INVALID_PARAMS: return "Invalid parameters";
        case LUX_ERROR_OUT_OF_MEMORY: return "Out of memory";
        case LUX_ERROR_INVALID_STATE: return "Invalid state";
        case LUX_ERROR_CONSENSUS_FAILED: return "Consensus failed";
        case LUX_ERROR_NOT_IMPLEMENTED: return "Not implemented";
        default: return "Unknown error";
    }
}

// New v1.22.0 simplified API functions

lux_chain_t* lux_chain_new_default(void) {
    lux_config_t config = {
        .node_count = 1,
        .k = 1,
        .alpha = 1,
        .beta = 1
    };
    return lux_chain_new(&config);
}

lux_chain_t* lux_chain_new(const lux_config_t* config) {
    if (!config) {
        return NULL;
    }
    
    lux_chain_t* chain = (lux_chain_t*)calloc(1, sizeof(lux_chain_t));
    if (!chain) {
        return NULL;
    }
    
    // Copy config and set auto-calculated parameters
    chain->config.node_count = config->node_count;
    chain->config.k = config->k > 0 ? config->k : (config->node_count > 1 ? config->node_count / 2 : 1);
    chain->config.alpha = config->alpha > 0 ? config->alpha : (config->node_count > 1 ? (config->node_count * 2) / 3 : 1);
    chain->config.beta = config->beta > 0 ? config->beta : (config->node_count > 2 ? config->node_count - 2 : 1);
    
    // Initialize mutexes
    pthread_mutex_init(&chain->mutex, NULL);
    pthread_rwlock_init(&chain->rwlock, NULL);
    
    chain->start_time = (uint64_t)time(NULL);
    
    // Note: block_table is already a static array in the struct, not allocated
    
    return chain;
}

void lux_chain_destroy(lux_chain_t* chain) {
    if (!chain) {
        return;
    }
    
    // Free blocks in table
    for (size_t i = 0; i < HASH_TABLE_SIZE; i++) {
        hash_entry_t* entry = chain->block_table[i];
        while (entry) {
            hash_entry_t* next = entry->next;
            if (entry->node) {
                free(entry->node->children);
                free(entry->node);
            }
            free(entry);
            entry = next;
        }
    }
    
    // Free vote cache
    vote_cache_t* vote = chain->vote_cache;
    while (vote) {
        vote_cache_t* next = vote->next;
        free(vote);
        vote = next;
    }
    
    // Destroy mutexes
    pthread_mutex_destroy(&chain->mutex);
    pthread_rwlock_destroy(&chain->rwlock);
    
    free(chain);
}

lux_error_t lux_chain_start(lux_chain_t* chain) {
    if (!chain) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    // Mark as started (chain is always "running" in simple mode)
    chain->start_time = (uint64_t)time(NULL);
    
    return LUX_SUCCESS;
}

void lux_chain_stop(lux_chain_t* chain) {
    // No-op in simple implementation
    // Chain can be stopped by destroying it
    (void)chain;
}

lux_error_t lux_chain_add_block(lux_chain_t* chain, const lux_block_t* block) {
    if (!chain || !block) {
        return LUX_ERROR_INVALID_PARAMS;
    }
    
    // Create block node
    block_node_t* node = (block_node_t*)calloc(1, sizeof(block_node_t));
    if (!node) {
        return LUX_ERROR_OUT_OF_MEMORY;
    }
    
    // Copy block data
    memcpy(&node->block, block, sizeof(lux_block_t));
    node->is_processing = true;
    
    // Add to hash table
    lux_error_t err = add_block_to_table(chain, node);
    if (err != LUX_SUCCESS) {
        free(node);
        return err;
    }
    
    // Update stats
    pthread_rwlock_wrlock(&chain->rwlock);
    chain->stats.votes_processed++;
    chain->stats.blocks_accepted++;
    pthread_rwlock_unlock(&chain->rwlock);
    
    // Mark as accepted (simplified consensus)
    node->is_accepted = true;
    node->is_processing = false;
    
    // Trigger callback if set
    if (chain->decision_callback) {
        chain->decision_callback(block->id, chain->callback_user_data);
    }
    
    return LUX_SUCCESS;
}
