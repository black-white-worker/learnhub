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
	Id        int    `json:"id"`
	Name      string `json:"name"`
	Intro     string `json:"intro"`
	Teacher   string `json:"teacher"`
	CreatedBy string `json:"createdBy"` // 创建者用户名（非创建者只读）

	// ===== 以下字段为 MinIO 上传功能新增（原字段保持不变）=====
	CoverUrl      string   `json:"coverUrl"`      // 封面原图 URL（存 MinIO）
	CoverThumbUrl string   `json:"coverThumbUrl"` // 封面缩略图 URL（320x180，存 MinIO）
	Videos        []*Video `json:"videos"`        // 本课程已上传的视频列表

	// ===== 以下为「课程资源」分支新增 =====
	Resources []*Resource `json:"resources"` // 本课程已上传的文档资源（PPT/Word/PDF）

	nextVideoId    int // 本课程下一个视频 id（内存存储用，不参与 JSON 序列化）
	nextResourceId int // 本课程下一个资源 id（内存存储用，不参与 JSON 序列化）
}

// Video 表示课程下的一个视频文件。
// 视频本体存在 MinIO，这里只记录元信息（视频列表展示就靠它）。
type Video struct {
	Id         int    `json:"id"`
	Name       string `json:"name"`
	Url        string `json:"url"`
	Size       int64  `json:"size"`
	UploadedAt string `json:"uploadedAt"`

	// 以下为「课程观看」分支新增
	Faststarted bool `json:"faststarted"` // 是否已生成 moov 前置（+faststart）版本

	CourseId   int    `json:"-"` // 所属课程 id（内部用，不序列化）
	ObjectName string `json:"-"` // 源视频在 MinIO 内的对象路径（内部用，不序列化）
}

// CourseStore 课程存储接口：
// 内存实现（memoryStore，测试用）和 MySQL 实现（mysqlStore，生产用）都满足它。
// logic 层只依赖这个接口，不关心数据到底在内存还是数据库。
type CourseStore interface {
	AddCourse(name, intro, teacher, createdBy string) (*Course, error)
	GetCourse(id int) (*Course, error)
	SetCover(id int, coverUrl, coverThumbUrl string) error
	AddVideos(id int, videos []*Video) error
	UpdateCourse(id int, name, intro, teacher string) (*Course, error)
	DeleteCourse(id int) error
	List() []*Course
	FindVideo(courseId, videoId int) (*Video, error)
	AddResource(id int, res *Resource) error
	ListResources(id int) ([]*Resource, error)
	DeleteResource(id, resourceId int) error
	MarkVideoFaststart(courseId, videoId int, newUrl string) error
}

// memoryStore 内存课程存储（教学版 / 测试用）：
// 进程重启即丢。生产环境由 svc 注入 MySQL 实现（见 mysql_store.go）。
type memoryStore struct {
	mu      sync.RWMutex // 读写锁：上传封面/视频会并发写 map，加锁保证线程安全
	courses map[int]*Course
	nextId  int
}

// store 课程存储包级单例。默认是内存实现；
// 生产启动时由 svc 调 InitCourseStore 换成 MySQL 实现。
var Store CourseStore = newMemoryStore()

