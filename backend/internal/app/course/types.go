package course

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
