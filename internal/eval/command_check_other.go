//go:build !unix

package eval

import "os/exec"

// configureProcessTree is a no-op where process groups are unavailable; the
// default cancel kills the direct child and WaitDelay bounds the wait.
func configureProcessTree(*exec.Cmd) {}
