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
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func resourceDownloadGuardRT(t *testing.T, values map[string]string) *shortcut.RuntimeContext {
	t.Helper()
	root := newPlatformCoverageRoot()
	cmd, _, err := root.Find([]string{"chat", "+messages-mget"})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%q: %v", name, value, err)
		}
	}
	return shortcut.RuntimeContextForTest(cmd, MessagesResourceDownload)
}

func TestCheckResourceDownloadSize(t *testing.T) {
	ok := func(maxFileSize, declared, actual int64) error {
		return checkResourceDownloadSize(maxFileSize, declared, actual)
	}
	if err := ok(1024, 1024, 0); err != nil {
		t.Fatalf("declared == cap must pass: %v", err)
	}
	if err := ok(1024, 0, 1024); err != nil {
		t.Fatalf("actual == cap must pass: %v", err)
	}
	if err := ok(0, 1<<40, 1<<40); err != nil {
		t.Fatalf("limit 0 must disable the guard: %v", err)
	}
	if err := ok(1024, 2048, 0); err == nil {
		t.Fatal("oversized declared length must be rejected")
	}
	if err := ok(1024, 0, 2048); err == nil {
		t.Fatal("oversized streamed size must be rejected")
	}
}

func TestWithResourceDownloadLimitsRoundTrip(t *testing.T) {
	if got := resourceDownloadMaxFileSize(context.Background()); got != 0 {
		t.Fatalf("empty context limit = %d, want 0", got)
	}
	ctx := withResourceDownloadLimits(context.Background(), 4096)
	if got := resourceDownloadMaxFileSize(ctx); got != 4096 {
		t.Fatalf("limit = %d, want 4096", got)
	}
	// Non-positive limits must not be installed.
	if got := resourceDownloadMaxFileSize(withResourceDownloadLimits(ctx, 0)); got != 4096 {
		t.Fatalf("zero limit overwrote active limit: %d", got)
	}
}

func TestValidateMessageResourceDownloadRejectsNegativeLimits(t *testing.T) {
	rt := resourceDownloadGuardRT(t, map[string]string{
		"download-resources": "true",
		"max-file-size":      "-1",
	})
	if err := ValidateMessageResourceDownload(rt); err == nil {
		t.Fatal("negative --max-file-size must be rejected")
	}
	rt = resourceDownloadGuardRT(t, map[string]string{
		"download-resources": "true",
		"max-total-size":     "-5",
	})
	var typed *apperrors.Error
	if err := ValidateMessageResourceDownload(rt); !stderrors.As(err, &typed) {
		t.Fatalf("negative --max-total-size error = %v", err)
	}
}

func TestCrossPlatformCoverageDownloadResourcesStopsAtTotalLimit(t *testing.T) {
	resetResourceDownloadHooks(t)
	t.Chdir(t.TempDir())
	resourceDownload = func(
		_ context.Context,
		_ *http.Client,
		_ string,
		_ map[string]string,
		_ string,
		_ bool,
	) (int64, error) {
		return 6, nil
	}
	fake := &larkAlignmentCaller{responses: map[string]string{
		"im/list_messages_by_ids": `{"result":[
			{"openMessageId":"m1","openConversationId":"cid","content":"{\"mediaId\":\"@f1\"}"},
			{"openMessageId":"m2","openConversationId":"cid","content":"{\"mediaId\":\"@f2\"}"}
		]}`,
		"im/get_resource_download_url": `{"result":{"resourceUrl":"https://download.dingtalk.com/a.bin"}}`,
	}}
	helpers.InitDeps(fake)
	root := newPlatformCoverageRoot()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{
		"chat", "+messages-mget",
		"--msg-ids", "m1,m2",
		"--download-resources",
		"--max-total-size", "8",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var ledger map[string]any
	if err := json.Unmarshal(output.Bytes(), &ledger); err != nil {
		t.Fatal(err)
	}
	downloads := ledger["resourceDownloads"].(map[string]any)
	if downloads["downloadedCount"] != float64(1) || downloads["failedCount"] != float64(1) {
		t.Fatalf("total-limit ledger = %#v", downloads)
	}
	failures, _ := downloads["failures"].([]any)
	if len(failures) != 1 {
		t.Fatalf("total-limit failures = %#v", failures)
	}
	first := failures[0].(map[string]any)
	if first["stage"] != "total-limit" {
		t.Fatalf("total-limit failure stage = %#v", first)
	}
}
