package course

import "errors"

// Course 表示一门课程。
//
// 【冲突点 1】本文件由「课程列表」和「课程创建」两个分支各自从零编写，
// 两边都会定义 Course 结构体（以及自己的课程存储），
// 合并到「课程管理功能」主干时整个文件冲突，需要手动合并。
type Course struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	Intro   string `json:"intro"`
	Teacher string `json:"teacher"`
}

// courseStore 内存课程存储（教学版）：
// 与 user 模块的 UserStore 同款思路，先放内存，后续接 MySQL。
// 这里用包级单例，避免引入 model 包的循环依赖（app/course 依赖 svc，svc 依赖 model）；
// 合并解决冲突后，可以再统一迁移到 svc.ServiceContext 注入。
type courseStore struct {
	courses map[int]*Course
	nextId  int
}

// store 课程存储包级单例（进程内共享，重启丢失）。
var store = newCourseStore()

// newCourseStore 创建课程存储并预置两条示例课程。
func newCourseStore() *courseStore {
	s := &courseStore{
		courses: make(map[int]*Course),
		nextId:  1,
	}
	s.courses[s.nextId] = &Course{Id: s.nextId, Name: "Go 语言基础", Intro: "从零上手 Go 语言，涵盖语法、并发与 Web 开发", Teacher: "陈老师"}
	s.nextId++
	s.courses[s.nextId] = &Course{Id: s.nextId, Name: "数据结构与算法", Intro: "链表、栈、队列、树与常见算法思想", Teacher: "李老师"}
	s.nextId++
	return s
}

// AddCourse 新增一门课程（课程创建功能用）。
// 课程名称不能为空；名称重复时返回错误，避免重复录入。
func (s *courseStore) AddCourse(name, intro, teacher string) (*Course, error) {
	// 1. 基础校验：课程名称不能为空
	if name == "" {
		return nil, errors.New("课程名称不能为空")
	}

	// 2. 重名校验：遍历已有课程，发现同名直接报错
	for _, c := range s.courses {
		if c.Name == name {
			return nil, errors.New("课程名称已存在")
		}
	}

	// 3. 写入存储并自增 id
	c := &Course{Id: s.nextId, Name: name, Intro: intro, Teacher: teacher}
	s.courses[c.Id] = c
	s.nextId++
	return c, nil
}
