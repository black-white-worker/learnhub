package logic

import (
	"context"
	"testing"

	"learningplatform/internal/app/course"
)

// 「课程观看」分支测试：播放接口取视频 + 进度读写的降级行为。
//
// 说明：ffmpeg 参数拼装（buildFaststartArgs）是 course 包内的私有函数，
// 本包是 logic，跨包访问不到，相关用例已移到 course/ffmpeg_args_test.go。

// 播放：先造一门课 + 一个视频，再取
func TestVideoPlay_Success(t *testing.T) {
	c, err := course.Store.AddCourse("观看测试课", "", "", "tester")
	if err != nil {
		t.Fatalf("准备课程失败: %v", err)
	}
	_ = course.Store.AddVideos(c.Id, []*course.Video{{Name: "a.mp4", Url: "http://x/a.mp4", Size: 10}})

	l := NewVideoPlayLogic(context.Background(), newTestSvcCtx())
	resp, err := l.Play(&course.PlayVideoRequest{CourseId: c.Id, VideoId: 1})
	if err != nil {
		t.Fatalf("期望播放成功，却报错: %v", err)
	}
	if resp.Video == nil || resp.Video.Name != "a.mp4" {
		t.Fatalf("返回视频不对: %+v", resp.Video)
	}
}

// 播放：视频不存在 → 报错
func TestVideoPlay_NotFound(t *testing.T) {
	l := NewVideoPlayLogic(context.Background(), newTestSvcCtx())
	_, err := l.Play(&course.PlayVideoRequest{CourseId: 999999, VideoId: 1})
	if err == nil {
		t.Fatal("期望播放不存在视频时报错，却没报错")
	}
}

// 进度：没有用户上下文 / 没有 MySQL 数据层时，Get 返回 0，不报错（降级）
func TestProgress_GetWithoutStore(t *testing.T) {
	l := NewVideoProgressLogic(context.Background(), newTestSvcCtx())
	resp, err := l.Get(&course.GetProgressRequest{CourseId: 1, VideoId: 1})
	if err != nil {
		t.Fatalf("期望降级也能正常返回，却报错: %v", err)
	}
	if resp.Position != 0 {
		t.Fatalf("期望无记录返回 0，实际 %d", resp.Position)
	}
}

// 进度：Save 在无数据层时原样返回位置（进度不能阻断观看）
func TestProgress_SaveWithoutStore(t *testing.T) {
	l := NewVideoProgressLogic(context.Background(), newTestSvcCtx())
	resp, err := l.Save(&course.SaveProgressRequest{CourseId: 1, VideoId: 1, Position: 42})
	if err != nil {
		t.Fatalf("期望降级也能保存成功，却报错: %v", err)
	}
	if resp.Position != 42 {
		t.Fatalf("期望返回 42，实际 %d", resp.Position)
	}
}

// 进度：即使传了 UserId，没有 CourseStore 也不能 panic（回归保护）
func TestProgress_SaveWithUserButNoStore(t *testing.T) {
	l := NewVideoProgressLogic(context.Background(), newTestSvcCtx())
	resp, err := l.Save(&course.SaveProgressRequest{
		CourseId: 1, VideoId: 1, Position: 10, UserId: 7, WatchedDelta: 10, Duration: 100,
	})
	if err != nil {
		t.Fatalf("期望不报错，实际: %v", err)
	}
	if resp.Position != 10 {
		t.Fatalf("期望回显 10，实际 %d", resp.Position)
	}
}
