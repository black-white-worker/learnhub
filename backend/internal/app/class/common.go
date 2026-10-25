// Package class 班级模块根包：放 types、鉴权辅助、错误写回等跨子包共享的纯定义。
// 不依赖 handler / logic 子包，避免循环依赖。
package class

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

// ParseJSON 手动解析 JSON 请求体。
// go-zero 默认的 httpx.Parse 会对 struct 字段做 required 校验，
// bool/int 零值和 string 空串都会触发 "field xxx is not set"，
// 前端少传一个可选字段就 400。这里直接 json.Decoder，不做 required 校验，
// 由 logic 层自己判断必填字段。
func ParseJSON(r *http.Request, v interface{}) error {
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

// CurrentUser 解析登录用户，失败返回 *BizError（401/500），由 handler 统一写响应
func CurrentUser(r *http.Request, svcCtx *svc.ServiceContext) (*CtxUser, *BizError) {
	u := user.CurrentUser(r)
	if u == nil {
		return nil, &BizError{Code: http.StatusUnauthorized, Msg: "未登录"}
	}
	uid, err := svcCtx.UserStore.GetIdByUsername(u.Username)
	if err != nil {
		return nil, &BizError{Code: http.StatusInternalServerError, Msg: "用户不存在"}
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

// RequireTeacher 校验老师/管理员身份
func RequireTeacher(svcCtx *svc.ServiceContext, classId int64, cu *CtxUser) *BizError {
	role, err := RequireClassMember(svcCtx, classId, cu)
	if err != nil {
		return err
	}
	if role != "teacher" && role != "admin" {
		return &BizError{Code: http.StatusForbidden, Msg: "需要老师权限"}
	}
	return nil
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

// WriteBizError 统一写业务错误响应：识别 *BizError 用其状态码，否则按 500。
func WriteBizError(w http.ResponseWriter, err error) {
	if be, ok := err.(*BizError); ok {
		httpx.WriteJson(w, be.Code, map[string]string{"message": be.Msg})
		return
	}
	httpx.WriteJson(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
}
