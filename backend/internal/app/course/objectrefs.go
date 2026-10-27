package course

// 课程对象的生命周期：哪些 MinIO 对象正在被引用。
//
// 为什么要单独一个文件：
// 孤儿清理器（orphansweeper.go）会删除"没有被任何课程引用"的 course/ 对象。
// 引用集合只要漏掉一类对象，那类文件就会在上传 10 分钟后被静默删除
// （历史 bug：漏了 resources，讲义全被删）。所以"引用"必须集中在一处定义，
// 任何新增的对象类型都只改这里，并补一条对应测试。
//
// 长期方向（参考 Moodle 的 mdl_files）：对象表是唯一事实来源，
// 上传时登记、删除时按引用计数回收，不再靠扫 URL 反推。
// 在当前规模下，先保证"所有落库字段都被收集"这个不变量。

import (
	"strconv"
	"strings"
)

// ManagedObjectPrefixes 孤儿清理器允许清理的对象前缀（相对 bucket 根）。
// 不在白名单里的对象一律不删：新增一种对象（导出、缓存、临时文件）时，
// 即使忘了登记引用，最坏也只是留下垃圾，不会误删用户数据。
var ManagedObjectPrefixes = []string{"cover/", "videos/", "resources/", "previews/"}

// ObjectNameFromURL 从对象 URL 里反解桶内路径。
//
// 支持三种形态：
//   - http://host/bucket/course/1/cover/x.jpg  （正常直链）
//   - http://host/bucket/course/1/x.jpg?X-Amz-...（预签名链接，去掉 query）
//   - /course/1/cover/x.jpg                     （历史数据没带桶名）
func ObjectNameFromURL(raw, bucket string) string {
	if raw == "" {
		return ""
	}
	if bucket != "" {
		if i := strings.Index(raw, "/"+bucket+"/"); i >= 0 {
			return trimURLTail(raw[i+len(bucket)+2:])
		}
	}
	if i := strings.Index(raw, "/course/"); i >= 0 {
		return trimURLTail(raw[i+1:])
	}
	return ""
}

// trimURLTail 去掉查询串和锚点，只保留对象路径。
func trimURLTail(s string) string {
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	return s
}

// PreviewObjectName 讲义的 PDF 预览缓存对象名。
// 由课程 id + 资源 id 决定，所以可以随时重算（清理器据此判断是否仍被引用）。
func PreviewObjectName(courseId, resourceId int) string {
	return "course/" + strconv.Itoa(courseId) + "/previews/" + strconv.Itoa(resourceId) + ".pdf"
}

// ReferencedObjects 返回一门课程当前引用的全部对象名（已去重）。
// bucket 用来解析历史 URL；ObjectName 字段优先，因为它是写入时记下的准确路径。
func (c *Course) ReferencedObjects(bucket string) []string {
	if c == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 2+len(c.Videos)+len(c.Resources))

	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}

	add(ObjectNameFromURL(c.CoverUrl, bucket))
	add(ObjectNameFromURL(c.CoverThumbUrl, bucket))
	for _, v := range c.Videos {
		if v == nil {
			continue
		}
		add(ObjectNameFromURL(v.Url, bucket))
		add(v.ObjectName)
	}
	for _, r := range c.Resources {
		if r == nil {
			continue
		}
		add(ObjectNameFromURL(r.Url, bucket))
		add(r.ObjectName)
		// 预览缓存按资源 id 命名，资源还在就说明缓存仍可复用。
		if r.Id > 0 {
			add(PreviewObjectName(c.Id, r.Id))
		}
	}
	return out
}

// IsManagedObject 判断对象是否落在白名单前缀里（course/<id>/<kind>/...）。
//
// 只认"course/数字/已知种类/"这一层之后的路径，避免误伤将来新增的
// course/<id>/exports/ 之类对象。
func IsManagedObject(name string) bool {
	parts := strings.Split(name, "/")
	if len(parts) < 4 || parts[0] != "course" {
		return false
	}
	if !isAllDigits(parts[1]) || parts[1] == "" {
		return false
	}
	kind := parts[2] + "/"
	for _, p := range ManagedObjectPrefixes {
		if kind == p {
			return true
		}
	}
	return false
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
