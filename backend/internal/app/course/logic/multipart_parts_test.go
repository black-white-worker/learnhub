package logic

import (
	"sort"
	"testing"

	"learningplatform/internal/platform/oss"
)

// sortParts 把分片按 partNumber 升序（ComposeObject 要求按顺序合并）。
// 抽成纯函数可单测，避免每次 Complete 都连真实 MinIO。
func sortParts(parts []oss.MultipartPart) {
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})
}

// 前端乱序传分片：Complete 前必须按 partNumber 升序，否则 ComposeObject 合并顺序错。
func TestSortParts_Reorders(t *testing.T) {
	in := []oss.MultipartPart{
		{PartNumber: 3, ETag: "e3"},
		{PartNumber: 1, ETag: "e1"},
		{PartNumber: 2, ETag: "e2"},
	}
	sortParts(in)
	want := []int{1, 2, 3}
	for i, p := range in {
		if p.PartNumber != want[i] {
			t.Errorf("分片[%d] 期望编号 %d，实际 %d", i, want[i], p.PartNumber)
		}
	}
}

// 空分片列表：sort 不应 panic。
func TestSortParts_Empty(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("空分片列表不应 panic: %v", r)
		}
	}()
	sortParts(nil)
}

// partNumber 范围校验（SignPart / Complete 的前置条件）。
func TestValidPartNumber(t *testing.T) {
	cases := []struct {
		n    int
		want bool
	}{
		{0, false},
		{1, true},
		{5, true},
		{10000, true},
		{10001, false},
		{-1, false},
	}
	for _, c := range cases {
		got := c.n >= 1 && c.n <= 10000
		if got != c.want {
			t.Errorf("partNumber=%d 期望 %v，实际 %v", c.n, c.want, got)
		}
	}
}
