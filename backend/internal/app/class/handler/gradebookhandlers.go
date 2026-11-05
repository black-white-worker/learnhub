package handler

// 成绩册导出 + 逐份批阅队列（对标 Canvas 的 Gradebook export / SpeedGrader）。
//
// 这两个接口解决的都是"老师端效率"：
//   - 导出 CSV：成绩册页面上点一下就能带走，不用截图/手抄；
//   - 批阅队列：一次取回整份队列（未批优先），前端本地上下翻，保存才发请求。

import (
	"fmt"
	"net/http"
	"net/url"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/class/logic"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ExportGradesHandler GET /api/class/:id/grades/export
// 返回 text/csv（带 UTF-8 BOM，Excel 直接打开不乱码）。
func ExportGradesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		classId := class.ParseIntPath(r, "id")
		data, filename, err := logic.NewListGradesLogic(r.Context(), svcCtx).ExportCSV(classId, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		// filename* 用 RFC 5987 编码，中文文件名在 Chrome/Edge 下才不会乱码
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename*=UTF-8''%s", url.QueryEscape(filename)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

// GradeQueueHandler GET /api/assignment/:aid/grade-queue
// 一次返回批阅所需的一切：作业信息、题目、全部提交（未批优先）。
func GradeQueueHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		aid := class.ParseIntPath(r, "aid")
		a, err := svcCtx.ClassStore.GetAssignment(aid)
		if err != nil {
			class.WriteBizError(w, &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"})
			return
		}
		// 与作业批改一致的授权：班内老师 / 管理员
		if be := class.RequireTeacher(svcCtx, a.ClassId, cu); be != nil {
			class.WriteBizError(w, be)
			return
		}
		questions, _ := svcCtx.ClassStore.ListQuestions(aid)
		queue, err := svcCtx.ClassStore.ListSubmissionQueue(aid)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		pending := 0
		var nextId int64
		for _, s := range queue {
			if !s.Score.Valid {
				pending++
				if nextId == 0 {
					nextId = s.Id
				}
			}
		}
		httpx.WriteJson(w, http.StatusOK, map[string]interface{}{
			"assignment": a,
			"questions":  questions,
			"items":      queue,
			"total":      len(queue),
			"pending":    pending,
			"nextId":     nextId,
			"maxScore":   maxScoreOf(a),
		})
	}
}

// maxScoreOf 作业满分（没配时退回 100），前端分数输入框的上限。
func maxScoreOf(a *model.AssignmentRow) int {
	if a.TotalScore > 0 {
		return a.TotalScore
	}
	return 100
}
