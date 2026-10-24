package class

import (
	"net/http"
	"time"

	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ---- 班级签到：发起/当前进行中/学生凭码签到/名单 ----

// startSignin 老师发起一次5分钟限时签到
func startSignin(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		code := rand6Code()
		expiresAt := time.Now().Add(5 * time.Minute)
		id, err := svcCtx.ClassStore.CreateSignin(&model.SigninRow{
			ClassId: classId, TeacherId: cu.id, Code: code, ExpiresAt: expiresAt,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id, "code": code, "expiresAt": expiresAt})
	}
}

// getActiveSignin 查当前进行中的签到
func getActiveSignin(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		s, err := svcCtx.ClassStore.GetActiveSignin(classId)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "当前没有进行中的签到"})
			return
		}
		httpx.WriteJson(w, 200, s)
	}
}

// doSignin 学生凭码签到
func doSignin(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		var req struct {
			Code string `json:"code"`
		}
		body(r, &req)
		s, err := svcCtx.ClassStore.GetSigninByCode(req.Code)
		if err != nil || s.ClassId != classId {
			httpx.WriteJson(w, 400, map[string]string{"message": "签到码错误"})
			return
		}
		if time.Now().After(s.ExpiresAt) {
			httpx.WriteJson(w, 400, map[string]string{"message": "签到已过期"})
			return
		}
		// 用 RowsAffected 判断是否真的新插入：唯一键冲突说明该学生已签到过
		inserted, err := svcCtx.ClassStore.AddSigninRecord(s.Id, cu.id)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		if !inserted {
			httpx.WriteJson(w, 409, map[string]string{"message": "你已经签到过了，请勿重复签到"})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "签到成功"})
	}
}

// listSigninRecords 老师查看已签名单
func listSigninRecords(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		s, err := svcCtx.ClassStore.GetActiveSignin(classId)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "当前没有进行中的签到"})
			return
		}
		records, err := svcCtx.ClassStore.ListSigninRecords(s.Id)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, records)
	}
}