// newMemoryStore 创建内存课程存储并预置两条示例课程。
func newMemoryStore() *memoryStore {
	s := &memoryStore{
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
func (s *memoryStore) AddCourse(name, intro, teacher, createdBy string) (*Course, error) {
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
	c := &Course{Id: s.nextId, Name: name, Intro: intro, Teacher: teacher, CreatedBy: createdBy}
	s.courses[c.Id] = c
	s.nextId++
	return c, nil
}

// GetCourse 根据 id 获取课程（封面上传 / 视频上传前先确认课程存在）。
// 课程不存在时返回错误。
func (s *memoryStore) GetCourse(id int) (*Course, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.courses[id]
	if !ok {
		return nil, errors.New("课程不存在")
	}
	return c, nil
}

// SetCover 设置课程封面（原图 URL + 缩略图 URL）。
func (s *memoryStore) SetCover(id int, coverUrl, coverThumbUrl string) error {
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
func (s *memoryStore) AddVideos(id int, videos []*Video) error {
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

// UpdateCourse 更新课程基本信息（课程编辑功能用，来自「课程编辑」分支）。
// 课程不存在报错；课程名称必填；重名校验时排除课程自身，
// 这样"改名后点保存、名字没变"也不会误判成重名。
func (s *memoryStore) UpdateCourse(id int, name, intro, teacher string) (*Course, error) {
	if name == "" {
		return nil, errors.New("课程名称不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.courses[id]
	if !ok {
		return nil, errors.New("课程不存在")
	}

	// 重名校验：发现别的课程已经叫这个名字就拒绝
	for _, other := range s.courses {
		if other.Id != id && other.Name == name {
			return nil, errors.New("课程名称已存在")
		}
	}

	c.Name = name
	c.Intro = intro
	c.Teacher = teacher
	return c, nil
}

// DeleteCourse 删除一门课程（课程删除功能用，来自「课程编辑」分支）。
// 只删内存里的课程元信息；MinIO 里的封面 / 视频对象由孤儿清理器兜底回收，
// 避免这里再写一套对象删除逻辑、和上传回滚逻辑重复。
func (s *memoryStore) DeleteCourse(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.courses[id]; !ok {
		return errors.New("课程不存在")
	}
	delete(s.courses, id)
	return nil
}

// List 返回全部课程（按 id 升序），课程列表接口用（来自「课程列表」分支）。
// map 的遍历顺序不固定，这里先收集 id 排序再取值，保证返回顺序稳定。
func (s *memoryStore) List() []*Course {
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

// FindVideo 在指定课程下按视频 id 找视频（课程观看分支：播放 / 进度用）。
// 课程或视频不存在时返回错误。
func (s *memoryStore) FindVideo(courseId, videoId int) (*Video, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.courses[courseId]
	if !ok {
		return nil, errors.New("课程不存在")
	}
	for _, v := range c.Videos {
		if v.Id == videoId {
			return v, nil
		}
	}
	return nil, errors.New("视频不存在")
}

// ===== 以下为「课程资源」分支新增 =====

// AddResource 给课程追加一个文档资源。
// 课程内部的资源 id 从 1 开始自增；课程不存在时报错。
func (s *memoryStore) AddResource(id int, res *Resource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.courses[id]
	if !ok {
		return errors.New("课程不存在")
	}
	c.nextResourceId++
	res.Id = c.nextResourceId
	c.Resources = append(c.Resources, res)
	return nil
}

// ListResources 返回某课程的全部资源（按上传顺序）。
// 课程不存在时报错。
func (s *memoryStore) ListResources(id int) ([]*Resource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.courses[id]
	if !ok {
		return nil, errors.New("课程不存在")
	}
	return c.Resources, nil
}

// DeleteResource 删除课程下的某个资源。
// 课程或资源不存在时返回错误。
func (s *memoryStore) DeleteResource(id, resourceId int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.courses[id]
	if !ok {
		return errors.New("课程不存在")
	}
	for i, r := range c.Resources {
		if r.Id == resourceId {
			c.Resources = append(c.Resources[:i], c.Resources[i+1:]...)
			return nil
		}
	}
	return errors.New("资源不存在")
}

// MarkVideoFaststart 内存实现：直接改对应视频对象的 URL 和标记。
func (s *memoryStore) MarkVideoFaststart(courseId, videoId int, newUrl string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.courses[courseId]
	if !ok {
		return errors.New("课程不存在")
	}
	for _, v := range c.Videos {
		if v.Id == videoId {
			v.Url = newUrl
			v.Faststarted = true
			return nil
		}
	}
	return errors.New("视频不存在")
}
