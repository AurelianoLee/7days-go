package client

import "errors"

var ErrShutdown = errors.New("client: connection is shut down")
