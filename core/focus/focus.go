package focus

// optional separate β accounting if you keep it outside wave; stub for now.
type Focus[ID comparable] struct{}
func New[ID comparable]() *Focus[ID] { return &Focus[ID]{} }
