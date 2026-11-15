package handler

import (
	"testing"

	"learningplatform/internal/app/course"
)

// 清单是 ai-service 灌库的唯一数据源，字段名/口径错了就是「页面有讲义、AI 说没资料」，
// 所以这里把映射规则钉死。
func TestBuildKbManifestMapsResourcesAndVideos(t *testing.T) {
	c := &course.Course{
		Id:   7,
		Name: "前端开发入门",
		Resources: []*course.Resource{
			{Id: 6, Name: "实验八_软间隔SVM_实验报告.docx", Url: "http://minio/r6.docx",
				ObjectName: "course/7/resources/r6.docx", Type: "docx", Size: 842},
			nil, // 脏数据不能把整份清单搞崩
		},
		Videos: []*course.Video{
			{Id: 3, Name: "第一讲 Vue3 起步.mp4", Url: "http://minio/v3.mp4",
				ObjectName: "course/7/videos/v3.mp4", Size: 1048576},
		},
	}

	got := buildKbManifest(nil, c)

	if got.CourseId != 7 || got.CourseName != "前端开发入门" {
		t.Fatalf("课程头不对: %+v", got)
	}
	if len(got.Resources) != 1 {
		t.Fatalf("资源条数应为 1（nil 跳过），实际 %d", len(got.Resources))
	}
	r := got.Resources[0]
	if r.Id != 6 || r.Type != "docx" || r.ObjectName != "course/7/resources/r6.docx" || r.Size != 842 {
		t.Fatalf("资源字段映射不对: %+v", r)
	}
	// 公开桶 / 未配置私有桶：URL 必须原样透出，否则 ai-service 取不到文件
	if r.Url != "http://minio/r6.docx" {
		t.Fatalf("公开桶 URL 应原样返回，实际 %q", r.Url)
	}
	if len(got.Videos) != 1 {
		t.Fatalf("视频条数应为 1，实际 %d", len(got.Videos))
	}
	v := got.Videos[0]
	// 视频没有 type 列，清单里统一标成 video，ai-service 才好区分讲义与视频
	if v.Type != "video" || v.Name != "第一讲 Vue3 起步.mp4" {
		t.Fatalf("视频字段映射不对: %+v", v)
	}
}

// 没上传任何资料的课程要返回空数组而不是 null：
// 前端 / Python 侧按 list 直接遍历，null 会让 json 解析出 None 再炸在 len() 上。
func TestBuildKbManifestEmptyCourseIsEmptySlices(t *testing.T) {
	got := buildKbManifest(nil, &course.Course{Id: 9, Name: "空课程"})
	if got.Resources == nil || got.Videos == nil {
		t.Fatalf("空课程应返回空切片而非 nil: %+v", got)
	}
	if len(got.Resources) != 0 || len(got.Videos) != 0 {
		t.Fatalf("空课程不该有资料: %+v", got)
	}
}
