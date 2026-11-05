package course

import (
	"strings"
	"testing"
)

// ffmpeg 参数拼装的单元测试（纯函数，不依赖真实 ffmpeg）。
// 这些用例原本误放在 course/logic 包里，跨包访问不到私有函数，已移回本包。

// 参数必须含 +faststart（moov 前置，边下边播 / 可拖动）
func TestBuildFaststartArgs_ContainsFaststart(t *testing.T) {
	args := buildFaststartArgs("in.mp4", "out.mp4")
	found := false
	for i, a := range args {
		if a == "-movflags" && i+1 < len(args) && args[i+1] == "+faststart" {
			found = true
		}
	}
	if !found {
		t.Fatalf("参数里缺少 -movflags +faststart: %v", args)
	}
}

// 编码参数统一 H.264 / AAC，保证浏览器都能播
func TestBuildFaststartArgs_CommonCodecs(t *testing.T) {
	args := buildFaststartArgs("in.mp4", "out.mp4")
	joined := strings.Join(args, " ")
	for _, want := range []string{"libx264", "yuv420p", "aac"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("参数里缺少 %s: %v", want, args)
		}
	}
}

// 源/目标文件名要出现在参数里（防止拼错位置导致覆盖源文件）
func TestBuildFaststartArgs_UsesPaths(t *testing.T) {
	args := buildFaststartArgs("src.mp4", "dst.mp4")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "src.mp4") || !strings.Contains(joined, "dst.mp4") {
		t.Fatalf("参数里缺少源/目标路径: %v", args)
	}
}
