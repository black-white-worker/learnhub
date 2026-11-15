package logic

import (
	"context"
	"errors"
	"testing"

	"learningplatform/internal/app/course"
	"learningplatform/internal/model"
)

// 视频后处理"重试"接口的状态机测试（走内存存储，不依赖 MySQL）。
// 锁住两件事：只有 failed 能重新排队，以及排队后状态与错误信息都被清零。

func addCourseWithVideoForReprocess(t *testing.T, name string) (int, *course.Video) {
	t.Helper()
	c, err := course.Store.AddCourse(name, "重试用课程", "老师", "tester")
	if err != nil {
		t.Fatalf("建课失败: %v", err)
	}
	videos := []*course.Video{{Name: "第一章.mp4", Url: "http://minio/a.mp4", ObjectName: "course/1/videos/a.mp4"}}
	if err := course.Store.AddVideos(c.Id, videos); err != nil {
		t.Fatalf("加视频失败: %v", err)
	}
	return c.Id, videos[0]
}

func TestReprocessVideo_RequeuesFailed(t *testing.T) {
	courseId, v := addCourseWithVideoForReprocess(t, "重试课-失败")
	if v.ProcessStatus != model.VideoProcessPending {
		t.Fatalf("新上传的视频应处于 pending，实际 %q", v.ProcessStatus)
	}
	v.ProcessStatus = model.VideoProcessFailed
	v.ProcessError = "ffmpeg exit 1"

	resp, err := NewReprocessVideoLogic(context.Background(), newTestSvcCtx()).
		Reprocess(&course.ReprocessVideoRequest{CourseId: courseId, VideoId: v.Id})
	if err != nil {
		t.Fatalf("failed 视频应能重新排队，却报错: %v", err)
	}
	if resp.ProcessStatus != model.VideoProcessPending {
		t.Fatalf("响应里的状态应是 pending，实际 %q", resp.ProcessStatus)
	}
	if v.ProcessStatus != model.VideoProcessPending || v.ProcessError != "" {
		t.Fatalf("存储侧状态没清干净: status=%q err=%q", v.ProcessStatus, v.ProcessError)
	}
}

// done / pending / processing 都不该被"重试"打断：
// done 再转一遍是白烧 CPU，后两个本来就在队列里。
func TestReprocessVideo_OnlyFailedAccepted(t *testing.T) {
	for _, status := range []string{model.VideoProcessDone, model.VideoProcessPending, model.VideoProcessProcessing} {
		courseId, v := addCourseWithVideoForReprocess(t, "重试课-拒绝"+status)
		v.ProcessStatus = status

		_, err := NewReprocessVideoLogic(context.Background(), newTestSvcCtx()).
			Reprocess(&course.ReprocessVideoRequest{CourseId: courseId, VideoId: v.Id})
		var be *course.BizError
		if !errors.As(err, &be) || be.Code != 400 {
			t.Fatalf("状态 %q 应返回 400，实际 %v", status, err)
		}
		if v.ProcessStatus != status {
			t.Fatalf("状态 %q 被改成了 %q", status, v.ProcessStatus)
		}
	}
}

func TestReprocessVideo_NotFoundAndBadArgs(t *testing.T) {
	courseId, v := addCourseWithVideoForReprocess(t, "重试课-不存在")
	v.ProcessStatus = model.VideoProcessFailed

	l := NewReprocessVideoLogic(context.Background(), newTestSvcCtx())
	_, err := l.Reprocess(&course.ReprocessVideoRequest{CourseId: courseId, VideoId: v.Id + 999})
	var be *course.BizError
	if !errors.As(err, &be) || be.Code != 404 {
		t.Fatalf("不存在的视频应 404，实际 %v", err)
	}

	if _, err := l.Reprocess(&course.ReprocessVideoRequest{CourseId: 0, VideoId: 1}); err == nil {
		t.Fatal("courseId=0 应被拒绝")
	}
}
