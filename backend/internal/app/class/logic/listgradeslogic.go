package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListGradesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListGradesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListGradesLogic {
	return &ListGradesLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ListGradesLogic) ListGrades(classId int64, cu *class.CtxUser) (*class.ListGradesResp, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	rows, err := l.svcCtx.ClassStore.ListGrades(classId)
	if err != nil {
		return nil, err
	}
	vW, aW, sW, _, _ := l.svcCtx.ClassStore.GetGradeRule(classId)
	for _, g := range rows {
		g.TotalScore = g.AvgScore*float64(aW)/100 + float64(g.SigninCnt*10)*float64(sW)/100 + 100*float64(vW)/100
	}
	return &class.ListGradesResp{
		Rule: map[string]int{"video": vW, "assignment": aW, "signin": sW},
		Rows: rows,
	}, nil
}
