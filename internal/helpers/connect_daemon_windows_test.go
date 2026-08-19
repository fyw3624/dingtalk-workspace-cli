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
	"os/exec"
	"testing"
	"time"
)

func TestWindowsDaemonDetachIsEnabled(t *testing.T) {
	if !daemonDetachSupported {
		t.Fatal("daemonDetachSupported must be true on Windows")
	}
	if !daemonDetachEnabled {
		t.Fatal("daemonDetachEnabled must follow the platform constant")
	}
}

func TestWindowsApplyDetachConfiguresHiddenDetachedChild(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit 0")
	applyDetach(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("applyDetach must set SysProcAttr")
	}
	if cmd.SysProcAttr.CreationFlags&createNewProcessGroup == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NEW_PROCESS_GROUP", cmd.SysProcAttr.CreationFlags)
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow must be true")
	}
}

func TestWindowsConfigureWorkerProcessGroupDetachesWorker(t *testing.T) {
	worker := exec.Command("cmd", "/c", "exit 0")
	configureWorkerProcessGroup(worker)
	if worker.SysProcAttr == nil || worker.SysProcAttr.CreationFlags&createNewProcessGroup == 0 || !worker.SysProcAttr.HideWindow {
		t.Fatalf("worker SysProcAttr = %#v, want detached + hidden", worker.SysProcAttr)
	}
	// Cleanup must be a safe no-op.
	cleanupWorkerProcessGroup(123)
}

func TestWindowsDaemonSignalProcessTerminatesProcess(t *testing.T) {
	// A long-running child: Windows has no graceful signal, so the installed
	// stop primitive must terminate it deterministically.
	cmd := exec.Command("cmd", "/c", "ping -n 30 127.0.0.1 > nul")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	if err := daemonSignalProcess(cmd.Process, nil); err != nil {
		t.Fatalf("daemonSignalProcess: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !daemonProcessAlive(cmd.Process.Pid) {
			_, _ = cmd.Process.Wait()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("daemonSignalProcess did not terminate the child process")
}
