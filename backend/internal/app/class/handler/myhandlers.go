package handler

// 「我的考试 / 我的作业」聚合接口。
//
// 动机：学生首页原来要「取我的班级 → 逐个班级取考试」，班级越多越慢，
// 而且每场考试的"我交了没"还要单独查。现在一次 JOIN 全拿到，
// 前端单次请求即可渲染，班级数量不再影响首屏耗时。

import (
	"net/http"
	"strconv"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// MyExamsHandler GET /api/my/exams?limit=100
func MyExamsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		limit := parseLimit(r, 100)
		rows, err := svcCtx.ExamStore.ListMyExams(cu.Id, limit)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		// 用服务器时间补齐"时间阶段"：不能拿"有没有交卷"当"开没开始"。
		// 之前前端只有 myStatus（空串=没交卷），于是已结束的考试全显示"未开始"。
		now := time.Now()
		pending := 0
		for _, row := range rows {
			row.Phase = class.ExamPhase(now, row.StartAt, row.EndAt)
			// 只有"学生 + 正在进行 + 还没交卷"才需要提示去参加；
			// 老师/管理员本来就不参加考试，不该出现在"待参加"里。
			row.CanAttend = row.MyRole == "student" && row.Phase == class.PhaseOngoing && row.MyStatus == ""
			if row.CanAttend {
				pending++
			}
		}
		httpx.WriteJson(w, http.StatusOK, map[string]interface{}{
			"list":    rows,
			"total":   len(rows),
			"pending": pending,
		})
	}
}

// MyAssignmentsHandler GET /api/my/assignments?limit=100
func MyAssignmentsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		limit := parseLimit(r, 100)
		rows, err := svcCtx.ClassStore.ListMyAssignments(cu.Id, limit)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]interface{}{
			"list":  rows,
			"total": len(rows),
		})
	}
}

// parseLimit 解析 limit 参数，非法值退回默认值。
func parseLimit(r *http.Request, def int) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			return n
		}
	}
	return def
}
