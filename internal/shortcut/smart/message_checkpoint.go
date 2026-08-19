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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	chatshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
)

// messageCheckpointVersion is the on-disk schema version of a
// +chat-messages checkpoint. Bump it (and migrate readers) when the JSON
// shape changes; mismatched versions are treated as "no checkpoint".
const messageCheckpointVersion = 1

// messageCheckpoint persists the pagination state of a bounded
// +chat-messages run so a later invocation of the same conversation can
// resume exactly where the previous run stopped (cursor + direction + time
// boundary), without re-reading already covered messages.
type messageCheckpoint struct {
	Version      int                           `json:"version"`
	Conversation messageCheckpointConversation `json:"conversation"`
	Tool         string                        `json:"tool"`
	Window       *messageCheckpointWindow      `json:"window,omitempty"`
	Cursor       *messageCheckpointCursor      `json:"cursor,omitempty"`
	Complete     bool                          `json:"complete"`
	StopReason   string                        `json:"stopReason"`
	UpdatedAt    string                        `json:"updatedAt"`
}

type messageCheckpointConversation struct {
	Kind string `json:"kind"` // "group" | "user"
	ID   string `json:"id"`
}

type messageCheckpointWindow struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
	Order string `json:"order"`
}

// messageCheckpointCursor mirrors the typed nextPage contract of the
// im.message-list.v1 envelope: the RFC3339Nano time boundary is the actual
// resume point, nextCursor is retained for diagnostics.
type messageCheckpointCursor struct {
	Direction  string `json:"direction"`
	Time       string `json:"time"`
	NextCursor any    `json:"nextCursor,omitempty"`
}

// validateCheckpointFilePath applies the same workspace-relative, .json-only,
// no-symlink boundary used by --output so a checkpoint can never be written
// outside the current working directory or through a symlink.
func validateCheckpointFilePath(path string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return apperrors.NewValidation("--checkpoint-file 必须是工作目录内的相对 JSON 文件路径")
	}
	if err := chatshortcut.ValidateMessageExportOutput(trimmed); err != nil {
		// Re-word the shared validator's flag name so the message points at
		// the flag the user actually set.
		return apperrors.NewValidation(strings.ReplaceAll(err.Error(), "--output", "--checkpoint-file"))
	}
	return nil
}

// readCheckpoint loads a valid checkpoint; a missing, corrupt, or
// schema-mismatched file degrades to "no checkpoint" instead of failing the
// command.
func readCheckpoint(path string) (*messageCheckpoint, bool) {
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return nil, false
	}
	var cp messageCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, false
	}
	if cp.Version != messageCheckpointVersion {
		return nil, false
	}
	return &cp, true
}

// writeCheckpoint atomically publishes a checkpoint with secure permissions;
// a failure keeps the previous checkpoint untouched.
func writeCheckpoint(path string, cp messageCheckpoint) error {
	rendered, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("编码消息 checkpoint 失败: %v", err))
	}
	rendered = append(rendered, '\n')
	if err := helpers.AtomicWriteJSON(strings.TrimSpace(path), rendered); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("写入消息 checkpoint 失败: %v", err))
	}
	return nil
}

// checkpointConversationFor derives the stable conversation identity of the
// resolved request, matching how the checkpoint reader later binds a resume
// to the same conversation.
func checkpointConversationFor(request *chatMessagesRequest) (kind, id string) {
	if request.fallbackConversationID != "" {
		return "group", request.fallbackConversationID
	}
	if value, ok := request.params["openDingTalkId"].(string); ok && value != "" {
		return "user", value
	}
	if value, ok := request.params["userId"].(string); ok && value != "" {
		return "user", value
	}
	return "", ""
}

// applyChatMessagesCheckpoint resumes a bounded read from a stored
// checkpoint. It only ever fires when the user did not explicitly choose a
// time window, a direction, or a time boundary: any explicit cursor or window
// wins over the checkpoint. A non-matching conversation is ignored.
func applyChatMessagesCheckpoint(rt *shortcut.RuntimeContext, request *chatMessagesRequest, path string) {
	if rt.Changed("time") || rt.Changed("start") || rt.Changed("start-time") ||
		rt.Changed("end") || rt.Changed("end-time") || rt.Changed("order") ||
		rt.Changed("sort") || rt.Changed("direction") {
		return
	}
	cp, ok := readCheckpoint(path)
	if !ok || cp.Cursor == nil || strings.TrimSpace(cp.Cursor.Time) == "" {
		return
	}
	kind, id := checkpointConversationFor(request)
	if kind == "" || id == "" || cp.Conversation.Kind != kind || cp.Conversation.ID != id {
		return
	}
	request.params["time"] = cp.Cursor.Time
	if cp.Cursor.Direction == "newer" || cp.Cursor.Direction == "older" {
		request.direction = cp.Cursor.Direction
		request.params["forward"] = cp.Cursor.Direction == "newer"
	}
}

// writeChatMessagesCheckpoint records the pagination state after a clean
// run. Nothing is written on --dry-run, on any page failure (the failure
// ledger means the state cannot be trusted), or when no page was produced.
// A completed run clears the cursor so the next invocation starts fresh.
func writeChatMessagesCheckpoint(rt *shortcut.RuntimeContext, request *chatMessagesRequest, payload map[string]any, path string) error {
	if rt.DryRun() {
		return nil
	}
	failures, _ := payload["failures"].([]map[string]any)
	if len(failures) > 0 {
		return nil
	}
	hasOutput := false
	if rt.Bool("page-all") {
		if pages, _ := payload["pagesFetched"].(int); pages > 0 {
			hasOutput = true
		}
	} else {
		// A single-page run publishes its envelope (hasMore/nextPage) even
		// when the page itself is empty.
		hasOutput = true
	}
	if !hasOutput {
		return nil
	}

	kind, id := checkpointConversationFor(request)
	complete, _ := payload["complete"].(bool)
	stopReason, _ := payload["stopReason"].(string)
	cp := messageCheckpoint{
		Version:      messageCheckpointVersion,
		Conversation: messageCheckpointConversation{Kind: kind, ID: id},
		Tool:         request.tool,
		Complete:     complete,
		StopReason:   stopReason,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if metadata := request.timeRange.metadata(); metadata != nil {
		window := &messageCheckpointWindow{Order: request.timeRange.order}
		if start, ok := metadata["startTime"].(string); ok {
			window.Start = start
		}
		if end, ok := metadata["endTime"].(string); ok {
			window.End = end
		}
		cp.Window = window
	}
	if !complete {
		if nextPage, ok := payload["nextPage"].(map[string]any); ok {
			cp.Cursor = &messageCheckpointCursor{
				Direction:  checkpointString(nextPage["direction"]),
				Time:       checkpointString(nextPage["time"]),
				NextCursor: nextPage["nextCursor"],
			}
		}
	}
	return writeCheckpoint(path, cp)
}

func checkpointString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
