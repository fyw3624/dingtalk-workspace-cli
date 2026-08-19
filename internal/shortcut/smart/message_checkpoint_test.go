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
	"encoding/json"
	stderrors "errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

const checkpointPageOne = `{"result":{"hasMore":true,"nextCursor":1767225600123,"messages":[{"openMessageId":"m2","createTime":"2026-01-02 00:00:00"},{"openMessageId":"m1","createTime":"2026-01-01 00:00:00"}]}}`
const checkpointPageTwo = `{"result":{"hasMore":false,"messages":[{"openMessageId":"m0","createTime":"2025-12-31 00:00:00"}]}}`

func runChatMessagesCheckpoint(t *testing.T, caller *chatMessagesPagingCaller, args []string) error {
	t.Helper()
	helpers.InitDeps(caller)
	root := newPlatformCoverageRoot()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs(args)
	return root.Execute()
}

func readCheckpointFile(t *testing.T, path string) messageCheckpoint {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read checkpoint %s: %v", path, err)
	}
	var cp messageCheckpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		t.Fatalf("decode checkpoint %s: %v", path, err)
	}
	return cp
}

func TestCrossPlatformCoverageChatMessagesCheckpointWritesAfterCleanRun(t *testing.T) {
	t.Chdir(t.TempDir())
	caller := &chatMessagesPagingCaller{responses: []string{checkpointPageOne, checkpointPageTwo}}
	if err := runChatMessagesCheckpoint(t, caller, []string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--page-all", "--checkpoint-file", "cp.json",
	}); err != nil {
		t.Fatal(err)
	}
	cp := readCheckpointFile(t, "cp.json")
	if cp.Version != 1 || cp.Complete != true || cp.Cursor != nil {
		t.Fatalf("clean-run checkpoint = %#v", cp)
	}
	if cp.Conversation.Kind != "group" || cp.Conversation.ID != "cid" {
		t.Fatalf("checkpoint conversation = %#v", cp.Conversation)
	}
	if cp.UpdatedAt == "" {
		t.Fatal("checkpoint updatedAt missing")
	}
}

func TestCrossPlatformCoverageChatMessagesCheckpointResumesFromCursor(t *testing.T) {
	t.Chdir(t.TempDir())
	// First run stops at --page-limit 1 with hasMore=true, so the checkpoint
	// must retain the typed cursor boundary.
	first := &chatMessagesPagingCaller{responses: []string{checkpointPageOne, checkpointPageTwo}}
	if err := runChatMessagesCheckpoint(t, first, []string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--page-all", "--page-limit", "1", "--checkpoint-file", "cp.json",
	}); err != nil {
		t.Fatal(err)
	}
	cp := readCheckpointFile(t, "cp.json")
	if cp.Complete != false || cp.Cursor == nil || cp.Cursor.Time == "" {
		t.Fatalf("truncated-run checkpoint = %#v", cp)
	}
	wantBoundary := time.UnixMilli(1767225600123).UTC().Format(time.RFC3339Nano)
	if cp.Cursor.Time != wantBoundary {
		t.Fatalf("cursor time = %q, want %q", cp.Cursor.Time, wantBoundary)
	}

	// Second run with no explicit time window must resume from the cursor.
	second := &chatMessagesPagingCaller{responses: []string{checkpointPageTwo}}
	if err := runChatMessagesCheckpoint(t, second, []string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--checkpoint-file", "cp.json",
	}); err != nil {
		t.Fatal(err)
	}
	if len(second.args) != 1 || second.args[0]["time"] != wantBoundary {
		t.Fatalf("resume args = %#v, want time=%q", second.args, wantBoundary)
	}
	// The resumed clean run must clear the cursor.
	after := readCheckpointFile(t, "cp.json")
	if after.Complete != true || after.Cursor != nil {
		t.Fatalf("resumed checkpoint = %#v", after)
	}
}

