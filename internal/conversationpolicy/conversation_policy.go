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

// Package conversationpolicy provides an opt-in, fail-closed conversation
// allowlist for chat read commands. When the policy file is absent or the
// allowlist is empty, reads are not restricted (fully backward compatible);
// once a non-empty allowlist is configured, every conversation read target
// must match an entry exactly.
package conversationpolicy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const conversationPolicyFile = "conversation_policy.json"

// conversationPolicyVersion is the on-disk schema version. Mismatched
// versions are treated as a corrupt policy (fail closed to no allowlist).
const conversationPolicyVersion = 1

var (
	policyReadFile    = os.ReadFile
	policyUserHomeDir = os.UserHomeDir
	policyAtomicWrite = helpers.AtomicWriteJSON
)

// AllowedConversation is one exact conversation read target.
type AllowedConversation struct {
	// Kind is "group" (openConversationId) or "user" (userId /
	// openDingTalkId).
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// ConversationPolicy is the persisted conversation allowlist.
type ConversationPolicy struct {
	Version              int                   `json:"version"`
	AllowedConversations []AllowedConversation `json:"allowedConversations"`
}

// ConfigDir resolves the DWS configuration directory, honouring
// DWS_CONFIG_DIR, then the edition default, then ~/.dws.
func ConfigDir() string {
	if envDir := os.Getenv("DWS_CONFIG_DIR"); envDir != "" {
		return envDir
	}
	if fn := edition.Get().ConfigDir; fn != nil {
		return fn()
	}
	homeDir, err := policyUserHomeDir()
	if err != nil {
		return ".dws"
	}
	return filepath.Join(homeDir, ".dws")
}

// PolicyPath returns the policy file path for a configuration directory.
func PolicyPath(configDir string) string {
	return filepath.Join(configDir, conversationPolicyFile)
}

// Load reads the policy. A missing file is an empty (non-restricting)
// policy; a corrupt or schema-mismatched file fails closed to an empty
// allowlist so reads are not silently restricted by garbage.
func Load(configDir string) (*ConversationPolicy, error) {
	path := PolicyPath(configDir)
	data, err := policyReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ConversationPolicy{Version: conversationPolicyVersion}, nil
		}
		return nil, fmt.Errorf("reading conversation policy: %w", err)
	}
	var policy ConversationPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("parsing conversation policy: %w", err)
	}
	if policy.Version != conversationPolicyVersion {
		return nil, fmt.Errorf("unsupported conversation policy version %d", policy.Version)
	}
	return &policy, nil
}

// Save atomically persists the policy with secure permissions.
func Save(configDir string, policy *ConversationPolicy) error {
	if policy == nil {
		policy = &ConversationPolicy{Version: conversationPolicyVersion}
	}
	if policy.Version == 0 {
		policy.Version = conversationPolicyVersion
	}
	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling conversation policy: %w", err)
	}
	data = append(data, '\n')
	if err := policyAtomicWrite(PolicyPath(configDir), data); err != nil {
		return fmt.Errorf("writing conversation policy: %w", err)
	}
	return nil
}

// Enabled reports whether the allowlist restricts reads. An absent or empty
// allowlist means the policy is not active (fully backward compatible).
func (p *ConversationPolicy) Enabled() bool {
	return p != nil && len(p.AllowedConversations) > 0
}

// Allows reports whether an exact conversation target is permitted.
func (p *ConversationPolicy) Allows(kind, id string) bool {
	if p == nil {
		return false
	}
	for _, entry := range p.AllowedConversations {
		if entry.Kind == kind && entry.ID == id {
			return true
		}
	}
	return false
}
