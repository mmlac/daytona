// Copyright Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package sandbox

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/daytonaio/daytona/cli/apiclient"
	"github.com/daytonaio/daytona/cli/cmd/common"
	"github.com/daytonaio/daytona/cli/toolbox"
	apiclient_go "github.com/daytonaio/daytona/libs/api-client-go"
	"github.com/spf13/cobra"
)

var ExecCmd = &cobra.Command{
	Use:   "exec [SANDBOX_ID | SANDBOX_NAME] [-- COMMAND [ARGS...]]",
	Short: "Execute a command in a sandbox",
	Long:  "Execute a command in a running sandbox",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		apiClient, err := apiclient.GetApiClient(nil, nil)
		if err != nil {
			return err
		}

		sandboxIdOrName := args[0]

		// Find the command args after "--"
		commandArgs := []string{}
		dashDashIndex := -1
		for i, arg := range args {
			if arg == "--" {
				dashDashIndex = i
				break
			}
		}

		if dashDashIndex > -1 && dashDashIndex+1 < len(args) {
			commandArgs = args[dashDashIndex+1:]
		}

		if len(commandArgs) == 0 {
			return fmt.Errorf("no command specified")
		}

		// First, get the sandbox to get its ID and region (in case name was provided)
		sandbox, res, err := apiClient.SandboxAPI.GetSandbox(ctx, sandboxIdOrName).Execute()
		if err != nil {
			return apiclient.HandleErrorResponse(res, err)
		}

		if err := common.RequireStartedState(sandbox); err != nil {
			return err
		}

		toolboxClient := toolbox.NewClient(apiClient)

		// If TTY mode is enabled, use interactive TTY execution
		if execTTY {
			return executeTTY(ctx, toolboxClient, sandbox, commandArgs)
		}

		// Otherwise use regular command execution
		return executeRegular(ctx, toolboxClient, sandbox, commandArgs)
	},
}

func executeRegular(ctx context.Context, toolboxClient *toolbox.Client, sandbox *apiclient_go.Sandbox, commandArgs []string) error {
	// Build command from args
	command := buildCommand(commandArgs)

	executeRequest := toolbox.ExecuteRequest{
		Command: command,
	}
	if execCwd != "" {
		executeRequest.Cwd = &execCwd
	}
	if execTimeout > 0 {
		timeout := float32(execTimeout)
		executeRequest.Timeout = &timeout
	}

	// Execute the command via toolbox
	response, err := toolboxClient.ExecuteCommand(ctx, sandbox, executeRequest)
	if err != nil {
		return err
	}

	// Print the output (stdout + stderr combined)
	if response.Result != "" {
		fmt.Print(response.Result)
	}

	// Exit with the command's exit code
	exitCode := int(response.ExitCode)
	if exitCode != 0 {
		if response.Result == "" {
			fmt.Fprintf(os.Stderr, "Command failed with exit code %d\n", exitCode)
		}
		os.Exit(exitCode)
	}

	return nil
}

func executeTTY(ctx context.Context, toolboxClient *toolbox.Client, sandbox *apiclient_go.Sandbox, commandArgs []string) error {
	// For TTY mode, we need to convert our args into a format suitable for the TTY request
	var command string
	var args []string

	if len(commandArgs) > 0 {
		command = commandArgs[0]
		if len(commandArgs) > 1 {
			args = commandArgs[1:]
		}
	}

	executeRequest := toolbox.ExecuteTTYRequest{
		Command: command,
		Args:    args,
	}

	if execCwd != "" {
		executeRequest.Cwd = &execCwd
	}
	if execTimeout > 0 {
		timeout := uint32(execTimeout)
		executeRequest.Timeout = &timeout
	}

	// Execute the command via TTY
	return toolboxClient.ExecuteCommandTTY(ctx, sandbox, executeRequest)
}

var (
	execCwd     string
	execTTY     bool
	execTimeout int
)

// buildCommand reconstructs the command with proper shell escaping
// For args like ["sh", "-c", "python3 '...'"], it will properly quote them
func buildCommand(args []string) string {
	if len(args) == 0 {
		return ""
	}

	// Build command string for execution
	// If the second argument is "-c" (shell command), we need to preserve quoting
	if len(args) >= 3 && args[1] == "-c" {
		// For shell -c commands, we build: sh -c "command"
		parts := []string{args[0], args[1]}
		// Join the remaining args (the actual command) as-is
		cmdPart := strings.Join(args[2:], " ")
		return strings.Join(append(parts, cmdPart), " ")
	}

	// For regular commands, join all args with spaces
	return strings.Join(args, " ")
}

func init() {
	ExecCmd.Flags().StringVar(&execCwd, "cwd", "", "Working directory for command execution")
	ExecCmd.Flags().IntVar(&execTimeout, "timeout", 0, "Command timeout in seconds (0 for no timeout)")
	ExecCmd.Flags().BoolVar(&execTTY, "tty", false, "Enable TTY mode for interactive commands")
}
