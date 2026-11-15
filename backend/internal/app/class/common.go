// Package class 班级模块根包：放 types、鉴权辅助、错误写回等跨子包共享的纯定义。
// 不依赖 handler / logic 子包，避免循环依赖。
package class

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"learningplatform/internal/app/user"
	"learningplatform/internal/domain/authz"
	"learningplatform/internal/platform/middleware"
	"learningplatform/internal/platform/response"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/pathvar"
)

// ParseJSON 手动解析 JSON 请求体。
// go-zero 默认的 httpx.Parse 会对 struct 字段做 required 校验，
// bool/int 零值和 string 空串都会触发 "field xxx is not set"，
// 前端少传一个可选字段就 400。这里直接 json.Decoder，不做 required 校验，
// 由 logic 层自己判断必填字段。
//
// 请求体大小由 middleware.BodyLimit 统一限制，这里只负责把"超限"翻译成 413，
// 而不是笼统的 500。
func ParseJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if middleware.BodyTooLarge(err) {
			return &BizError{Code: http.StatusRequestEntityTooLarge, Msg: "请求体过大"}
		}
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

// Rand6Code 生成6位数字随机码（邀请码/签到码通用）
func Rand6Code() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// GenerateInviteCode 生成6位数字邀请码
func GenerateInviteCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	num := binary.BigEndian.Uint32(b) % 1000000
	return fmt.Sprintf("%06d", num)
}

// CtxUser 从登录态解析出的当前用户信息（handler/logic 共用）
type CtxUser struct {
	Username string
	Role     string
	Id       int64
}

// CurrentUser 解析登录用户，失败返回 *BizError（401/500），由 handler 统一写响应。
//
// 性能：会话（Redis）里已带 id 时直接用，不再每请求查一次 users 表。
// 只有滚动发布前写入的老会话（id=0）才回落到 GetIdByUsername，
// 这样上线新版本不会要求所有人重新登录。
func CurrentUser(r *http.Request, svcCtx *svc.ServiceContext) (*CtxUser, *BizError) {
	u := user.CurrentUser(r)
	if u == nil {
		return nil, &BizError{Code: http.StatusUnauthorized, Msg: "未登录"}
	}
	uid := u.Id
	if uid <= 0 {
		var err error
		uid, err = svcCtx.UserStore.GetIdByUsername(u.Username)
		if err != nil {
			return nil, &BizError{Code: http.StatusInternalServerError, Msg: "用户不存在"}
		}
	}
	role := u.Role
	if role == "" {
		if dbUser := svcCtx.UserStore.GetUser(u.Username); dbUser != nil {
			role = dbUser.Role
		}
	}
	return &CtxUser{Username: u.Username, Role: role, Id: uid}, nil
}

// RequireClassMember 校验班级成员身份，返回该用户在班角色。失败返回 *BizError（403/500）。
func RequireClassMember(svcCtx *svc.ServiceContext, classId int64, cu *CtxUser) (string, *BizError) {
	if cu.Role == "admin" {
		return "admin", nil
	}
	role, err := svcCtx.ClassStore.GetUserClassRole(classId, cu.Id)
	if err != nil {
		return "", &BizError{Code: http.StatusInternalServerError, Msg: err.Error()}
	}
	if role == "" {
		return "", &BizError{Code: http.StatusForbidden, Msg: "你不在这个班级"}
	}
	return role, nil
}

// RequireClassTeacher 班级范围内的权限校验（统一走 authz 能力矩阵）。
//
// 为什么不让各模块自己写 if role != "teacher"：
//
//	改造前班级模块用裸字符串判断、考试模块又自己写了一版、课程模块干脆没写，
//	三套标准必然漂移（实际就漂成了"作业详情看全局角色、考试详情看班内角色"）。
//	现在所有"班级里的老师才能做"的判定都收敛到这一条路径。
func RequireClassTeacher(svcCtx *svc.ServiceContext, classId int64, cu *CtxUser, cap authz.Capability) *BizError {
	role, be := RequireClassMember(svcCtx, classId, cu)
	if be != nil {
		return be
	}
	actor := authz.Actor{UserId: cu.Id, Username: cu.Username, Role: cu.Role, ClassRole: role}
	if err := actor.Require(cap, authz.Resource{ClassRole: role}); err != nil {
		return AsBizError(err)
	}
	return nil
}

// RequireTeacher 校验老师/管理员身份（班级管理类能力的通用入口）。
func RequireTeacher(svcCtx *svc.ServiceContext, classId int64, cu *CtxUser) *BizError {
	return RequireClassTeacher(svcCtx, classId, cu, authz.ClassManage)
}

