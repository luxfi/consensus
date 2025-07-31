// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wrappers

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Errs is a collection of errors
type Errs struct {
	mu   sync.RWMutex
	errs []error
}

// Add adds an error to the collection
func (e *Errs) Add(err error) {
	if err == nil {
		return
	}
	
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errs = append(e.errs, err)
}

// Errored returns true if any errors have been added
func (e *Errs) Errored() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.errs) > 0
}

// Err returns the errors as a single error
func (e *Errs) Err() error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	switch len(e.errs) {
	case 0:
		return nil
	case 1:
		return e.errs[0]
	default:
		return errors.New(e.String())
	}
}

// String returns a string representation of all errors
func (e *Errs) String() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	if len(e.errs) == 0 {
		return ""
	}
	
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d error", len(e.errs)))
	if len(e.errs) != 1 {
		sb.WriteString("s")
	}
	sb.WriteString(" occurred:")
	
	for _, err := range e.errs {
		sb.WriteString("\n\t* ")
		sb.WriteString(err.Error())
	}
	
	return sb.String()
}

// Len returns the number of errors
func (e *Errs) Len() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.errs)
}

// Packer packs data into bytes
type Packer struct {
	Bytes []byte
	Err   error
}

// NewPacker returns a new Packer
func NewPacker(size int) *Packer {
	return &Packer{
		Bytes: make([]byte, 0, size),
	}
}

// PackByte packs a byte
func (p *Packer) PackByte(b byte) {
	if p.Err != nil {
		return
	}
	p.Bytes = append(p.Bytes, b)
}

// PackBytes packs bytes
func (p *Packer) PackBytes(bytes []byte) {
	if p.Err != nil {
		return
	}
	p.Bytes = append(p.Bytes, bytes...)
}

// PackInt packs an int as 4 bytes
func (p *Packer) PackInt(i uint32) {
	if p.Err != nil {
		return
	}
	p.Bytes = append(p.Bytes, byte(i>>24), byte(i>>16), byte(i>>8), byte(i))
}

// PackLong packs a long as 8 bytes
func (p *Packer) PackLong(l uint64) {
	if p.Err != nil {
		return
	}
	p.Bytes = append(p.Bytes, 
		byte(l>>56), byte(l>>48), byte(l>>40), byte(l>>32),
		byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
}