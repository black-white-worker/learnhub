package logic

import (
	"context"
	"net/http"
	"strconv"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListSigninRecordsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSigninRecordsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSigninRecordsLogic {
	return &ListSigninRecordsLogic{ctx: ctx, svcCtx: svcCtx}
}

// ListSigninRecords 返回当前进行中签到的已签/未签名单（问题7）。
// 已签：直接查 signin_records；未签：班级学生 minus 已签。
func (l *ListSigninRecordsLogic) ListSigninRecords(classId int64, cu *class.CtxUser) (interface{}, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	s, err := l.svcCtx.ClassStore.GetActiveSignin(classId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "当前没有进行中的签到"}
	}
	signed, _ := l.svcCtx.ClassStore.ListSigninRecords(s.Id)
	members, _ := l.svcCtx.ClassStore.ListMembers(classId)

	signedSet := map[int64]bool{}
	for _, r := range signed {
		signedSet[r.UserId] = true
	}
	// 未签：班级里 role=student 且不在已签名单的
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
