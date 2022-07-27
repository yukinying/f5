// Package pgexec provides a Cmd struct that set process group by default.
// It also overrides the kill function when context is used, so all child
// process created would be killed correctly if the context is done.
//
// This is inspired by https://bigkevmcd.github.io/go/pgrp/context/2019/02/19/terminating-processes-in-go.html
package pgexec

import (
	"os/exec"
	"syscall"
)

type Cmd struct {
	origCmd *exec.Cmd
}

func Command(name string, arg ...string) *Cmd {
	cmd := Cmd{
		origCmd: exec.Command(name, arg...),
	}
	// set process group, so we can kill all of the spawned processes.
	cmd.origCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return &cmd
}
