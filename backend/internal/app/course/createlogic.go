package course

import (
	"context"
	"errors"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateLogic {
	return &CreateLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Create 课程创建业务逻辑（模拟"同事李四"开发的功能）：
// 1. 基础校验：课程名称不能为空
// 2. 调用 store.AddCourse 写入课程存储（名称重复时 AddCourse 返回错误）
// 3. 成功则返回新课程的 id 和名称
func (l *CreateLogic) Create(req *CreateRequest) (resp *CreateResponse, err error) {
	// 1. 基础校验：课程名称不能为空
	if req.Name == "" {
		return nil, errors.New("课程名称不能为空")
	}

	// 2. 写入课程存储（重复课程名由 AddCourse 拦截）
	course, err := store.AddCourse(req.Name, req.Intro, req.Teacher)
	if err != nil {
		return nil, err
	}

	// 3. 创建成功，返回新课程信息
	return &CreateResponse{Id: course.Id, Name: course.Name}, nil
}
