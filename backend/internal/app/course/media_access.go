package course

// 课程媒体（视频 / 讲义 / 预览）的访问策略。
//
// 背景：改造前 /api/course/detail、/video/play、/resources 都是公开路由，
// 而私有桶的预签名 URL 就是在这些公开请求里签发的 —— 等于"桶是私有的，
// 但任何人只要知道课程 id 就能拿到 30 分钟有效的直链"，私有桶形同虚设。
//
// 这里把"能不能看这门课的媒体"收敛成一个纯函数 + 一个取数函数：
//
//	DecideMediaAccess：纯规则，可穷举单测；
//	ResolveMediaAccess：补上"课程绑定班级 / 用户是否在班"的取数。
//
// 规则（对标 Canvas 的 course visibility + enrollment）：
//  1. 未登录 → 拒绝（401 由 handler 输出）
//  2. 管理员 / 课程创建者 → 放行
//  3. 课程被至少一个班级绑定 → 必须是其中一个班级的成员
//  4. 没有被任何班级绑定的独立课程 → 登录即可观看（公开课语义）
//
// 封面是例外：它出现在公开的课程列表页上，属于宣传素材，
// 由 list/detail 直接签名（30 分钟），不参与上面的判定。

import (
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

// MediaAccess 一次媒体访问判定的结果。
type MediaAccess struct {
	// Allowed 是否允许访问。
	Allowed bool `json:"allowed"`
	// Reason 拒绝原因（可直接展示给用户），允许时为空。
	Reason string `json:"reason,omitempty"`
	// Open 课程没有绑定任何班级（独立公开课）。
	Open bool `json:"open"`
}

// DecideMediaAccess 纯规则判定。
//
//	actor        当前登录用户（nil 表示未登录）
//	owner        课程创建者用户名
//	boundClasses 课程被多少个班级绑定
//	myClasses    当前用户加入了其中多少个班级
func DecideMediaAccess(actor *model.SessionUser, owner string, boundClasses, myClasses int) MediaAccess {
	if actor == nil {
		return MediaAccess{Allowed: false, Reason: "请先登录后再观看课程"}
	}
	if actor.Role == "admin" {
		return MediaAccess{Allowed: true}
	}
	if owner != "" && actor.Username == owner {
		return MediaAccess{Allowed: true}
	}
	if boundClasses <= 0 {
		// 独立课程：没有班级可以校验，登录即可学习。
		return MediaAccess{Allowed: true, Open: true}
	}
	if myClasses > 0 {
		return MediaAccess{Allowed: true}
	}
	return MediaAccess{Allowed: false, Reason: "该课程属于班级教学，请先加入对应班级"}
}

// ResolveMediaAccess 取出课程与班级关系后做判定。
//
// 取数失败按拒绝处理（fail-closed）：媒体是付费/教学内容，
// 宁可让用户重试一次，也不能因为数据库抖动把内容放出去。
func ResolveMediaAccess(svcCtx *svc.ServiceContext, actor *model.SessionUser, c *Course) MediaAccess {
	if actor == nil {
		return DecideMediaAccess(nil, "", 0, 0)
	}
	if c == nil {
		return MediaAccess{Allowed: false, Reason: "课程不存在"}
	}
	// 管理员 / 创建者不需要查库
	if actor.Role == "admin" || (c.CreatedBy != "" && actor.Username == c.CreatedBy) {
		return MediaAccess{Allowed: true}
	}
	if svcCtx == nil || svcCtx.ClassStore == nil || svcCtx.UserStore == nil {
		// 单测 / 无班级数据层：按独立课程处理（登录即可）
		return DecideMediaAccess(actor, c.CreatedBy, 0, 0)
	}
	userId, err := svcCtx.UserStore.GetIdByUsername(actor.Username)
	if err != nil {
		return MediaAccess{Allowed: false, Reason: "无法确认当前用户身份"}
	}
	bound, mine, err := svcCtx.ClassStore.CourseEnrollmentStats(int64(c.Id), userId)
	if err != nil {
		return MediaAccess{Allowed: false, Reason: "暂时无法校验选课关系，请稍后重试"}
	}
	return DecideMediaAccess(actor, c.CreatedBy, bound, mine)
}
