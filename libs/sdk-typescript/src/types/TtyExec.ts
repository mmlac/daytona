/*
 * Copyright 2025 Daytona Platforms Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * Options for executing a command with TTY support.
 */
export interface TtyExecOptions {
  /**
   * Command to execute
   */
  command: string

  /**
   * Command arguments
   */
  args?: string[]

  /**
   * Working directory for the command. Defaults to the sandbox's working directory.
   */
  cwd?: string

  /**
   * Maximum time in seconds to wait for the command to complete. 0 means wait indefinitely.
   */
  timeout?: number

  /**
   * Number of terminal columns
   */
  cols?: number

  /**
   * Number of terminal rows
   */
  rows?: number

  /**
   * Environment variables for the command
   */
  envs?: Record<string, string>

  /**
   * Callback to handle terminal output data. If omitted, output is silently
   * discarded (useful for headless / CI use cases that only care about the
   * exit code).
   */
  onData?: (data: Uint8Array) => void | Promise<void>
}
