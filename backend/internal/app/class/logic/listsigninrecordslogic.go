package logic

import (
	"context"
	"net/http"
	"strconv"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

type ListSigninRecordsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSigninRecordsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSigninRecordsLogic {
	return &ListSigninRecordsLogic{ctx: ctx, svcCtx: svcCtx}
}

// ListSigninRecords 返回签到的已签/未签名单。
// signinId 可选：传了就查指定签到（历史签到详情），没传就查当前进行中的。
func (l *ListSigninRecordsLogic) ListSigninRecords(classId int64, signinId int64, cu *class.CtxUser) (interface{}, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	var s *model.SigninRow
	var err error
	if signinId > 0 {
		s, err = l.svcCtx.ClassStore.GetSignin(signinId)
	} else {
		s, err = l.svcCtx.ClassStore.GetActiveSignin(classId)
	}
	if err != nil || s == nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "签到不存在"}
	}
	signed, _ := l.svcCtx.ClassStore.ListSigninRecords(s.Id)
	members, _ := l.svcCtx.ClassStore.ListMembers(classId)

	signedSet := map[int64]bool{}
	for _, r := range signed {
		signedSet[r.UserId] = true
	}
	var unsigned []map[string]string
	for _, m := range members {
		if m.Role == "student" && !signedSet[m.UserId] {
			unsigned = append(unsigned, map[string]string{
				"userId":   strconv.FormatInt(m.UserId, 10),
				"username": m.Username,
				"nickname": m.Nickname,
			})
		}
	}
	return map[string]interface{}{
		"signin":   s,
		"signed":   signed,
		"unsigned": unsigned,
	}, nil
}
