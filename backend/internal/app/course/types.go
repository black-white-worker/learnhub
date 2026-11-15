package course

import (
	"io"

	"learningplatform/internal/platform/oss"
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
	// CanWatch 当前访问者能否观看本课程媒体（视频/讲义）。
	// 未登录、或课程绑定班级但访问者不在班里时为 false，此时 Video.Url 为空。
	CanWatch bool `json:"canWatch"`
	// MediaReason 不能观看时的可读原因（可直接展示）。
	MediaReason string `json:"mediaReason,omitempty"`
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

// SignPartsRequest 批量给若干片签直传 URL。
// 一次一批而不是一次一片：1GB 视频有 200 片，一片一个请求要多烧 200 次往返。
// 签名本身是本地 HMAC（不碰 MinIO），批量签几乎零代价。
type SignPartsRequest struct {
	UploadID    string `json:"uploadId"`
	PartNumbers []int  `json:"partNumbers"` // 一次最多 maxSignBatch 片，片号从 1 开始
}

// SignedPart 一片的直传地址。
type SignedPart struct {
	PartNumber int    `json:"partNumber"`
	URL        string `json:"url"`
}

// SignPartsResponse 批量签名结果 + URL 有效期，前端据此提前重签。
type SignPartsResponse struct {
	Parts    []SignedPart `json:"parts"`
	ExpireIn int          `json:"expireIn"` // 秒
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

// ===== 断点续传：查询会话已上传的分片 =====

// MultipartStatusRequest 查询分片会话进度。
type MultipartStatusRequest struct {
	UploadID string `json:"uploadId"`
}

// UploadedPart 一个已经躺在 MinIO 里的分片。
type UploadedPart struct {
	PartNumber int   `json:"partNumber"`
	Size       int64 `json:"size"` // 字节，前端拿它和预期分片大小比对，不一致就重传该片
}

// MultipartStatusResponse 会话状态 + 已完成分片清单。
// 前端刷新页面/断网重连后先调这里，再决定从第几片继续传。
type MultipartStatusResponse struct {
	UploadID  string         `json:"uploadId"`
	Status    string         `json:"status"` // init/completed/aborted/expired
	CourseId  int            `json:"courseId"`
	FileName  string         `json:"fileName"`
	FileSize  int64          `json:"fileSize"`
	ChunkSize int64          `json:"chunkSize"` // 会话建立时的分片大小
	Uploaded  int64          `json:"uploaded"`  // 已完成分片的字节合计
	Parts     []UploadedPart `json:"parts"`     // 按 partNumber 升序
	ExpireAt  string         `json:"expireAt"`  // 会话作废时间（RFC3339）
	Resumable bool           `json:"resumable"` // 还能不能接着传（false = 前端该重新 init）
}

// ===== 视频后处理（faststart）=====

// ReprocessVideoRequest 手工重试失败的视频后处理。
type ReprocessVideoRequest struct {
	CourseId int `path:"courseId"`
	VideoId  int `path:"videoId"`
}

// ReprocessVideoResponse 重试受理结果：状态回到 pending，前端立刻改徽标。
type ReprocessVideoResponse struct {
	CourseId      int    `json:"courseId"`
	VideoId       int    `json:"videoId"`
	ProcessStatus string `json:"processStatus"`
}

// ===== 知识库资料清单（内部接口，GET /api/internal/course/:courseId/kb-manifest）=====
//
// 为什么要有这个接口：AI 答疑的正文来自「课程资源」，而 resources 的唯一对外口径
// 就是课程详情页那份列表（/api/course/resources/:courseId）。ai-service 如果自己
// 直连 MySQL 再查一次，等于把口径复制了一份 —— 私有桶预签名、类型映射、资源删除
// 只要有一处改动，两边就会对不上（线上就出现过「页面有讲义、AI 却说没有资料」）。
// 所以灌库一律向 Go 要这份清单，字段名对齐 ai-service 的 dict（下划线风格）。

// KbManifestRequest 路径参数：课程 id。
type KbManifestRequest struct {
	CourseId int `path:"courseId"`
}

// KbManifestItem 一份可灌库的资料元信息（讲义或视频）。
type KbManifestItem struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
	// Url 与页面看到的一致：私有桶下是预签名地址，ai-service 直接 GET 就能取到文件
	Url        string `json:"url"`
	ObjectName string `json:"object_name"` // MinIO 对象名，私有桶可用 SDK 直取
	Type       string `json:"type"`        // pdf/doc/docx/ppt/pptx；视频固定 video
	Size       int64  `json:"size"`
	UploadedAt string `json:"uploaded_at,omitempty"`
}

// KbManifestResponse 一门课的全部资料清单。
type KbManifestResponse struct {
	CourseId   int              `json:"course_id"`
	CourseName string           `json:"course_name"`
	Resources  []KbManifestItem `json:"resources"` // 讲义（灌库正文来源）
	Videos     []KbManifestItem `json:"videos"`    // 课程视频（正文需 ASR，先给标题清单）
}
