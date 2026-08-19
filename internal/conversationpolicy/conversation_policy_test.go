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

package conversationpolicy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writePolicy(t *testing.T, configDir string, policy *ConversationPolicy) {
	t.Helper()
	if err := Save(configDir, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}
}

func TestLoadMissingFileIsEmptyPolicy(t *testing.T) {
	t.Chdir(t.TempDir())
	policy, err := Load(".")
	if err != nil {
		t.Fatal(err)
	}
	if policy.Enabled() {
		t.Fatalf("missing policy must not restrict reads: %#v", policy)
	}
}

func TestLoadAndAllowsExactMatch(t *testing.T) {
	t.Chdir(t.TempDir())
	writePolicy(t, ".", &ConversationPolicy{
		Version: conversationPolicyVersion,
		AllowedConversations: []AllowedConversation{
			{Kind: "group", ID: "cid-123"},
			{Kind: "user", ID: "user-001"},
		},
	})
	policy, err := Load(".")
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Enabled() {
		t.Fatal("non-empty allowlist must be enabled")
	}
	if !policy.Allows("group", "cid-123") || !policy.Allows("user", "user-001") {
		t.Fatalf("allowlist match failed: %#v", policy.AllowedConversations)
	}
	if policy.Allows("group", "cid-456") || policy.Allows("user", "cid-123") {
		t.Fatal("non-matching target must be refused")
	}
}

func TestLoadCorruptFileFailsClosed(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(PolicyPath("."), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("."); err == nil {
		t.Fatal("corrupt policy must error (fails closed to no restriction)")
	}
}

func TestLoadUnsupportedVersionFailsClosed(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(PolicyPath("."), []byte(`{"version":99,"allowedConversations":[{"kind":"group","id":"cid"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("."); err == nil {
		t.Fatal("unsupported version must error")
	}
}

func TestConfigDirHonoursDWSConfigDir(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "dws-cfg")
	t.Setenv("DWS_CONFIG_DIR", configDir)
	if got := ConfigDir(); got != configDir {
		t.Fatalf("ConfigDir = %q, want %q", got, configDir)
	}
}

func TestSaveRoundTripsAndPermissions(t *testing.T) {
	t.Chdir(t.TempDir())
	writePolicy(t, ".", &ConversationPolicy{
		Version: conversationPolicyVersion,
		AllowedConversations: []AllowedConversation{
			{Kind: "group", ID: "cid-1"},
		},
	})
	loaded, err := Load(".")
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Allows("group", "cid-1") {
		t.Fatalf("round-trip lost allowlist: %#v", loaded)
	}
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}
	info, err := os.Stat(PolicyPath("."))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("policy permission = %v, want 0600", info.Mode().Perm())
	}
}
