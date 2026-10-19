package course

import "sort"

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

// courseStore 内存课程存储（教学版）。
// 本分支（课程列表）只实现列表需要的方法；
// 「课程创建」分支的同名文件里会有新增课程的 AddCourse 等方法。
// 合并冲突时：Course 结构体保留一份，两个分支的方法合进同一个 courseStore。
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

// List 返回全部课程（按 id 升序），课程列表接口用。
// map 的遍历顺序不固定，这里先收集 id 排序再取值，保证返回顺序稳定。
func (s *courseStore) List() []*Course {
	ids := make([]int, 0, len(s.courses))
	for id := range s.courses {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	result := make([]*Course, 0, len(ids))
	for _, id := range ids {
		result = append(result, s.courses[id])
	}
	return result
}
