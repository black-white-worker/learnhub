package class

import (
	"fmt"
	"net/http"

	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ---- 公告：发布/列表/删除/已读标记 ----

func createAnnouncement(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		var req struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			IsTop   bool   `json:"isTop"`
		}
		body(r, &req)
		id, err := svcCtx.ClassStore.InsertAnnouncement(&model.AnnouncementRow{
			ClassId: classId, Title: req.Title, Content: req.Content,
			PublisherId: cu.id, IsTop: req.IsTop,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id})
		logOperation(svcCtx, r, cu.id, cu.username, "publish_notice", fmt.Sprintf("class:%d", classId), req.Title)
	}
}

func listAnnouncements(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		list, err := svcCtx.ClassStore.ListAnnouncements(classId)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		// 给每条公告附加：当前用户是否已读 + 已读人数，前端据此渲染红点和老师端统计
		type annWithRead struct {
			*model.AnnouncementRow
			ReadByMe  bool `json:"readByMe"`
			ReadCount int  `json:"readCount"`
		}
		out := make([]annWithRead, 0, len(list))
		for _, a := range list {
			readIds, _ := svcCtx.ClassStore.ListReadUserIds(a.Id)
			readByMe := false
			for _, uid := range readIds {
				if uid == cu.id {
					readByMe = true
					break
				}
			}
			out = append(out, annWithRead{AnnouncementRow: a, ReadByMe: readByMe, ReadCount: len(readIds)})
		}
		httpx.WriteJson(w, 200, out)
	}
}

// markAnnouncementRead 学生/成员打开公告后调用，标记已读（幂等）
func markAnnouncementRead(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		aid := parseIntPath(r, "aid")
		if err := svcCtx.ClassStore.MarkAnnouncementRead(aid, cu.id); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}

func deleteAnnouncement(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		aid := parseIntPath(r, "aid")
		a, err := svcCtx.ClassStore.GetAnnouncement(aid)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "公告不存在"})
			return
		}
		// 仅发布者本人、该班老师或管理员可删
		if a.PublisherId != cu.id && !requireTeacher(w, r, svcCtx, a.ClassId, cu) {
			return
		}
		if err := svcCtx.ClassStore.DeleteAnnouncement(aid); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}
