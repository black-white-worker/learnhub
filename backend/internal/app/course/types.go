package course

import "io"

// ===== 课程创建接口 POST /api/course/create =====

// CreateRequest 创建课程请求体。
// 课程名称必填，简介和教师可空。
type CreateRequest struct {
	Name    string `json:"name"`
	Intro   string `json:"intro"`
	Teacher string `json:"teacher"`
}

// CreateResponse 创建课程响应体。
// 成功时返回新课程的 id 和名称。
type CreateResponse struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

// ===== 封面上传接口 POST /api/course/upload/cover =====

// UploadCoverRequest 封面上传请求。
// 注意：这不是 JSON 请求体，是 handler 从 multipart 表单解析后组装的
// （表单字段：courseId + file）。
type UploadCoverRequest struct {
	CourseId  int    // 课程 id（表单字段 courseId）
	FileName  string // 原文件名（表单文件 file 的文件名）
	FileBytes []byte // 封面文件字节（handler 已按上限读入内存，供校验和生成缩略图）
}

// UploadCoverResponse 封面上传响应：原图 + 缩略图两个 MinIO URL。
type UploadCoverResponse struct {
	CourseId      int    `json:"courseId"`
	CoverUrl      string `json:"coverUrl"`
	CoverThumbUrl string `json:"coverThumbUrl"`
}

// ===== 视频上传接口 POST /api/course/upload/videos =====

// UploadVideosRequest 视频上传请求（支持一次上传多个文件）。
// handler 从 multipart 表单解析后组装（表单字段：courseId + files）。
type UploadVideosRequest struct {
	CourseId int
	Files    []*UploadFile
}

// UploadFile 一个待上传的视频文件。
// 用 Reader 流式传递，避免把大视频一次性读进内存。
type UploadFile struct {
	FileName string
	Size     int64
	Reader   io.Reader
}

// UploadVideosResponse 视频上传响应：返回该课程最新的视频列表。
type UploadVideosResponse struct {
	CourseId int      `json:"courseId"`
	Videos   []*Video `json:"videos"`
}

// ===== 课程列表接口 GET /api/course/list =====

// ListResponse 课程列表响应体：返回全部课程（来自「课程列表」分支）。
type ListResponse struct {
	Courses []*Course `json:"courses"`
}

// ===== 课程视频观看 / 进度（「课程观看」分支）=====

// PlayVideoRequest 路径参数：课程 id + 视频 id。
type PlayVideoRequest struct {
	CourseId int `path:"courseId"`
	VideoId  int `path:"videoId"`
}

// PlayVideoResponse 播放信息：返回视频元信息（Url 已是 faststart 直链）。
type PlayVideoResponse struct {
	Video *Video `json:"video"`
}

// SaveProgressRequest 保存播放进度请求：路径定位视频，body 带当前播放位置（秒）。
type SaveProgressRequest struct {
	CourseId int `path:"courseId"`
	VideoId  int `path:"videoId"`
	Position int `json:"position"`
}

// SaveProgressResponse 保存进度响应。
type SaveProgressResponse struct {
	Position int `json:"position"`
}

// GetProgressRequest 路径参数：课程 id + 视频 id。
type GetProgressRequest struct {
	CourseId int `path:"courseId"`
	VideoId  int `path:"videoId"`
}

// GetProgressResponse 返回上次记住的播放位置（秒），没看过则为 0。
type GetProgressResponse struct {
	Position int `json:"position"`
}