func TestCrossPlatformCoverageChatMessagesCheckpointCorruptFileIgnored(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("cp.json", []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	caller := &chatMessagesPagingCaller{responses: []string{checkpointPageTwo}}
	if err := runChatMessagesCheckpoint(t, caller, []string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--page-all", "--checkpoint-file", "cp.json",
	}); err != nil {
		t.Fatalf("corrupt checkpoint must not fail the command: %v", err)
	}
	// A clean run repairs the corrupt file.
	cp := readCheckpointFile(t, "cp.json")
	if cp.Complete != true || cp.Version != 1 {
		t.Fatalf("repaired checkpoint = %#v", cp)
	}
}

func TestCrossPlatformCoverageChatMessagesCheckpointExplicitTimeWins(t *testing.T) {
	t.Chdir(t.TempDir())
	cp := messageCheckpoint{
		Version:      1,
		Conversation: messageCheckpointConversation{Kind: "group", ID: "cid"},
		Cursor:       &messageCheckpointCursor{Direction: "older", Time: "2026-01-05T00:00:00Z"},
	}
	if err := writeCheckpoint("cp.json", cp); err != nil {
		t.Fatal(err)
	}
	caller := &chatMessagesPagingCaller{responses: []string{checkpointPageTwo}}
	if err := runChatMessagesCheckpoint(t, caller, []string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--time", "2026-01-03 00:00:00", "--checkpoint-file", "cp.json",
	}); err != nil {
		t.Fatal(err)
	}
	if caller.args[0]["time"] != "2026-01-03 00:00:00" {
		t.Fatalf("explicit --time lost: args = %#v", caller.args)
	}
}

func TestCrossPlatformCoverageChatMessagesCheckpointConversationMismatchIgnored(t *testing.T) {
	t.Chdir(t.TempDir())
	cp := messageCheckpoint{
		Version:      1,
		Conversation: messageCheckpointConversation{Kind: "group", ID: "cid-other"},
		Cursor:       &messageCheckpointCursor{Direction: "older", Time: "2026-01-05T00:00:00Z"},
	}
	if err := writeCheckpoint("cp.json", cp); err != nil {
		t.Fatal(err)
	}
	caller := &chatMessagesPagingCaller{responses: []string{checkpointPageTwo}}
	if err := runChatMessagesCheckpoint(t, caller, []string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--checkpoint-file", "cp.json",
	}); err != nil {
		t.Fatal(err)
	}
	if caller.args[0]["time"] == "2026-01-05T00:00:00Z" {
		t.Fatalf("checkpoint for another conversation was applied: args = %#v", caller.args)
	}
}

func TestCrossPlatformCoverageChatMessagesCheckpointRejectsUnsafePath(t *testing.T) {
	t.Chdir(t.TempDir())
	abs := filepath.Join(t.TempDir(), "outside.json")
	caller := &chatMessagesPagingCaller{responses: []string{checkpointPageTwo}}
	for _, bad := range []string{abs, "../escape.json", "notes.txt"} {
		if err := runChatMessagesCheckpoint(t, caller, []string{
			"chat", "+chat-messages", "--conversation-id", "cid",
			"--checkpoint-file", bad,
		}); err == nil {
			t.Fatalf("unsafe checkpoint path %q unexpectedly accepted", bad)
		}
	}
}

func TestCrossPlatformCoverageChatMessagesCheckpointFailedPageKeepsPrevious(t *testing.T) {
	t.Chdir(t.TempDir())
	cp := messageCheckpoint{
		Version:      1,
		Conversation: messageCheckpointConversation{Kind: "group", ID: "cid"},
		Cursor:       &messageCheckpointCursor{Direction: "older", Time: "2026-01-05T00:00:00Z"},
	}
	if err := writeCheckpoint("cp.json", cp); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile("cp.json")
	if err != nil {
		t.Fatal(err)
	}
	caller := &chatMessagesPagingCaller{responses: []string{checkpointPageOne}, failAt: 1}
	runErr := runChatMessagesCheckpoint(t, caller, []string{
		"chat", "+chat-messages", "--conversation-id", "cid",
		"--page-all", "--checkpoint-file", "cp.json",
	})
	var typed *apperrors.Error
	if !stderrors.As(runErr, &typed) || typed.Reason != "chat_messages_incomplete" {
		t.Fatalf("expected incomplete error, got %v", runErr)
	}
	after, err := os.ReadFile("cp.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("failed run rewrote the checkpoint\nbefore: %s\nafter:  %s", before, after)
	}
}
