package logic

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

// AdminClassLogic 管理员班级管理（总览 / 改基本信息）。
//
// 删除与成员管理没有在这里另开一套：管理员本来就能走
// DELETE /api/class/:id 与 /api/class/:id/members/:userId，
// 再造一条 admin 专用删除路径只会多一份需要维护的级联逻辑。
type AdminClassLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAdminClassLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AdminClassLogic {
	return &AdminClassLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *AdminClassLogic) requireAdmin(cu *class.CtxUser) error {
	if cu.Role != "admin" {
		return &class.BizError{Code: http.StatusForbidden, Msg: "仅管理员可管理班级"}
	}
	return nil
}

// List 返回所有班级及规模统计。
func (l *AdminClassLogic) List(cu *class.CtxUser) (interface{}, error) {
	if err := l.requireAdmin(cu); err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.ClassStore.ListAdminClasses()
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []*model.AdminClassRow{}
	}
	return rows, nil
}

// Update 改班级名称/年级/专业。
func (l *AdminClassLogic) Update(req *class.AdminUpdateClassReq, cu *class.CtxUser, ip string) error {
	if err := l.requireAdmin(cu); err != nil {
		return err
	}
	if req.Id <= 0 {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "班级 id 不正确"}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "班级名不能为空"}
	}
	// 先确认班存在：改一个不存在的 id 会静默成功 0 行，前端以为改好了
	if _, err := l.svcCtx.ClassStore.GetClass(req.Id); err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "班级不存在"}
	}
	if err := l.svcCtx.ClassStore.UpdateClassInfo(req.Id, name, strings.TrimSpace(req.Grade), strings.TrimSpace(req.Major)); err != nil {
		if model.IsDuplicateNameErr(err) {
			return &class.BizError{Code: http.StatusBadRequest, Msg: "班级名已存在，请换一个"}
		}
		return err
	}
	class.LogOperation(l.svcCtx, ip, cu.Id, cu.Username, "update_class", fmt.Sprintf("class:%d", req.Id), name)
	return nil
}