// RequireClassOwner 用于"班级本身是操作对象"的能力（目前只有 class.delete）。
//
// 和 RequireClassTeacher 的区别：后者的判据是"我在这个班里是不是老师"，
// 适合改作业、发公告这类班内事务；删班要看的是"这个班是不是我建的"，
// 资源属性来自 classes.created_by，班内角色只是佐证（得先证明你在班里）。
// 管理员照例全放行，作为建班人离职/失联时的兜底。
func RequireClassOwner(svcCtx *svc.ServiceContext, classId int64, cu *CtxUser, cap authz.Capability) *BizError {
	cls, err := svcCtx.ClassStore.GetClass(classId)
	if err != nil || cls == nil {
		return &BizError{Code: http.StatusNotFound, Msg: "班级不存在"}
	}

	classRole := "admin"
	if cu.Role != "admin" {
		// 先确认在班内：全局老师也可能和这个班毫无关系，
		// 直接拿 users.role 判会变成"任何老师都能删任何班"。
		r, be := RequireClassMember(svcCtx, classId, cu)
		if be != nil {
			return be
		}
		classRole = r
	}

	actor := authz.Actor{UserId: cu.Id, Username: cu.Username, Role: cu.Role, ClassRole: classRole}
	if err := actor.Require(cap, authz.Resource{ClassRole: classRole, OwnerUsername: cls.CreatedBy}); err != nil {
		return AsBizError(err)
	}
	return nil
}

// QuitDenyReason 判断某班内角色能否主动退出班级，返回拒绝原因（空串=可以退出）。
//
// 抽成纯函数是为了让规则可测：退出接口本身要查库，
// 但"谁能退"这条判断不该被数据库挡住才验证。
// 学生退出只解除成员关系；老师退出会把班变成没人管的状态，
// 正确路径是删班或移交管理员。
func QuitDenyReason(classRole string) string {
	switch classRole {
	case "teacher":
		return "老师不能退出自己任教的班级，请删除班级或由管理员处理"
	case "admin":
		return "管理员不是班级成员，无需退出"
	}
	return ""
}

// ParseIntPath 从 go-zero 路径参数取 int。
func ParseIntPath(r *http.Request, key string) int64 {
	if vars := pathvar.Vars(r); vars != nil {
		if s := vars[key]; s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
		}
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for _, p := range parts {
		if n, err := strconv.ParseInt(p, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// AsBizError 把任意 error 转成 *BizError，便于 logic 层混用不同来源的拒绝错误。
//
// 典型场景：authz.Require 返回 *authz.Denied（403），
// 而 logic 的返回类型是 *BizError，这里统一转换，不需要在每处手写 if。
func AsBizError(err error) *BizError {
	if err == nil {
		return nil
	}
	var be *BizError
	if errors.As(err, &be) {
		return be
	}
	// 实现了 StatusCode() 的错误（如 *authz.Denied）按它给的状态码输出
	var coded interface{ StatusCode() int }
	if errors.As(err, &coded) {
		return &BizError{Code: coded.StatusCode(), Msg: err.Error()}
	}
	return &BizError{Code: http.StatusInternalServerError, Msg: err.Error()}
}

// StatusCode 实现 response.codedError 接口，让 response.Error 能识别 BizError。
// 不直接复用 Code 字段是为了语义分离：Code 既是 HTTP 状态码也是业务码，
// 这里统一返回 Code，response 包会同时用作 HTTP 状态码和 body.code。
func (e *BizError) StatusCode() int { return e.Code }

// WriteBizError 统一写业务错误响应：委托到 response 包，输出 {code,message,data} 三层结构。
// 历史上直接写 {"message":...}，现已统一为三层，前端拦截器按 code===0 判断成功即可。
func WriteBizError(w http.ResponseWriter, err error) {
	response.WriteError(w, err)
}

// MarshalQuestionScores 把老师「逐题最终评分」序列化进 question_scores 列（作业 / 考试共用）。
//
// 空 map 存空串而不是 "{}"：前端需要区分"没批过逐题，可以拿 AI 初评回填"和
// "批过，且这次每道题都是 0 分" —— 存成 "{}" 两种就分不开了。
// 负数直接拒绝：分数为负只可能是前端算错，落库后会让总分对不上。
func MarshalQuestionScores(scores map[string]float64) (string, error) {
	if len(scores) == 0 {
		return "", nil
	}
	for qid, v := range scores {
		if v < 0 {
			return "", &BizError{Code: http.StatusBadRequest, Msg: "题目 " + qid + " 的分数不能为负"}
		}
	}
	b, err := json.Marshal(scores)
	if err != nil {
		return "", &BizError{Code: http.StatusBadRequest, Msg: "逐题分数格式不正确"}
	}
	return string(b), nil
}
