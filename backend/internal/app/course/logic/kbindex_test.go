package logic

// 讲义 → 知识库的联动测试。
//
// 这是「AI 答疑查不到内容」的根因之一：上传讲义和知识库之间原本没有任何连接，
// 只能靠人工跑 scripts/ingest.py。这里用一个假的 ai-service 把这条链路钉死：
// 上传 → POST /api/ai/kb/ingest，删除 → POST /api/ai/kb/purge。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"learningplatform/internal/app/course"
	"learningplatform/internal/config"
	"learningplatform/internal/platform/aiservice"
)

type kbCall struct {
	path string
	body map[string]interface{}
}

// newFakeAI 起一个假 ai-service，把收到的请求投进 channel。
func newFakeAI(t *testing.T) (*httptest.Server, chan kbCall) {
	t.Helper()
	calls := make(chan kbCall, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		_ = json.Unmarshal(raw, &body)
		calls <- kbCall{path: r.URL.Path, body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"course_id":1,"resource_id":1,"accepted":true,"running":true,"reason":""}`))
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

func mustReceive(t *testing.T, ch chan kbCall, what string) kbCall {
	t.Helper()
	select {
	case c := <-ch:
		return c
	case <-time.After(5 * time.Second):
		t.Fatalf("%s：等不到 ai-service 的调用（联动没接上）", what)
		return kbCall{}
	}
}

func TestResourceUpload_TriggersKBIngest(t *testing.T) {
	srv, calls := newFakeAI(t)
	svcCtx := newTestSvcCtx()
	svcCtx.AI = aiservice.New(config.AiServiceConf{Url: srv.URL, InternalToken: "test-token"})

	id := mustAddTempCourseForResource(t, "资源联动-上传")
	up := NewResourceUploadLogic(context.Background(), svcCtx)
	resp, err := up.UploadResource(&course.UploadResourceRequest{
		CourseId:  id,
		FileName:  "第一章讲义.docx",
		FileBytes: []byte("fake docx"),
	})
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}

	call := mustReceive(t, calls, "上传后应触发知识库灌库")
	if call.path != "/api/ai/kb/ingest" {
		t.Fatalf("期望触发 /api/ai/kb/ingest，实际 %s", call.path)
	}
	// 必须精确到「这份资源」：整门课重灌会把同课其他讲义重复解析一遍
	if got := int(call.body["course_id"].(float64)); got != id {
		t.Fatalf("course_id 不对：期望 %d，实际 %d", id, got)
	}
	if got := int(call.body["resource_id"].(float64)); got != resp.Resource.Id {
		t.Fatalf("resource_id 不对：期望 %d，实际 %d", resp.Resource.Id, got)
	}
}

func TestResourceDelete_TriggersKBPurge(t *testing.T) {
	srv, calls := newFakeAI(t)
	svcCtx := newTestSvcCtx()
	svcCtx.AI = aiservice.New(config.AiServiceConf{Url: srv.URL, InternalToken: "test-token"})

	id := mustAddTempCourseForResource(t, "资源联动-删除")
	up := NewResourceUploadLogic(context.Background(), svcCtx)
	resp, err := up.UploadResource(&course.UploadResourceRequest{
		CourseId: id, FileName: "a.pdf", FileBytes: []byte("%PDF-1.4"),
	})
	if err != nil {
		t.Fatalf("前置上传失败: %v", err)
	}
	ingest := mustReceive(t, calls, "上传应触发灌库")
	if ingest.path != "/api/ai/kb/ingest" {
		t.Fatalf("前置调用不对: %s", ingest.path)
	}

	del := NewResourceDeleteLogic(context.Background(), svcCtx)
	if _, err := del.Delete(&course.DeleteResourceRequest{
		CourseId: id, ResourceId: resp.Resource.Id,
	}); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	call := mustReceive(t, calls, "删除后应清理知识库索引")
	if call.path != "/api/ai/kb/purge" {
		t.Fatalf("期望 /api/ai/kb/purge，实际 %s", call.path)
	}
	if int(call.body["resource_id"].(float64)) != resp.Resource.Id {
		t.Fatalf("清理的资源 id 不对: %s", call.body)
	}
}

// 没配 AI 时上传照样成功：索引联动是可选的旁路，不能反过来卡住主流程。
func TestResourceUpload_NoAIStillWorks(t *testing.T) {
	svcCtx := newTestSvcCtx()
	svcCtx.AI = nil

	id := mustAddTempCourseForResource(t, "资源联动-未启用AI")
	up := NewResourceUploadLogic(context.Background(), svcCtx)
	if _, err := up.UploadResource(&course.UploadResourceRequest{
		CourseId: id, FileName: "b.pdf", FileBytes: []byte("%PDF-1.4"),
	}); err != nil {
		t.Fatalf("AI 未启用时上传不该失败: %v", err)
	}
}

// ai-service 挂了也不能把用户的删除/上传变成失败：只在日志里留痕。
func TestResourceUpload_AIDownDoesNotFailUpload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"boom"}`, http.StatusInternalServerError)
	}))
	srv.Close() // 直接关掉，模拟连不上

	svcCtx := newTestSvcCtx()
	svcCtx.AI = aiservice.New(config.AiServiceConf{Url: srv.URL, InternalToken: "t"})

	id := mustAddTempCourseForResource(t, "资源联动-AI挂了")
	up := NewResourceUploadLogic(context.Background(), svcCtx)
	if _, err := up.UploadResource(&course.UploadResourceRequest{
		CourseId: id, FileName: "c.pdf", FileBytes: []byte("%PDF-1.4"),
	}); err != nil {
		t.Fatalf("AI 挂掉不该导致上传失败: %v", err)
	}
	// 给后台 goroutine 一点时间跑完，避免测试结束时它还在动
	time.Sleep(50 * time.Millisecond)
}
