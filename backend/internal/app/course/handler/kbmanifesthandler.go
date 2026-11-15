package handler

// 知识库资料清单：GET /api/internal/course/:courseId/kb-manifest
//
// 只有 ai-service 会调（X-Internal-Token 鉴权），返回这门课「页面上能看到的」
// 全部讲义 + 视频元信息，作为灌库与答疑的唯一数据源。
//
// 与 /api/course/resources/:courseId 的关系：数据同源（course.Store.GetCourse），
// 区别只是鉴权方式 —— 浏览器那条按登录态 + 选课关系判，这条按服务间共享密钥判。
// 之所以不直接让 ai-service 用那条：它手里没有用户 JWT，也不该为了灌库
// 去伪造某个学生/老师的身份。

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// KbManifestHandler 返回课程资料清单。
func KbManifestHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req course.KbManifestRequest
		if err := httpx.Parse(r, &req); err != nil {
			courseError(r, w, course.ErrBadRequest("参数错误"))
			return
		}
		if req.CourseId <= 0 {
			courseError(r, w, course.ErrBadRequest("courseId 必须为正整数"))
			return
		}
		c, err := course.Store.GetCourse(req.CourseId)
		if err != nil {
			courseError(r, w, course.CourseLookupError(req.CourseId, err))
			return
		}
		httpx.OkJsonCtx(r.Context(), w, buildKbManifest(svcCtx, c))
	}
}

// buildKbManifest 把课程实体映射成清单。抽成纯函数便于单测（不经过路由）。
//
// MediaURL 与课程详情页同一条规则：私有桶把 URL 换成预签名地址，
// ai-service 拿到的地址可以直接 GET，不必再配一套 MinIO 凭据。
func buildKbManifest(svcCtx *svc.ServiceContext, c *course.Course) *course.KbManifestResponse {
	resp := &course.KbManifestResponse{
		CourseId:   c.Id,
		CourseName: c.Name,
		Resources:  make([]course.KbManifestItem, 0, len(c.Resources)),
		Videos:     make([]course.KbManifestItem, 0, len(c.Videos)),
	}
	for _, res := range c.Resources {
		if res == nil {
			continue
		}
		resp.Resources = append(resp.Resources, course.KbManifestItem{
			Id:         res.Id,
			Name:       res.Name,
			Url:        course.MediaURL(svcCtx, res.ObjectName, res.Url),
			ObjectName: res.ObjectName,
			Type:       res.Type,
			Size:       res.Size,
			UploadedAt: res.UploadedAt,
		})
	}
	for _, v := range c.Videos {
		if v == nil {
			continue
		}
		resp.Videos = append(resp.Videos, course.KbManifestItem{
			Id:         v.Id,
			Name:       v.Name,
			Url:        course.MediaURL(svcCtx, v.ObjectName, v.Url),
			ObjectName: v.ObjectName,
			Type:       "video",
			Size:       v.Size,
			UploadedAt: v.UploadedAt,
		})
	}
	return resp
}
