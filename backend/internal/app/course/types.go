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

// ===== 课程资源（PPT / Word / PDF 讲义）上传 / 列表 / 删除 =====

// Resource 课程下的一个文档资源文件。
// 文件本体存 MinIO，这里只记元信息，供资源列表展示和点击下载。
type Resource struct {
	Id         int    `json:"id"`
	Name       string `json:"name"`       // 原始文件名（带扩展名）
	Url        string `json:"url"`        // MinIO 直链（浏览器可直接打开 / 下载）
	Size       int64  `json:"size"`       // 文件大小（字节）
	Type       string `json:"type"`       // 资源类型：pdf / ppt / pptx / doc / docx
	UploadedAt string `json:"uploadedAt"` // 上传时间（RFC3339）
}

// UploadResourceRequest 资源上传请求。
// handler 从 multipart 表单解析后组装（表单字段：courseId + file）。
type UploadResourceRequest struct {
	CourseId  int
	FileName  string
	FileBytes []byte
}

// UploadResourceResponse 资源上传响应：返回新上传的资源元信息。
type UploadResourceResponse struct {
	CourseId int       `json:"courseId"`
	Resource *Resource `json:"resource"`
}

// ListResourcesRequest 路径参数：课程 id。
type ListResourcesRequest struct {
	CourseId int `path:"courseId"`
}

// ListResourcesResponse 资源列表响应：返回某课程全部资源。
type ListResourcesResponse struct {
	CourseId  int         `json:"courseId"`
	Resources []*Resource `json:"resources"`
}

// DeleteResourceRequest 路径参数：课程 id + 资源 id。
type DeleteResourceRequest struct {
	CourseId   int `path:"courseId"`
	ResourceId int `path:"resourceId"`
}

// DeleteResourceResponse 删除资源响应。
type DeleteResourceResponse struct {
	CourseId   int `json:"courseId"`
	ResourceId int `json:"resourceId"`
}
