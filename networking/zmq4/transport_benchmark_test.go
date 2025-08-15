// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zmq4

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BenchmarkZMQTransportThroughput tests message throughput
func BenchmarkZMQTransportThroughput(b *testing.B) {
	ctx := context.Background()

	// Create server transport
	serverTransport := NewTransport(ctx, "server", 15555)
	defer serverTransport.Stop()

	// Create client transport
	clientTransport := NewTransport(ctx, "client", 15556)
	defer clientTransport.Stop()

	// Start transports
	err := serverTransport.Start()
	require.NoError(b, err)
	err = clientTransport.Start()
	require.NoError(b, err)

	// Set up message handler on server
	receivedCount := 0
	serverTransport.RegisterHandler("benchmark", func(msg *Message) {
		receivedCount++
	})

	// Connect client to server
	_ = clientTransport.ConnectPeer("server", 15555)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Message to send
	msg := &Message{
		Type:      "benchmark",
		From:      "client",
		To:        "server",
		Data:      json.RawMessage(`"Hello, ZeroMQ benchmark test message!"`),
		Timestamp: time.Now().Unix(),
	}

	b.ResetTimer()
	b.SetBytes(int64(len(msg.Data)))

	// Benchmark sending messages
	for i := 0; i < b.N; i++ {
		err := clientTransport.Send("server", msg)
		if err != nil {
			b.Fatal(err)
		}
	}

	// Give some time for messages to be received
	time.Sleep(100 * time.Millisecond)
	b.Logf("Sent: %d, Received: %d", b.N, receivedCount)
}

// BenchmarkZMQTransportLatency tests round-trip latency
func BenchmarkZMQTransportLatency(b *testing.B) {
	ctx := context.Background()

	// Create server transport
	serverTransport := NewTransport(ctx, "server", 15557)
	defer serverTransport.Stop()

	// Create client transport
	clientTransport := NewTransport(ctx, "client", 15558)
	defer clientTransport.Stop()

	// Start transports
	err := serverTransport.Start()
	require.NoError(b, err)
	err = clientTransport.Start()
	require.NoError(b, err)

	// Echo server handler
	serverTransport.RegisterHandler("ping", func(msg *Message) {
		reply := &Message{
			Type:      "pong",
			From:      "server",
			To:        msg.From,
			Data:      msg.Data,
			Timestamp: time.Now().Unix(),
		}
		_ = serverTransport.Send(msg.From, reply)
	})

	// Response channel for client
	responses := make(chan *Message, 1)
	clientTransport.RegisterHandler("pong", func(msg *Message) {
		responses <- msg
	})

	// Connect bidirectionally
	_ = clientTransport.ConnectPeer("server", 15557)
	_ = serverTransport.ConnectPeer("client", 15558)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	msg := &Message{
		Type:      "ping",
		From:      "client",
		To:        "server",
		Data:      json.RawMessage(`"Latency test message"`),
		Timestamp: time.Now().Unix(),
	}

	b.ResetTimer()

	// Benchmark round-trip latency
	for i := 0; i < b.N; i++ {
		err := clientTransport.Send("server", msg)
		if err != nil {
			b.Fatal(err)
		}

		select {
		case <-responses:
			// Got response
		case <-time.After(time.Second):
			b.Fatal("timeout waiting for response")
		}
	}
}

