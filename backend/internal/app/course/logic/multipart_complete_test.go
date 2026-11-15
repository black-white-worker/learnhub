package logic

import (
	"strings"
	"testing"
)

// checkSessionParts：合并分片前的最后一道闸。
// 这里放错的后果是"合出一个看起来正常、实际缺字节/尾部截断的视频"，
// 上传接口返回 200、课程列表看得到、播放器也能打开 —— 只能等学生看到第 37 分钟黑屏才发现。

const mb int64 = 1 << 20
const kb int64 = 1 << 10

func parts(sizes ...int64) map[int]int64 {
	m := make(map[int]int64, len(sizes))
	for i, s := range sizes {
		m[i+1] = s
	}
	return m
}

func TestCheckSessionParts_OK(t *testing.T) {
	numbers, err := checkSessionParts(parts(5*mb, 5*mb, 3*mb), 13*mb)
	if err != nil {
		t.Fatalf("连续且总大小一致应通过，实际 %v", err)
	}
	if len(numbers) != 3 || numbers[0] != 1 || numbers[2] != 3 {
		t.Fatalf("片号应按序返回，实际 %v", numbers)
	}
}

func TestCheckSessionParts_Empty(t *testing.T) {
	_, err := checkSessionParts(map[int]int64{}, 10*mb)
	if err == nil || !strings.Contains(err.Msg, "还没有收到任何分片") {
		t.Fatalf("没有分片应拒绝合并，实际 %v", err)
	}
}

// 断号：第 2 片没传上来，第 3 片传了。绝不能合。
func TestCheckSessionParts_GapRejected(t *testing.T) {
	_, err := checkSessionParts(map[int]int64{1: 5 * mb, 3: 5 * mb}, 10*mb)
	if err == nil || !strings.Contains(err.Msg, "第 2 片缺失") {
		t.Fatalf("断号应提示缺第 2 片，实际 %v", err)
	}
}

// 半截分片：总字节少于声明大小（断点续传记账错、或 PUT 成功但内容截断）。
func TestCheckSessionParts_SizeMismatchRejected(t *testing.T) {
	_, err := checkSessionParts(parts(5*mb, 2*mb), 10*mb)
	if err == nil || !strings.Contains(err.Msg, "不一致") {
		t.Fatalf("总大小不符应拒绝，实际 %v", err)
	}
}

// MinIO 要求除最后一片外每片 ≥5MB：前端把 1GB 切成 1000 片 1MB 时必须在服务端拦下。
func TestCheckSessionParts_SmallMiddlePartRejected(t *testing.T) {
	_, err := checkSessionParts(parts(1*mb, 1*mb, 1*mb), 3*mb)
	if err == nil || !strings.Contains(err.Msg, "分片大小不合法") {
		t.Fatalf("中间分片小于 5MB 应拒绝，实际 %v", err)
	}
	// 只有最后一片可以小于 5MB
	if _, err := checkSessionParts(parts(5*mb, 1*mb), 6*mb); err != nil {
		t.Fatalf("末片小于 5MB 应放行，实际 %v", err)
	}
}

// 单片文件（没有"中间片"）不应该被大小规则误伤。
func TestCheckSessionParts_SinglePartFile(t *testing.T) {
	if _, err := checkSessionParts(parts(800*kb), 800*kb); err != nil {
		t.Fatalf("单片小文件应放行，实际 %v", err)
	}
}
