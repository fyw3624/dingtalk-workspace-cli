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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/conversationpolicy"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// ValidateConversationReadAccess enforces the opt-in conversation allowlist
// for a chat read target. It is shared by every chat read command so the
// policy has a single enforcement point. When the policy is not configured,
// reads are unrestricted (fully backward compatible); once a non-empty
// allowlist exists, any target outside it fails closed with
// reason=conversation_not_allowed.
func ValidateConversationReadAccess(kind, id string) error {
	if kind == "" || id == "" {
		return nil
	}
	policy, err := conversationpolicy.Load(conversationpolicy.ConfigDir())
	if err != nil {
		return apperrors.NewValidation("读取会话读取策略失败: "+err.Error(),
			apperrors.WithReason("conversation_policy_unreadable"),
			apperrors.WithHint("检查 "+conversationpolicy.PolicyPath(conversationpolicy.ConfigDir())+" 是否可读且格式正确；修复前读取命令不会因策略文件损坏而放行名单外会话"),
		)
	}
	if !policy.Enabled() {
		return nil
	}
	if !policy.Allows(kind, id) {
		return apperrors.NewValidation("目标会话不在允许读取白名单内",
			apperrors.WithReason("conversation_not_allowed"),
			apperrors.WithHint("在 conversation_policy.json 的 allowedConversations 中添加该会话（kind="+kind+" id="+id+"）后重试"),
		)
	}
	return nil
}
