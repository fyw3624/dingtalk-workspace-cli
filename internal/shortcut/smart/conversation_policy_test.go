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

package smart

import (
	"bytes"
	stderrors "errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/conversationpolicy"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

const policyPageFixture = `{"result":{"hasMore":false,"messages":[{"openMessageId":"m1","createTime":"2026-01-01 00:00:00"}]}}`

func runChatMessagesPolicy(t *testing.T, caller *chatMessagesPagingCaller, args []string) error {
	t.Helper()
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageChatMessagesConversationPolicyEnforced(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	if err := conversationpolicy.Save(configDir, &conversationpolicy.ConversationPolicy{
		AllowedConversations: []conversationpolicy.AllowedConversation{
			{Kind: "group", ID: "cid-allowed"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// A target outside the allowlist is refused before any MCP call.
	blocked := &chatMessagesPagingCaller{responses: []string{policyPageFixture}}
	blockedErr := runChatMessagesPolicy(t, blocked, []string{
		"chat", "+chat-messages", "--conversation-id", "cid-other",
	})
	var typed *apperrors.Error
	if !stderrors.As(blockedErr, &typed) || typed.Reason != "conversation_not_allowed" {
		t.Fatalf("outside-allowlist read error = %v", blockedErr)
	}
	if len(blocked.args) != 0 {
		t.Fatalf("refused read still invoked MCP: %#v", blocked.args)
	}

	// An allowlisted target reads normally.
	allowed := &chatMessagesPagingCaller{responses: []string{policyPageFixture}}
	if err := runChatMessagesPolicy(t, allowed, []string{
		"chat", "+chat-messages", "--conversation-id", "cid-allowed",
	}); err != nil {
		t.Fatalf("allowlisted read failed: %v", err)
	}
	if len(allowed.args) != 1 {
		t.Fatalf("allowlisted read calls = %#v", allowed.args)
	}
}

func TestCrossPlatformCoverageChatMessagesConversationPolicyUserScope(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	if err := conversationpolicy.Save(configDir, &conversationpolicy.ConversationPolicy{
		AllowedConversations: []conversationpolicy.AllowedConversation{
			{Kind: "user", ID: "user-001"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	blocked := &chatMessagesPagingCaller{responses: []string{policyPageFixture}}
	blockedErr := runChatMessagesPolicy(t, blocked, []string{
		"chat", "+chat-messages", "--user", "user-999",
	})
	var typed *apperrors.Error
	if !stderrors.As(blockedErr, &typed) || typed.Reason != "conversation_not_allowed" {
		t.Fatalf("outside-allowlist user read error = %v", blockedErr)
	}
	if len(blocked.args) != 0 {
		t.Fatalf("refused user read still invoked MCP: %#v", blocked.args)
	}
}
