package course

import (
	"context"
	"strings"
	"testing"
)

// 「课程观看」分支测试：
// 1. ffmpeg 命令参数拼装（纯函数，不依赖真实 ffmpeg）
// 2. 播放接口取视频
// 3. 进度读取 / 保存（测试 svcCtx 不带 Redis，走降级分支）

// ffmpeg 参数必须含 +faststart（moov 前置，边下边播 / 可拖动）
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

// 播放：先造一门课 + 一个视频，再取
func TestVideoPlay_Success(t *testing.T) {
	c, _ := store.AddCourse("观看测试课", "", "")
	_ = store.AddVideos(c.Id, []*Video{{Name: "a.mp4", Url: "http://x/a.mp4", Size: 10}})

	l := NewVideoPlayLogic(context.Background(), newTestSvcCtx())
	resp, err := l.Play(&PlayVideoRequest{CourseId: c.Id, VideoId: 1})
	if err != nil {
		t.Fatalf("期望播放成功，却报错: %v", err)
	}
	if resp.Video.Name != "a.mp4" {
		t.Fatalf("返回视频不对: %+v", resp.Video)
	}
}

// 播放：视频不存在 → 报错
func TestVideoPlay_NotFound(t *testing.T) {
	l := NewVideoPlayLogic(context.Background(), newTestSvcCtx())
	_, err := l.Play(&PlayVideoRequest{CourseId: 999999, VideoId: 1})
	if err == nil {
		t.Fatal("期望播放不存在视频时报错，却没报错")
	}
}

// 进度：无 Redis / 无记录时 Get 返回 0，不报错
func TestProgress_GetWithoutRedis(t *testing.T) {
	l := NewVideoProgressLogic(context.Background(), newTestSvcCtx())
	resp, err := l.Get(&GetProgressRequest{CourseId: 1, VideoId: 1})
	if err != nil {
		t.Fatalf("期望无 Redis 也能正常返回，却报错: %v", err)
	}
	if resp.Position != 0 {
		t.Fatalf("期望无记录返回 0，实际 %d", resp.Position)
	}
}

// 进度：Save 在无 Redis 时不报错，原样返回位置
func TestProgress_SaveWithoutRedis(t *testing.T) {
	l := NewVideoProgressLogic(context.Background(), newTestSvcCtx())
	resp, err := l.Save(&SaveProgressRequest{CourseId: 1, VideoId: 1, Position: 42})
	if err != nil {
		t.Fatalf("期望无 Redis 也能保存成功（降级），却报错: %v", err)
	}
	if resp.Position != 42 {
		t.Fatalf("期望返回 42，实际 %d", resp.Position)
	}
}
