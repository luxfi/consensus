// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package linked

// ListNode represents a node in a doubly linked list
type ListNode[T any] struct {
	Value T
	Next  *ListNode[T]
	Prev  *ListNode[T]
}

// List is a doubly linked list implementation
type List[T any] struct {
	head   *ListNode[T]
	tail   *ListNode[T]
	length int
}

// NewList creates a new doubly linked list
func NewList[T any]() *List[T] {
	return &List[T]{}
}

// Len returns the length of the list
func (l *List[T]) Len() int {
	return l.length
}

// Front returns the first node in the list
func (l *List[T]) Front() *ListNode[T] {
	return l.head
}

// Back returns the last node in the list
func (l *List[T]) Back() *ListNode[T] {
	return l.tail
}

// PushFront adds a value to the front of the list
func (l *List[T]) PushFront(value T) *ListNode[T] {
	node := &ListNode[T]{Value: value}
	if l.head == nil {
		l.head = node
		l.tail = node
	} else {
		node.Next = l.head
		l.head.Prev = node
		l.head = node
	}
	l.length++
	return node
}

// PushBack adds a value to the back of the list
func (l *List[T]) PushBack(value T) *ListNode[T] {
	node := &ListNode[T]{Value: value}
	if l.tail == nil {
		l.head = node
		l.tail = node
	} else {
		node.Prev = l.tail
		l.tail.Next = node
		l.tail = node
	}
	l.length++
	return node
}

// Remove removes a node from the list
func (l *List[T]) Remove(node *ListNode[T]) {
	if node == nil {
		return
	}
	
	if node.Prev != nil {
		node.Prev.Next = node.Next
	} else {
		l.head = node.Next
	}
	
	if node.Next != nil {
		node.Next.Prev = node.Prev
	} else {
		l.tail = node.Prev
	}
	
	node.Next = nil
	node.Prev = nil
	l.length--
}

// Clear removes all elements from the list
func (l *List[T]) Clear() {
	l.head = nil
	l.tail = nil
	l.length = 0
}