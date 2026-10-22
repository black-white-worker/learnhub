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

// ===== 课程编辑接口 PUT /api/course/edit =====

// EditCourseRequest 编辑课程基本信息请求体。
// Id 定位要编辑的课程；Name 必填（重名校验排除自身）；Intro / Teacher 可空。
type EditCourseRequest struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	Intro   string `json:"intro"`
	Teacher string `json:"teacher"`
}

// EditCourseResponse 编辑课程响应体：返回更新后的课程基本信息。
type EditCourseResponse struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	Intro   string `json:"intro"`
	Teacher string `json:"teacher"`
}

// ===== 课程详情接口 GET /api/course/detail/:id =====

// CourseDetailRequest 路径参数：要查看的课程 id。
type CourseDetailRequest struct {
	Id int `path:"id"`
}

// CourseDetailResponse 课程详情响应体：
// 比列表多返回封面原图 / 缩略图 / 完整视频列表，供课程详情页使用。
type CourseDetailResponse struct {
	Id            int      `json:"id"`
	Name          string   `json:"name"`
	Intro         string   `json:"intro"`
	Teacher       string   `json:"teacher"`
	CoverUrl      string   `json:"coverUrl"`
	CoverThumbUrl string   `json:"coverThumbUrl"`
	Videos        []*Video `json:"videos"`
}

// ===== 课程删除接口 DELETE /api/course/delete/:id =====

// DeleteCourseRequest 路径参数：要删除的课程 id。
type DeleteCourseRequest struct {
	Id int `path:"id"`
}

// DeleteCourseResponse 删除课程响应体。
type DeleteCourseResponse struct {
	Id int `json:"id"`
}
