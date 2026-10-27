package course

import (
	"io"

	"learningplatform/internal/oss"
)

// ===== 课程创建接口 POST /api/course/create =====

// CreateRequest 创建课程请求体。
// 课程名称必填，简介和教师可空；createdBy 由前端从登录态带入，记录创建者。
type CreateRequest struct {
	Name      string `json:"name"`
	Intro     string `json:"intro"`
	Teacher   string `json:"teacher"`
	CreatedBy string `json:"createdBy"`
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

// ListResponse 课程列表响应体：分页返回课程。
type ListResponse struct {
	Courses  []*Course `json:"courses"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
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

// SaveProgressRequest 保存播放进度请求：路径定位视频，body 带当前播放位置。
//
// 字段说明（改造后按用户落库，支持学情统计）：
//   - Position       播放头位置（秒），用于下次续播
//   - WatchedDelta   上次上报以来"真实播放"的秒数增量（拖动进度条不计入）
//   - Duration       视频总时长（秒），由播放器上报
//   - UserId         登录用户 id，由 handler 从会话注入，前端不可传
type SaveProgressRequest struct {
	CourseId     int   `path:"courseId"`
	VideoId      int   `path:"videoId"`
	Position     int   `json:"position,optional"`
	WatchedDelta int   `json:"watchedDelta,optional"`
	Duration     int   `json:"duration,optional"`
	UserId       int64 `json:"-"`
}

// SaveProgressResponse 保存进度响应：回显服务端累计的观看量与完成标记。
type SaveProgressResponse struct {
	Position       int  `json:"position"`
	WatchedSeconds int  `json:"watchedSeconds"`
	Completed      bool `json:"completed"`
}

// GetProgressRequest 路径参数：课程 id + 视频 id。
type GetProgressRequest struct {
	CourseId int   `path:"courseId"`
	VideoId  int   `path:"videoId"`
	UserId   int64 `json:"-"`
}

// GetProgressResponse 返回上次记住的播放位置（秒），没看过则为 0。
type GetProgressResponse struct {
	Position        int  `json:"position"`
	WatchedSeconds  int  `json:"watchedSeconds"`
	DurationSeconds int  `json:"durationSeconds"`
	Completed       bool `json:"completed"`
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
	CreatedBy     string   `json:"createdBy"`
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

// ===== 课程资源（PPT / Word / PDF 讲义）上传 / 列表 / 删除 =====

// Resource 课程下的一个文档资源文件。
// 文件本体存 MinIO，这里只记元信息，供资源列表展示和点击下载。
type Resource struct {
	Id         int    `json:"id"`
	Name       string `json:"name"`       // 原始文件名（带扩展名）
	Url        string `json:"url"`        // MinIO 直链（浏览器可直接打开 / 下载）
	ObjectName string `json:"-"`          // MinIO 桶内路径（后端转预览用，不暴露给前端）
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

// ===== 分片直传上传（视频大文件，前端直传 MinIO，后端只签 URL）=====

// InitMultipartRequest 开启分片上传。
type InitMultipartRequest struct {
	CourseId int    `json:"courseId"`
	FileName string `json:"fileName"`
	FileSize int64  `json:"fileSize"` // 字节，用于校验上限
}

// InitMultipartResponse 返回 uploadId + 最终 objectName + 推荐分片大小。
type InitMultipartResponse struct {
	UploadID    string `json:"uploadId"`
	ObjectName  string `json:"objectName"`
	ChunkSize   int64  `json:"chunkSize"`   // 推荐分片大小（5MB）
	ExpireAt    string `json:"expireAt"`    // 会话过期时间（RFC3339）
	MinPartSize int64  `json:"minPartSize"` // 除最后一片外，每片最小 5MB（MinIO 限制）
}

// SignPartRequest 给某一片签直传 URL。
type SignPartRequest struct {
	UploadID   string `json:"uploadId"`
	PartNumber int    `json:"partNumber"` // 从 1 开始
}

// SignPartResponse 返回预签名 PUT URL，前端直接 PUT 分片字节到这个 URL。
type SignPartResponse struct {
	URL        string `json:"url"`
	PartNumber int    `json:"partNumber"`
}

// CompleteMultipartRequest 合并分片。
type CompleteMultipartRequest struct {
	UploadID string              `json:"uploadId"`
	Parts    []oss.MultipartPart `json:"parts"`
}

// CompleteMultipartResponse 合并成功，返回挂到课程下的视频元信息。
type CompleteMultipartResponse struct {
	CourseId int    `json:"courseId"`
	Video    *Video `json:"video"`
}

// AbortMultipartRequest 取消分片上传。
type AbortMultipartRequest struct {
	UploadID string `json:"uploadId"`
}
