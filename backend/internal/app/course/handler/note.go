package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"

	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"
)

// VideoNoteRow 视频笔记
type VideoNoteRow struct {
	Id       int64  `json:"id"`
	UserId   int64  `json:"userId"`
	CourseId int64  `json:"courseId"`
	VideoId  int64  `json:"videoId"`
	TimeSec  int    `json:"timestampSeconds"`
	Content  string `json:"content"`
}

// addNoteHandler 学生在视频时间点记笔记
func addNoteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := user.CurrentUser(r)
		if u == nil {
			httpx.WriteJson(w, http.StatusUnauthorized, map[string]string{"message": "未登录"})
			return
		}
		var req struct {
			CourseId int64  `json:"courseId"`
			VideoId  int64  `json:"videoId"`
			TimeSec  int    `json:"timestampSeconds"`
			Content  string `json:"content"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Content == "" {
			httpx.WriteJson(w, 400, map[string]string{"message": "笔记内容不能为空"})
			return
		}
		// 从 session 拿 user_id：class 模块用 UserStore.GetIdByUsername，这里也走它
		uid, err := svcCtx.UserStore.GetIdByUsername(u.Username)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": "用户不存在"})
			return
		}
		_, err = svcCtx.DB.ExecCtx(r.Context(),
			"INSERT INTO video_notes (user_id, course_id, video_id, timestamp_seconds, content) VALUES (?,?,?,?,?)",
			uid, req.CourseId, req.VideoId, req.TimeSec, req.Content)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}

// listNotesHandler 查当前用户在某视频下的所有笔记
func listNotesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := user.CurrentUser(r)
		if u == nil {
			httpx.WriteJson(w, http.StatusUnauthorized, map[string]string{"message": "未登录"})
			return
		}
		uid, err := svcCtx.UserStore.GetIdByUsername(u.Username)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": "用户不存在"})
			return
		}
		vars := pathvar.Vars(r)
		courseId, _ := strconv.ParseInt(vars["courseId"], 10, 64)
		videoId, _ := strconv.ParseInt(vars["videoId"], 10, 64)
		if courseId == 0 || videoId == 0 {
			logx.Errorf("note: bad path vars courseId=%q videoId=%q", vars["courseId"], vars["videoId"])
		}
		var rows []*VideoNoteRow
		err = svcCtx.DB.QueryRowsCtx(r.Context(), &rows,
			`SELECT id, user_id, course_id, video_id, timestamp_seconds AS timestampSeconds, content
			 FROM video_notes
			 WHERE user_id=? AND course_id=? AND video_id=?
			 ORDER BY timestamp_seconds`, uid, courseId, videoId)
		if err != nil && err != sql.ErrNoRows {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, rows)
	}
}
