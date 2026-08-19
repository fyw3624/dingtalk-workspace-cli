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

package chat

import (
	stderrors "errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/conversationpolicy"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestValidateConversationReadAccessAllowsWithoutPolicy(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	if err := ValidateConversationReadAccess("group", "cid-123"); err != nil {
		t.Fatalf("no policy must not restrict reads: %v", err)
	}
}

func TestValidateConversationReadAccessAllowsAllowlistedTarget(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	if err := conversationpolicy.Save(configDir, &conversationpolicy.ConversationPolicy{
		AllowedConversations: []conversationpolicy.AllowedConversation{
			{Kind: "group", ID: "cid-123"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateConversationReadAccess("group", "cid-123"); err != nil {
		t.Fatalf("allowlisted target refused: %v", err)
	}
}

func TestValidateConversationReadAccessFailsClosedOutsideAllowlist(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	if err := conversationpolicy.Save(configDir, &conversationpolicy.ConversationPolicy{
		AllowedConversations: []conversationpolicy.AllowedConversation{
			{Kind: "group", ID: "cid-123"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	err := ValidateConversationReadAccess("group", "cid-456")
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed.Reason != "conversation_not_allowed" {
		t.Fatalf("outside-allowlist error = %#v", err)
	}
}

func TestValidateConversationReadAccessIgnoresEmptyIdentity(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	if err := ValidateConversationReadAccess("", ""); err != nil {
		t.Fatalf("empty identity must be skipped: %v", err)
	}
}
