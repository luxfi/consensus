package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
)

func main() {
	vmPath := "/home/z/.lux-cli/mainnet/chainData/network-96369/4aYc2FXx3EDKf98wqmxaRkkLERa7QSbbNnKRL7awjHqVqGgxj/db/vm"
	
	// The actual canonical tip from the database
	tipHashStr := "899b9fe03408bf9110e9ebddf136f8749cf9fbd58e45ca345b99976826718083"
	tipHashBytes, _ := hex.DecodeString(tipHashStr)
	tipHeight := uint64(1082780)

	db, err := badger.Open(badger.DefaultOptions(vmPath).WithLogger(nil))
	if err != nil {
		panic(err)
	}
	defer db.Close()

	txn := db.NewTransaction(true)
	defer txn.Discard()

	// Write lastAccepted
	txn.Set([]byte("lastAccepted"), tipHashBytes)

	// Write lastAcceptedHeight
	var heightBytes [8]byte
	binary.BigEndian.PutUint64(heightBytes[:], tipHeight)
	txn.Set([]byte("lastAcceptedHeight"), heightBytes[:])

	// Write initialized flag
	txn.Set([]byte("initialized"), []byte{1})

	if err := txn.Commit(); err != nil {
		panic(err)
	}

	fmt.Printf("✅ VM metadata updated:\n")
	fmt.Printf("   lastAccepted: 0x%s\n", tipHashStr)
	fmt.Printf("   lastAcceptedHeight: %d\n", tipHeight)
	fmt.Printf("   initialized: true\n")
}
