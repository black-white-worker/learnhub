package class

import (
	"net/http"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ---- 成绩与学情：老师端成绩聚合表 ----

// listGrades 老师端查看全班成绩聚合表（作业平均分、签到次数、加权总分、排名）
func listGrades(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		rows, err := svcCtx.ClassStore.ListGrades(classId)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		vW, aW, sW, _, _ := svcCtx.ClassStore.GetGradeRule(classId)
		// 用权重算总分（视频权重暂时给满分，等视频进度聚合接入再替换）
		for i, g := range rows {
			g.TotalScore = g.AvgScore*float64(aW)/100 + float64(g.SigninCnt*10)*float64(sW)/100 + 100*float64(vW)/100
			_ = i
		}
		httpx.WriteJson(w, 200, map[string]interface{}{
			"rule": map[string]int{"video": vW, "assignment": aW, "signin": sW},
			"rows": rows,
		})
	}
}
