// Package chain implements linear consensus via the Snowman protocol.
//
// Chain handles sequential blocks in a linear topology, where each block
// extends exactly one parent. The consensus pipeline flows:
// photon → wave → focus → nova, terminating in single-threaded finality.
// This is the classical blockchain model: one block, one parent, one chain.
package chain