// Copyright 2025 Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package process

type ExecuteRequest struct {
	Command string `json:"command" validate:"required"`
	// Timeout in seconds, defaults to 10 seconds
	Timeout *uint32 `json:"timeout,omitempty" validate:"optional"`
	// Current working directory
	Cwd *string `json:"cwd,omitempty" validate:"optional"`
	// Enable TTY mode for interactive commands
	TTY *bool `json:"tty,omitempty" validate:"optional"`
} // @name ExecuteRequest

// TODO: Set ExitCode as required once all sandboxes migrated to the new daemon
type ExecuteResponse struct {
	ExitCode int    `json:"exitCode"`
	Result   string `json:"result" validate:"required"`
} // @name ExecuteResponse

// ExecuteTTYRequest represents a request to execute a command with TTY support
type ExecuteTTYRequest struct {
	Command string `json:"command" validate:"required"`
	// Arguments to pass to the command (optional, command can include args)
	Args    []string          `json:"args,omitempty" validate:"optional"`
	Cwd     *string           `json:"cwd,omitempty" validate:"optional"`
	Timeout *uint32           `json:"timeout,omitempty" validate:"optional"`
	Cols    *uint16           `json:"cols" validate:"optional"`
	Rows    *uint16           `json:"rows" validate:"optional"`
	Envs    map[string]string `json:"envs,omitempty" validate:"optional"`
} // @name ExecuteTTYRequest

// ExecuteTTYResponse is sent as a control message when connecting to TTY exec WebSocket
type ExecuteTTYResponse struct {
	SessionID string `json:"sessionId" validate:"required"`
} // @name ExecuteTTYResponse
