# Tests CMakeLists.txt
add_executable(test_consensus test_consensus.cpp)

# Link against lux_consensus library (no ZMQ needed for tests)
target_link_libraries(test_consensus PRIVATE lux_consensus Threads::Threads)

# Don't link ZMQ for tests (library handles it internally)
# target_link_libraries(test_consensus PRIVATE ${ZEROMQ_LIBRARIES})

add_test(NAME consensus_test COMMAND test_consensus)