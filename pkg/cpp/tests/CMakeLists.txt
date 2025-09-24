# Tests CMakeLists.txt
add_executable(test_consensus test_consensus.cpp)
target_link_libraries(test_consensus PRIVATE lux_consensus)
add_test(NAME consensus_test COMMAND test_consensus)