package agent

import (
	"context"
	"os/exec"
)

// execCommand is a variable to allow mocking in tests.
var execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}