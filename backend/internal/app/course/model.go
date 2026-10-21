package course

import (
	"errors"
	"sort"
	"sync"
)

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

	// ===== 以下字段为 MinIO 上传功能新增（原字段保持不变）=====
	CoverUrl      string   `json:"coverUrl"`      // 封面原图 URL（存 MinIO）
	CoverThumbUrl string   `json:"coverThumbUrl"` // 封面缩略图 URL（320x180，存 MinIO）
	Videos        []*Video `json:"videos"`        // 本课程已上传的视频列表

	nextVideoId int // 本课程下一个视频 id（内存存储用，不参与 JSON 序列化）
}

// Video 表示课程下的一个视频文件。
// 视频本体存在 MinIO，这里只记录元信息（视频列表展示就靠它）。
type Video struct {
	Id         int    `json:"id"`
	Name       string `json:"name"`
	Url        string `json:"url"`
	Size       int64  `json:"size"`
	UploadedAt string `json:"uploadedAt"`
}

// courseStore 内存课程存储（教学版）：
// 与 user 模块的 UserStore 同款思路，先放内存，后续接 MySQL。
// 这里用包级单例，避免引入 model 包的循环依赖（app/course 依赖 svc，svc 依赖 model）；
// 本次合并：A 的 AddCourse/GetCourse/SetCover/AddVideos + B 的 List 全部保留。
type courseStore struct {
	mu      sync.RWMutex   // 读写锁：上传封面/视频会并发写 map，加锁保证线程安全
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

// AddCourse 新增一门课程（课程创建功能用，来自「课程创建」分支）。
// 课程名称不能为空；名称重复时返回错误，避免重复录入。
func (s *courseStore) AddCourse(name, intro, teacher string) (*Course, error) {
	// 1. 基础校验：课程名称不能为空
	if name == "" {
		return nil, errors.New("课程名称不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

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

// GetCourse 根据 id 获取课程（封面上传 / 视频上传前先确认课程存在）。
// 课程不存在时返回错误。
func (s *courseStore) GetCourse(id int) (*Course, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.courses[id]
	if !ok {
		return nil, errors.New("课程不存在")
	}
	return c, nil
}

// SetCover 设置课程封面（原图 URL + 缩略图 URL）。
func (s *courseStore) SetCover(id int, coverUrl, coverThumbUrl string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.courses[id]
	if !ok {
		return errors.New("课程不存在")
	}
	c.CoverUrl = coverUrl
	c.CoverThumbUrl = coverThumbUrl
	return nil
}

// AddVideos 给课程追加视频列表（视频上传功能用）。
// 每门课程内部的视频 id 从 1 开始自增。
func (s *courseStore) AddVideos(id int, videos []*Video) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.courses[id]
	if !ok {
		return errors.New("课程不存在")
	}
	for _, v := range videos {
		c.nextVideoId++
		v.Id = c.nextVideoId
		c.Videos = append(c.Videos, v)
	}
	return nil
}

// List 返回全部课程（按 id 升序），课程列表接口用（来自「课程列表」分支）。
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
