package handler

// 课程模块 handler 的公共守卫 + 响应辅助。
//
// 为什么鉴权放在 handler 而不是 logic：
// 课程 logic 的方法签名是"只接收请求体"（没有登录态参数），
// 为了不破坏既有调用方和测试，这里在 HTTP 边界统一拦截一次。
// 登录用户来自中间件注入的 context，handler 无法绕过。

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/course"
	"learningplatform/internal/app/user"
	"learningplatform/internal/domain/authz"
	"learningplatform/internal/platform/middleware"
	"learningplatform/internal/platform/response"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// authorizeCourse 课程写操作统一鉴权：未登录 401、不是创建者 403、课程不存在 404。
// 返回 false 表示已经写好响应，调用方直接 return。
func authorizeCourse(w http.ResponseWriter, r *http.Request, courseId int, cap authz.Capability) bool {
	u := user.CurrentUser(r)
	if err := course.Authorize(courseId, u, cap); err != nil {
		courseError(r, w, err)
		return false
	}
	return true
}

// requireCapability 非课程维度的能力校验（例如"创建课程"只需要老师身份）。
func requireCapability(w http.ResponseWriter, r *http.Request, cap authz.Capability) bool {
	u := user.CurrentUser(r)
	if u == nil {
		courseError(r, w, course.ErrUnauthorized())
		return false
	}
	actor := authz.Actor{Username: u.Username, Role: u.Role}
	if err := actor.Require(cap, authz.Resource{}); err != nil {
		courseError(r, w, &course.BizError{Code: http.StatusForbidden, Msg: err.Error()})
		return false
	}
	return true
}

// courseError 统一错误响应：走 {code,message} 三层结构，
// 不再让 httpx.ErrorCtx 把"课程不存在"变成 500。
func courseError(r *http.Request, w http.ResponseWriter, err error) {
	response.WriteError(w, err)
}

// guardCourseMedia 课程媒体访问校验（视频 / 讲义 / 在线预览）。
//
// 与 authorizeCourse 的区别：写操作看"是不是创建者"，媒体看"是不是学习者"——
// 班内学生也要能看，所以走 course.ResolveMediaAccess 的成员规则。
// 返回 false 表示已经写好响应，调用方直接 return。
func guardCourseMedia(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext, courseId int) bool {
	actor := user.CurrentUser(r)
	c, err := course.Store.GetCourse(courseId)
	if err != nil {
		courseError(r, w, course.ErrNotFound("课程不存在"))
		return false
	}
	access := course.ResolveMediaAccess(svcCtx, actor, c)
	if access.Allowed {
		return true
	}
	code := http.StatusForbidden
	if actor == nil {
		// 未登录：让前端走"跳转登录"分支，而不是弹一个 403
		code = http.StatusUnauthorized
	}
	courseError(r, w, &course.BizError{Code: code, Msg: access.Reason})
	return false
}

// courseOK 成功响应：保持既有接口的"裸 data"结构（前端已按 data.id / data.courses 解析），
// 只有错误统一成三层。这样本次改造不需要重写全部前端页面。
func courseOK(r *http.Request, w http.ResponseWriter, data interface{}) {
	httpx.OkJsonCtx(r.Context(), w, data)
}

// parseJSONBody 宽松解析 JSON 请求体：空 body 不报错，交由业务层校验必填项。
// 不用 httpx.Parse 的原因同 class.ParseJSON：它对 struct 字段做 required 校验，
// 可选字段缺失会直接 400。
func parseJSONBody(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if middleware.BodyTooLarge(err) {
			return &course.BizError{Code: http.StatusRequestEntityTooLarge, Msg: "请求体过大"}
		}
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

// courseIdFromQuery 从 query 取课程 id。
//
// 上传类接口要求 ?courseId=N：只有这样才能在解析 multipart 之前完成鉴权。
// 旧实现先 ParseMultipartForm 再判断"你是不是这门课的主人"，
// 未授权请求也会先把整个文件落到临时目录（最大 1GB），是可被利用的磁盘/内存消耗点。
func courseIdFromQuery(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.URL.Query().Get("courseId"))
	if err != nil || id <= 0 {
		courseError(r, w, course.ErrBadRequest("courseId 必须通过 query 传入且为正整数"))
		return 0, false
	}
	return id, true
}

// parseMultipartForm 解析表单，并把"超过请求体上限"翻译成 413。
func parseMultipartForm(w http.ResponseWriter, r *http.Request, maxMemory int64) bool {
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		if middleware.BodyTooLarge(err) {
			courseError(r, w, &course.BizError{Code: http.StatusRequestEntityTooLarge, Msg: "上传内容超过大小上限"})
			return false
		}
		courseError(r, w, course.ErrBadRequest("表单解析失败："+err.Error()))
		return false
	}
	return true
}

// reserveUploadQuota 预占当天的上传额度，返回"归还函数"。
//
// 调用方在后续任何失败路径上调用 release()；上传成功则保留额度。
// 配额未启用 / Redis 不可用时 release 是空操作，绝不挡住正常上传。
func reserveUploadQuota(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext, add int64) (release func(), ok bool) {
	release = func() {}
	if svcCtx.Quota == nil || !svcCtx.Quota.Enabled() {
		return release, true
	}
	uid := currentUserId(r, svcCtx)
	if uid <= 0 {
		// 拿不到用户 id（理论上不会：路由都挂了鉴权中间件）就不限制
		return release, true
	}
	if err := svcCtx.Quota.Reserve(uid, add); err != nil {
		courseError(r, w, err)
		return release, false
	}
	return func() { svcCtx.Quota.Release(uid, add) }, true
}

// currentUserId 从会话里取数据库用户 id，取不到返回 0。
func currentUserId(r *http.Request, svcCtx *svc.ServiceContext) int64 {
	cu, be := class.CurrentUser(r, svcCtx)
	if be != nil || cu == nil {
		return 0
	}
	return cu.Id
}
