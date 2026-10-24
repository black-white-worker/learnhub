package class

import (
	"net/http"

	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ---- 讨论：发帖/列表/详情/删除/回复 ----

func createDiscussion(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		var req struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		body(r, &req)
		id, err := svcCtx.ClassStore.InsertDiscussion(&model.DiscussionRow{
			ClassId: classId, UserId: cu.id, Title: req.Title, Content: req.Content,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id})
	}
}

func listDiscussions(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		list, _ := svcCtx.ClassStore.ListDiscussions(classId, 20, 0)
		httpx.WriteJson(w, 200, list)
	}
}

func discussionDetail(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		did := parseIntPath(r, "did")
		d, err := svcCtx.ClassStore.GetDiscussion(did)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "帖子不存在"})
			return
		}
		if _, ok := requireClassMember(w, r, svcCtx, d.ClassId, cu); !ok {
			return
		}
		replies, _ := svcCtx.ClassStore.ListReplies(did)
		httpx.WriteJson(w, 200, map[string]interface{}{"discussion": d, "replies": replies})
	}
}

func deleteDiscussion(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		did := parseIntPath(r, "did")
		d, err := svcCtx.ClassStore.GetDiscussion(did)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "帖子不存在"})
			return
		}
		// 作者本人可删自己的帖子；否则需该班老师/管理员
		isOwner := d.UserId == cu.id
		if !isOwner && !requireTeacher(w, r, svcCtx, d.ClassId, cu) {
			return
		}
		if err := svcCtx.ClassStore.DeleteDiscussion(did); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}

func replyDiscussion(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		did := parseIntPath(r, "did")
		d, err := svcCtx.ClassStore.GetDiscussion(did)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "帖子不存在"})
			return
		}
		if _, ok := requireClassMember(w, r, svcCtx, d.ClassId, cu); !ok {
			return
		}
		var req struct {
			Content string `json:"content"`
		}
		body(r, &req)
		_, err = svcCtx.ClassStore.InsertReply(&model.ReplyRow{
			DiscussionId: did, UserId: cu.id, Content: req.Content,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}
