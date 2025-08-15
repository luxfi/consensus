package tracker

// Tracker tracks consensus progress
type Tracker interface {
	IsProcessing(id interface{}) bool
	Add(id interface{})
	Remove(id interface{})
}
