package timeout

import "time"

// Manager manages request timeouts
type Manager interface {
    RegisterRequest(requestID uint32, timeout time.Duration)
    RemoveRequest(requestID uint32)
}
