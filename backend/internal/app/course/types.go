package course

// ===== 课程列表接口 GET /api/course/list =====
//
// 【冲突点 3】本文件由「课程列表」和「课程创建」两个分支各自编写：
// 「课程创建」分支定义了 CreateRequest / CreateResponse，
// 本分支定义了 ListResponse。合并时两边的类型都要保留。

// ListResponse 课程列表响应体：返回全部课程。
type ListResponse struct {
	Courses []*Course `json:"courses"`
}
