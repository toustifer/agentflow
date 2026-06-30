package server

import "errors"

var (
	ErrEngineRequired      = errors.New("engine required")
	ErrUnknownTool         = errors.New("unknown tool")
	ErrToolNotImplemented  = errors.New("tool not implemented")
	ErrInvalidToolInput    = errors.New("invalid tool input")
)
