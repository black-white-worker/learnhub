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

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/course"
	"learningplatform/internal/app/user"
	"learningplatform/internal/authz"
	"learningplatform/internal/response"
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
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

// uploadQuotaGuard 上传配额：按"用户 + 自然日"限制总上传量。
//
// 只做一件事：预占 add 字节。超限返回 413 + 中文提示（"今日上传额度不足…"）。
// 配额未配置（0）或 Redis 不可用时直接放行，绝不因为防护措施挡住正常上传。
func uploadQuotaGuard(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext, add int64) bool {
	if svcCtx.Quota == nil || !svcCtx.Quota.Enabled() {
		return true
	}
	uid := currentUserId(r, svcCtx)
	if uid <= 0 {
		// 拿不到用户 id（理论上不会：路由都挂了鉴权中间件）就不限制
		return true
	}
	if err := svcCtx.Quota.Reserve(uid, add); err != nil {
		courseError(r, w, err)
		return false
	}
	return true
}

// currentUserId 从会话里取数据库用户 id，取不到返回 0。
func currentUserId(r *http.Request, svcCtx *svc.ServiceContext) int64 {
	cu, be := class.CurrentUser(r, svcCtx)
	if be != nil || cu == nil {
		return 0
	}
	return cu.Id
}
