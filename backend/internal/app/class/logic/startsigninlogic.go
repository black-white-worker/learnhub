package logic

import (
	"context"
	"strconv"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

type StartSigninLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewStartSigninLogic(ctx context.Context, svcCtx *svc.ServiceContext) *StartSigninLogic {
	return &StartSigninLogic{ctx: ctx, svcCtx: svcCtx}
}

// StartSigninReq 发起签到请求（问题7：签到事件化）
type StartSigninReq struct {
	Title    string `json:"title" form:"title,optional"`
	Duration int    `json:"duration" form:"duration,optional"` // 持续分钟数，默认5
}

// StartSignin 发起一次签到事件。
// 问题7：签到不再只是生成一个码，而是有标题、起止时间的事件任务；
// 发起后给班级其他成员广播一条通知。
func (l *StartSigninLogic) StartSignin(classId int64, req *StartSigninReq, cu *class.CtxUser) (map[string]interface{}, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	title := req.Title
	if title == "" {
		title = "课堂签到"
	}
	dur := req.Duration
	if dur <= 0 {
		dur = 5
	}
	now := time.Now()
	expiresAt := now.Add(time.Duration(dur) * time.Minute)
	code := class.Rand6Code()
	id, err := l.svcCtx.ClassStore.CreateSignin(&model.SigninRow{
		ClassId: classId, Title: title, StartAt: now,
		TeacherId: cu.Id, Code: code, ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, err
	}
	// 广播通知给班级其他成员
	_ = l.svcCtx.ClassStore.BroadcastNotification(
		classId, cu.Id, "signin",
		"新的签到："+title,
		"请在 "+expiresAt.Format("15:04")+" 前完成签到",
		"/class/"+strconv.FormatInt(classId, 10),
	)
	return map[string]interface{}{
		"id": id, "code": code, "title": title,
		"startAt": now, "expiresAt": expiresAt,
	}, nil
}