// BenchmarkZMQTransportConcurrent tests concurrent message handling
func BenchmarkZMQTransportConcurrent(b *testing.B) {
	ctx := context.Background()

	// Create server transport
	serverTransport := NewTransport(ctx, "server", 15559)
	defer serverTransport.Stop()

	// Start server
	err := serverTransport.Start()
	require.NoError(b, err)

	// Message counter
	var received int64
	serverTransport.RegisterHandler("concurrent", func(msg *Message) {
		received++
	})

	// Message to send
	msgData := json.RawMessage(`"Concurrent benchmark test message!"`)

	b.ResetTimer()
	b.SetBytes(int64(len(msgData)))

	// Run concurrent senders
	b.RunParallel(func(pb *testing.PB) {
		// Create client transport for this goroutine
		clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
		clientTransport := NewTransport(ctx, clientID, 0) // Use dynamic port
		defer clientTransport.Stop()

		err := clientTransport.Start()
		if err != nil {
			b.Fatal(err)
		}

		_ = clientTransport.ConnectPeer("server", 15559)
		time.Sleep(50 * time.Millisecond) // Wait for connection

		msg := &Message{
			Type:      "concurrent",
			From:      clientID,
			To:        "server",
			Data:      msgData,
			Timestamp: time.Now().Unix(),
		}

		for pb.Next() {
			err := clientTransport.Send("server", msg)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// Give some time for messages to be received
	time.Sleep(200 * time.Millisecond)
	b.Logf("Total messages received: %d", received)
}

// BenchmarkZMQTransportLargeMessages tests performance with large messages
func BenchmarkZMQTransportLargeMessages(b *testing.B) {
	sizes := []int{
		1024,    // 1KB
		10240,   // 10KB
		102400,  // 100KB
		1048576, // 1MB
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			ctx := context.Background()

			// Create server transport
			serverTransport := NewTransport(ctx, "server", 15560+size/1024)
			defer serverTransport.Stop()

			// Create client transport
			clientTransport := NewTransport(ctx, "client", 15660+size/1024)
			defer clientTransport.Stop()

			// Start transports
			err := serverTransport.Start()
			require.NoError(b, err)
			err = clientTransport.Start()
			require.NoError(b, err)

			// Message handler
			serverTransport.RegisterHandler("large", func(msg *Message) {
				// Just receive
			})

			// Connect
			_ = clientTransport.ConnectPeer("server", 15560+size/1024)

			// Create large message data
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}
			jsonData, _ := json.Marshal(string(data))

			// Wait for connection
			time.Sleep(100 * time.Millisecond)

			msg := &Message{
				Type:      "large",
				From:      "client",
				To:        "server",
				Data:      jsonData,
				Timestamp: time.Now().Unix(),
			}

			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				err := clientTransport.Send("server", msg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkZMQTransportBroadcast tests broadcast performance
func BenchmarkZMQTransportBroadcast(b *testing.B) {
	numReceivers := []int{5, 10, 20}

	for _, n := range numReceivers {
		b.Run(fmt.Sprintf("receivers-%d", n), func(b *testing.B) {
			ctx := context.Background()

			// Create publisher
			publisher := NewTransport(ctx, "publisher", 15600)
			defer publisher.Stop()

			err := publisher.Start()
			require.NoError(b, err)

			// Create receivers
			var receivers []*Transport
			var wg sync.WaitGroup

			for i := 0; i < n; i++ {
				receiverID := fmt.Sprintf("receiver-%d", i)
				receiver := NewTransport(ctx, receiverID, 15700+i)
				defer receiver.Stop()

				err := receiver.Start()
				require.NoError(b, err)

				_ = receiver.ConnectPeer("publisher", 15600)
				_ = publisher.ConnectPeer(receiverID, 15700+i)

				receivers = append(receivers, receiver)

				// Start receiver handler
				received := 0
				receiver.RegisterHandler("broadcast", func(msg *Message) {
					received++
				})
			}

			// Wait for connections
			time.Sleep(200 * time.Millisecond)

			msgData := json.RawMessage(`"Broadcast benchmark message"`)

			b.ResetTimer()
			b.SetBytes(int64(len(msgData) * n)) // Total bytes sent to all receivers

			for i := 0; i < b.N; i++ {
				// Broadcast to all receivers
				for _, receiver := range receivers {
					msg := &Message{
						Type:      "broadcast",
						From:      "publisher",
						To:        receiver.nodeID,
						Data:      msgData,
						Timestamp: time.Now().Unix(),
					}
					err := publisher.Send(receiver.nodeID, msg)
					if err != nil {
						b.Fatal(err)
					}
				}
			}

			// Give time for messages to be received
			time.Sleep(100 * time.Millisecond)
			wg.Wait()
		})
	}
}

// BenchmarkZMQTransportSetup tests transport creation and setup overhead
func BenchmarkZMQTransportSetup(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create and start transport
		transport := NewTransport(ctx, fmt.Sprintf("node-%d", i), 0)

		err := transport.Start()
		if err != nil {
			b.Fatal(err)
		}

		// Add a peer
		_ = transport.ConnectPeer("peer", 16000)

		// Close
		transport.Stop()
	}
}

// Helper function to create test message
func createTestMessage(from, to, msgType string, data interface{}) *Message {
	jsonData, _ := json.Marshal(data)
	return &Message{
		Type:      msgType,
		From:      from,
		To:        to,
		Data:      jsonData,
		Timestamp: time.Now().Unix(),
	}
}
