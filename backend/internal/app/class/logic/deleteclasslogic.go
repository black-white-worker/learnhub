package logic

import (
	"context"
	"fmt"

	"learningplatform/internal/app/class"
	"learningplatform/internal/domain/authz"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

type DeleteClassLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteClassLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteClassLogic {
	return &DeleteClassLogic{ctx: ctx, svcCtx: svcCtx}
}

// DeleteClass 删除班级（连带作业、考试、成绩、成员等全部从属数据）。
//
// 权限从"仅管理员"放开到"建班老师本人 + 管理员"：
// 改造前老师能删掉自己建的课，却删不掉自己建的班，只能找管理员，
// 而管理员往往不清楚那个班是谁在用。判据走 authz 的 class.delete
// （OwnerUsername = classes.created_by），所以同班另一位老师仍删不了别人的班。
func (l *DeleteClassLogic) DeleteClass(classId int64, cu *class.CtxUser, ip string) (*model.DeleteClassStats, error) {
	if be := class.RequireClassOwner(l.svcCtx, classId, cu, authz.ClassDelete); be != nil {
		return nil, be
	}
	// 先把班名取出来写日志：删完再查就查不到了，日志里只剩一个 id 没法追责
	var name string
	if cls, err := l.svcCtx.ClassStore.GetClass(classId); err == nil && cls != nil {
		name = cls.Name
	}
	stats, err := l.svcCtx.ClassStore.DeleteClass(classId)
	if err != nil {
		return nil, err
	}
	class.LogOperation(l.svcCtx, ip, cu.Id, cu.Username, "delete_class", fmt.Sprintf("class:%d", classId),
		fmt.Sprintf("%s（成员%d 作业%d 考试%d）", name, stats.Members, stats.Assignments, stats.Exams))
	return stats, nil
}
