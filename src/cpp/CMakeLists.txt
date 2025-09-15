cmake_minimum_required(VERSION 3.20)
project(lux_consensus_cpp VERSION 1.0.0 LANGUAGES CXX)

# Set C++ standard
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Find required packages
find_package(Threads REQUIRED)
find_package(PkgConfig REQUIRED)
pkg_check_modules(ZEROMQ REQUIRED libzmq)

# Optional: Find MLX for GPU acceleration
find_package(MLX QUIET)

# Library sources
set(SOURCES
    src/consensus.cpp
    src/snowball.cpp
    src/avalanche.cpp
    src/network.cpp
    src/vote_processor.cpp
)

# Create library
add_library(lux_consensus STATIC ${SOURCES})

# Include directories
target_include_directories(lux_consensus
    PUBLIC
        $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
        $<INSTALL_INTERFACE:include>
    PRIVATE
        ${ZEROMQ_INCLUDE_DIRS}
)

# Link libraries
target_link_libraries(lux_consensus
    PUBLIC
        Threads::Threads
    PRIVATE
        ${ZEROMQ_LIBRARIES}
)

# Compile options
target_compile_options(lux_consensus PRIVATE
    -Wall -Wextra -Wpedantic
    $<$<CONFIG:Release>:-O3 -march=native>
    $<$<CONFIG:Debug>:-g -fsanitize=address>
)

# MLX support
if(MLX_FOUND)
    target_compile_definitions(lux_consensus PUBLIC HAS_MLX)
    target_link_libraries(lux_consensus PRIVATE MLX::MLX)
endif()

# Build examples
add_subdirectory(examples)

# Build tests
enable_testing()
add_subdirectory(tests)

# Installation
install(TARGETS lux_consensus
    EXPORT lux_consensus_targets
    LIBRARY DESTINATION lib
    ARCHIVE DESTINATION lib
)

install(DIRECTORY include/
    DESTINATION include
)

install(EXPORT lux_consensus_targets
    FILE lux_consensus_config.cmake
    NAMESPACE Lux::
    DESTINATION lib/cmake/lux_consensus
)