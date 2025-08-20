package set

// Set represents a set of unique elements
type Set[T comparable] map[T]struct{}

// Of creates a new set with the given elements
func Of[T comparable](elements ...T) Set[T] {
    s := make(Set[T], len(elements))
    for _, e := range elements {
        s[e] = struct{}{}
    }
    return s
}

// Add adds an element to the set
func (s Set[T]) Add(element T) {
    s[element] = struct{}{}
}

// Contains checks if an element is in the set
func (s Set[T]) Contains(element T) bool {
    _, exists := s[element]
    return exists
}

// Remove removes an element from the set
func (s Set[T]) Remove(element T) {
    delete(s, element)
}

// Len returns the number of elements in the set
func (s Set[T]) Len() int {
    return len(s)
}

// List returns a slice of all elements in the set
func (s Set[T]) List() []T {
    result := make([]T, 0, len(s))
    for element := range s {
        result = append(result, element)
    }
    return result
}
