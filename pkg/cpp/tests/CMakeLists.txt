# Tests CMakeLists.txt
add_executable(test_consensus test_consensus.cpp)
add_executable(test_all_consensus test_all_consensus.cpp)
add_executable(test_proven_features test_proven_features.cpp)

# Link against lux_consensus library (no ZMQ needed for tests)
target_link_libraries(test_consensus PRIVATE lux_consensus Threads::Threads)
target_link_libraries(test_all_consensus PRIVATE lux_consensus Threads::Threads)
target_link_libraries(test_proven_features PRIVATE lux_consensus Threads::Threads)

# Don't link ZMQ for tests (library handles it internally)
# target_link_libraries(test_consensus PRIVATE ${ZEROMQ_LIBRARIES})

add_test(NAME consensus_test COMMAND test_consensus)
add_test(NAME all_consensus_test COMMAND test_all_consensus)
add_test(NAME proven_features_test COMMAND test_proven_features)