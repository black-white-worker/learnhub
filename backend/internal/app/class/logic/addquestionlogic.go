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

func (l *AddQuestionLogic) AddQuestion(aid int64, req *class.AddQuestionReq, cu *class.CtxUser) (*class.AddQuestionResp, error) {
	a, err := l.svcCtx.ClassStore.GetAssignment(aid)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
		return nil, be
	}
	if req.Stem == "" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "题干不能为空"}
	}
	// 题型默认 single；essay 主观题不需要标准答案，老师手批
	if req.Type == "" {
		req.Type = "single"
	}
	if req.Type != "essay" && req.Answer == "" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "选择题/判断题答案不能为空"}
	}
	if req.Score <= 0 {
		req.Score = 5
	}
	id, err := l.svcCtx.ClassStore.InsertQuestion(&model.QuestionRow{
		AssignmentId: aid, Type: req.Type, Stem: req.Stem, Options: req.Options, Answer: req.Answer, Score: req.Score,
	})
	if err != nil {
		return nil, err
	}
	return &class.AddQuestionResp{Id: id}, nil
}
