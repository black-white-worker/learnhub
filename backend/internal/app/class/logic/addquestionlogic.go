package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/model"
	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type AddQuestionLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAddQuestionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddQuestionLogic {
	return &AddQuestionLogic{ctx: ctx, svcCtx: svcCtx}
}

// AddQuestion 老师给作业加题（空题卡，对标腾讯问卷）。
// 允许题干先空着，前端点题型栏追加一张空卡，再即时编辑。
func (l *AddQuestionLogic) AddQuestion(aid int64, req *class.AddQuestionReq, cu *class.CtxUser) (*class.AddQuestionResp, error) {
	a, err := l.svcCtx.ClassStore.GetAssignment(aid)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
		return nil, be
	}
	qt := req.Type
	if qt == "" {
		qt = "essay"
	}
	allowed := map[string]bool{"single": true, "multiple": true, "fill": true, "essay": true, "fileupload": true}
	if !allowed[qt] {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "题型不支持"}
	}
	score := req.Score
	if score <= 0 {
		score = 5
	}
	required := 1
	if req.Required != nil {
		required = *req.Required
	}
	maxOrder, _ := l.svcCtx.ClassStore.MaxAssignmentQuestionOrder(aid)
	id, err := l.svcCtx.ClassStore.InsertQuestion(&model.QuestionRow{
		AssignmentId: aid, Type: qt, Stem: req.Stem, Options: req.Options, Answer: req.Answer,
		Score: score, OrderNum: maxOrder + 1, Required: required,
	})
	if err != nil {
		return nil, err
	}
	return &class.AddQuestionResp{Id: id}, nil
}
