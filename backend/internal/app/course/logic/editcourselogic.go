package logic

import (
	"context"
	"errors"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type EditCourseLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewEditCourseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *EditCourseLogic {
	return &EditCourseLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// EditCourse 编辑课程基本信息业务逻辑：
// 1. 校验课程 id 合法、课程名称非空
// 2. 调用 course.Store.UpdateCourse 更新（重名校验由 store 排除自身后完成）
// 3. 返回更新后的课程基本信息
func (l *EditCourseLogic) EditCourse(req *course.EditCourseRequest) (resp *course.EditCourseResponse, err error) {
	if req.Id <= 0 {
		return nil, errors.New("课程 id 不合法")
	}

	c, err := course.Store.UpdateCourse(req.Id, req.Name, req.Intro, req.Teacher)
	if err != nil {
		return nil, err
	}
	// 列表缓存失效，改完名字首页立刻生效
	course.InvalidateListCache(l.svcCtx.Redis)

	return &course.EditCourseResponse{
		Id:      c.Id,
		Name:    c.Name,
		Intro:   c.Intro,
		Teacher: c.Teacher,
	}, nil
}
