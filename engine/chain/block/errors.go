package block

import "errors"

// ErrRemoteVMNotImplemented is returned when the remote VM is not implemented
var ErrRemoteVMNotImplemented = errors.New("remote VM not implemented")