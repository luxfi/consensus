package main

import (
	"encoding/binary"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/luxfi/geth/common"
)

func main() {
	dbPath := "/home/z/.lux-cli/mainnet/chainData/network-96369/4aYc2FXx3EDKf98wqmxaRkkLERa7QSbbNnKRL7awjHqVqGgxj/db/ethdb"

	db, err := badger.Open(badger.DefaultOptions(dbPath).WithLogger(nil))
	if err != nil {
		panic(err)
	}
	defer db.Close()

	tipHeight := uint64(1082780)

	// First, read the canonical hash at tip height
	var actualTipHash common.Hash
	err = db.View(func(txn *badger.Txn) error {
		var heightBytes [8]byte
		binary.BigEndian.PutUint64(heightBytes[:], tipHeight)
		canonKey := []byte{0x68} // 'h'
		canonKey = append(canonKey, heightBytes[:]...)
		canonKey = append(canonKey, 0x6e) // 'n'

		item, err := txn.Get(canonKey)
		if err != nil {
			return fmt.Errorf("canonical hash at tip not found: %v", err)
		}

		return item.Value(func(val []byte) error {
			copy(actualTipHash[:], val)
			return nil
		})
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Actual canonical tip hash: 0x%x\n", actualTipHash)

	// Now update head pointers to use the actual tip
	txn := db.NewTransaction(true)
	defer txn.Discard()

	txn.Set([]byte("LastBlock"), actualTipHash[:])
	txn.Set([]byte("LastHeader"), actualTipHash[:])
	txn.Set([]byte("LastFast"), actualTipHash[:])
	txn.Set([]byte("LastFinalized"), actualTipHash[:])
	txn.Set([]byte("LastSafe"), actualTipHash[:])

	if err := txn.Commit(); err != nil {
		panic(err)
	}

	fmt.Printf("✅ Set all head pointers to actual TIP: 0x%x\n", actualTipHash)
}
