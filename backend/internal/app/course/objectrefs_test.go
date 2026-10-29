package course

import (
	"strings"
	"testing"
)

// 对象引用收集是"不删用户数据"的前提，这里把每种对象都覆盖一遍。
// 历史事故：resources 没被收集，讲义上传 10 分钟后被孤儿清理器删掉。
func TestCourse_ReferencedObjects_CoversEveryKind(t *testing.T) {
	bucket := "learningplatform"
	c := &Course{
		Id:            7,
		CoverUrl:      "http://minio:9000/learningplatform/course/7/cover/original.jpg",
		CoverThumbUrl: "http://minio:9000/learningplatform/course/7/cover/thumb.jpg",
		Videos: []*Video{
			{Url: "http://minio:9000/learningplatform/course/7/videos/a.mp4", ObjectName: "course/7/videos/a.mp4"},
			// faststart 之后 url 指向新对象，ObjectName 仍是源文件：
			// 两个都要保留，否则源文件会被当孤儿删掉（重新转码时就没源了）。
			{Url: "http://minio:9000/learningplatform/course/7/videos/faststart/b.mp4", ObjectName: "course/7/videos/b.mp4"},
		},
		Resources: []*Resource{
			{Id: 3, Url: "http://minio:9000/learningplatform/course/7/resources/ppt.pptx", ObjectName: "course/7/resources/ppt.pptx"},
		},
	}

	got := map[string]bool{}
	for _, name := range c.ReferencedObjects(bucket) {
		got[name] = true
	}

	want := []string{
		"course/7/cover/original.jpg",
		"course/7/cover/thumb.jpg",
		"course/7/videos/a.mp4",
		"course/7/videos/faststart/b.mp4",
		"course/7/videos/b.mp4",
		"course/7/resources/ppt.pptx",
		// 预览缓存按资源 id 命名，资源还在就应保留
		PreviewObjectName(7, 3),
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("引用集合缺少 %s（现有：%v）", name, keys(got))
		}
	}
}

// 同一个对象被多处引用时只出现一次。
func TestCourse_ReferencedObjects_Dedup(t *testing.T) {
	c := &Course{
		Videos: []*Video{
			{Url: "http://h/learningplatform/course/1/videos/a.mp4", ObjectName: "course/1/videos/a.mp4"},
			{Url: "http://h/learningplatform/course/1/videos/a.mp4", ObjectName: "course/1/videos/a.mp4"},
		},
	}
	if got := c.ReferencedObjects("learningplatform"); len(got) != 1 {
		t.Fatalf("期望去重成 1 个对象，实际 %d: %v", len(got), got)
	}
}

func TestObjectNameFromURL(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		bucket string
		want   string
	}{
		{"标准直链", "http://127.0.0.1:9020/learningplatform/course/1/cover/a.jpg", "learningplatform", "course/1/cover/a.jpg"},
		{"预签名链接去 query", "http://127.0.0.1:9020/learningplatform/course/1/videos/a.mp4?X-Amz-Signature=abc", "learningplatform", "course/1/videos/a.mp4"},
		{"桶名不同", "http://h:9000/other-bucket/course/2/resources/b.pdf", "other-bucket", "course/2/resources/b.pdf"},
		{"历史 URL 无桶名", "/course/3/videos/c.mp4", "learningplatform", "course/3/videos/c.mp4"},
		{"外链原样返回空", "https://cdn.example.com/a/b.mp4", "learningplatform", ""},
		{"空串", "", "learningplatform", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ObjectNameFromURL(c.raw, c.bucket); got != c.want {
				t.Fatalf("ObjectNameFromURL(%q)=%q，期望 %q", c.raw, got, c.want)
			}
		})
	}
}

// 清理白名单：只认 course/<id>/<已知种类>/，其余一律不碰。
func TestIsManagedObject(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"course/1/cover/a.jpg", true},
		{"course/1/videos/a.mp4", true},
		{"course/1/resources/a.pdf", true},
		{"course/1/previews/3.pdf", true},
		// 未知种类 / 非课程前缀 / 层级不对，都不能删
		{"course/1/exports/a.zip", false},
		{"course/abc/videos/a.mp4", false},
		{"assignment/1/a.pdf", false},
		{"course/1/a.mp4", false},
		{"course/", false},
	}
	for _, c := range cases {
		if got := IsManagedObject(c.name); got != c.want {
			t.Fatalf("IsManagedObject(%q)=%v，期望 %v", c.name, got, c.want)
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// 防御：内容里出现 bucket 名之外的 "course/" 时仍能解析（历史数据兼容）。
func TestObjectNameFromURL_FallbackWithoutBucketMarker(t *testing.T) {
	raw := "https://legacy.example.com/static/course/9/videos/x.mp4"
	if got := ObjectNameFromURL(raw, "learningplatform"); !strings.HasPrefix(got, "course/9/") {
		t.Fatalf("期望回退解析出 course/9/...，实际 %q", got)
	}
}
