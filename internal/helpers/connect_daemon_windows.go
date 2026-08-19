// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build windows

package helpers

import (
	"os"
	"os/exec"
	"syscall"
)

// createNewProcessGroup (0x00000200) prevents the child from receiving the
// parent's Ctrl+C console event, mirroring Setsid on Unix. Combined with
// HideWindow this lets a re-exec'd supervisor outlive the parent terminal.
const createNewProcessGroup = 0x00000200

// daemonDetachSupported enables `connect --daemon` on Windows: the supervisor
// is spawned as a hidden, console-detached child (CREATE_NEW_PROCESS_GROUP +
// HideWindow) and is stopped via TerminateProcess, which os.Process.Kill maps
// to on Windows.
const daemonDetachSupported = true

func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
		HideWindow:    true,
	}
}

// applyDetach hides the daemon child and detaches it from the parent console.
func applyDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = detachSysProcAttr()
}

// configureWorkerProcessGroup gives the connector worker the same detached,
// hidden treatment so agent processes it spawns do not attach to any console.
func configureWorkerProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = detachSysProcAttr()
}

// cleanupWorkerProcessGroup is a no-op on Windows: there is no process-group
// kill; the worker's own termination cleans its children.
func cleanupWorkerProcessGroup(_ int) {}

// init installs the Windows stop primitive. Windows has no POSIX-style
// graceful signals: os.Process.Signal returns syscall.EWINDOWS for everything
// except Kill. daemonSignalProcess therefore maps any requested signal to
// TerminateProcess so both the graceful-stop and force-kill paths of the
// daemon supervisor terminate the process deterministically.
func init() {
	daemonSignalProcess = func(process *os.Process, _ os.Signal) error {
		return process.Kill()
	}
}
