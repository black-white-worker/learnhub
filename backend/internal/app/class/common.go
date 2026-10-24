// Package class 下的公共工具：当前登录用户解析、班级成员/老师鉴权、
// 路径参数解析、请求体解码、随机码生成。
// 这些函数被各业务 handler（班级/公告/作业/讨论/签到/成绩）共用。
package class

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
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

// rand6Code 生成6位数字随机码（邀请码/签到码通用）
// 用 crypto/rand 保证不可预测，失败时回退时间戳，不阻塞业务
func rand6Code() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// generateInviteCode 生成6位数字邀请码（crypto/rand 不可预测）
func generateInviteCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	num := binary.BigEndian.Uint32(b) % 1000000
	return fmt.Sprintf("%06d", num)
}

// ctxUser 从登录态解析出的当前用户信息
type ctxUser struct {
	username string
	role     string
	id       int64
}

// currentUser 从请求取当前登录用户，并查出 user_id
func currentUser(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext) (*ctxUser, bool) {
	u := user.CurrentUser(r)
	if u == nil {
		httpx.WriteJson(w, http.StatusUnauthorized, map[string]string{"message": "未登录"})
		return nil, false
	}
	uid, err := svcCtx.UserStore.GetIdByUsername(u.Username)
	if err != nil {
		httpx.WriteJson(w, http.StatusInternalServerError, map[string]string{"message": "用户不存在"})
		return nil, false
	}
	// 旧 session 可能没有 role 字段（V6 之前登录的），从数据库补查
	role := u.Role
	if role == "" {
		if dbUser := svcCtx.UserStore.GetUser(u.Username); dbUser != nil {
			role = dbUser.Role
		}
	}
	return &ctxUser{username: u.Username, role: role, id: uid}, true
}

// requireClassMember 校验当前用户是该班成员，返回角色
func requireClassMember(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext, classId int64, cu *ctxUser) (string, bool) {
	if cu.role == "admin" {
		return "admin", true
	}
	role, err := svcCtx.ClassStore.GetUserClassRole(classId, cu.id)
	if err != nil {
		httpx.WriteJson(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return "", false
	}
	if role == "" {
		httpx.WriteJson(w, http.StatusForbidden, map[string]string{"message": "你不在这个班级"})
		return "", false
	}
	return role, true
}

// requireTeacher 校验当前用户是该班老师或管理员
func requireTeacher(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext, classId int64, cu *ctxUser) bool {
	role, ok := requireClassMember(w, r, svcCtx, classId, cu)
	if !ok {
		return false
	}
	return role == "teacher" || role == "admin"
}

// parseIntPath 从 go-zero 路径参数取 int。
// go-zero v1.10+ 把 :xxx 路径参数存在 context 的 pathvar 里，不是标准库 r.PathValue，
// 所以必须用 pathvar.Vars(r)；旧代码用 r.PathValue 永远取到空，只能 fallback 取 URL 末段，
// 对 /api/class/2/announcements 这种带子路径的路由会把 "announcements" 当 id 解析成 0，
// 进而 requireClassMember(0, uid) 误判"你不在这个班级"。
func parseIntPath(r *http.Request, key string) int64 {
	// 1. 优先走 go-zero 官方 pathvar（:id / :userId / :aid 等）
	if vars := pathvar.Vars(r); vars != nil {
		if s := vars[key]; s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
		}
	}
	// 2. 兜底：从 URL 里找第一个纯数字段（防御性，正常不会走到这）
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for _, p := range parts {
		if n, err := strconv.ParseInt(p, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// body 把请求体 JSON 解码到 v（出错忽略，业务层自己判空）
func body(r *http.Request, v interface{}) {
	json.NewDecoder(r.Body).Decode(v)
}
