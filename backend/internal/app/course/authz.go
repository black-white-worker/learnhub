package course

import (
	"errors"
	"net/http"

	"learningplatform/internal/domain/authz"
	"learningplatform/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// BizError 课程模块的业务错误：携带 HTTP 状态码，交给 response.WriteError 输出。
//
// 为什么不用 httpx.ErrorCtx：它一律按 500 处理，前端会看到"服务器错误"，
// 但"这不是你的课"应该是 403、"课程不存在"应该是 404。
type BizError struct {
	Code int
	Msg  string
}

// Error 实现 error 接口。
func (e *BizError) Error() string { return e.Msg }

// StatusCode 让 response 包按业务状态码输出 {code,message}。
func (e *BizError) StatusCode() int { return e.Code }

// 常用错误构造器（少写点重复的字面量）。
func ErrForbidden(msg string) *BizError { return &BizError{Code: http.StatusForbidden, Msg: msg} }
func ErrNotFound(msg string) *BizError  { return &BizError{Code: http.StatusNotFound, Msg: msg} }
func ErrBadRequest(msg string) *BizError {
	return &BizError{Code: http.StatusBadRequest, Msg: msg}
}
func ErrUnauthorized() *BizError {
	return &BizError{Code: http.StatusUnauthorized, Msg: "未登录"}
}
func ErrInternal(msg string) *BizError {
	return &BizError{Code: http.StatusInternalServerError, Msg: msg}
}

// ErrCourseNotFound 表示"库里确实没有这门课"，与"查库失败"严格区分。
var ErrCourseNotFound = errors.New("课程不存在")

// CourseLookupError 把 Store.GetCourse 的错误翻译成对外的业务错误。
//
// 只有 ErrCourseNotFound 才是 404；连接失败、扫描失败等一律 500 并落日志。
// 改造前这里（以及各 logic 层）把任何错误都改写成"课程不存在"，
// 于是 go-zero 的 scan 报错在日志里查不到任何痕迹，排查绕了一大圈。
func CourseLookupError(courseId int, err error) error {
	if errors.Is(err, ErrCourseNotFound) {
		return ErrNotFound("课程不存在")
	}
	logx.Errorf("读取课程 %d 失败: %v", courseId, err)
	return ErrInternal("课程读取失败，请稍后重试")
}

// actorOf 用登录会话构造授权主体。
// 课程模块不区分班内角色，只用到全局角色 + 用户名（判断是否创建者）。
func actorOf(u *model.SessionUser) authz.Actor {
	if u == nil {
		return authz.Actor{}
	}
	return authz.Actor{Username: u.Username, Role: u.Role}
}

// Authorize 课程写操作的统一入口：校验"这个登录用户能不能对这门课做这件事"。
//
// 改造前：编辑 / 删除 / 上传 / 删资源 只挂了登录中间件，任何学生都能删任意课程。
// 现在所有课程写接口在 handler 里先调这里，能力矩阵在 authz 包统一定义。
//
// 注意 u 为 nil（未登录）时直接 401，不依赖中间件顺序。
func Authorize(courseId int, u *model.SessionUser, cap authz.Capability) error {
	if u == nil {
		return ErrUnauthorized()
	}
	return AuthorizeAs(u.Username, u.Role, courseId, cap)
}

// AuthorizeAs 用裸的用户名 + 全局角色鉴权。
// 给已经拿到 class.CtxUser（没有 *model.SessionUser）的调用方用，避免来回转换。
func AuthorizeAs(username, role string, courseId int, cap authz.Capability) error {
	c, err := Store.GetCourse(courseId)
	if err != nil {
		return CourseLookupError(courseId, err)
	}
	actor := authz.Actor{Username: username, Role: role}
	if err := actor.Require(cap, authz.Resource{OwnerUsername: c.CreatedBy}); err != nil {
		return &BizError{Code: http.StatusForbidden, Msg: err.Error()}
	}
	return nil
}
