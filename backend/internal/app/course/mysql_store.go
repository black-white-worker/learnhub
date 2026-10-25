package course

import (
	"database/sql"
	"errors"
	"time"

	"learningplatform/internal/model"
)

// mysqlStore 课程存储的 MySQL 实现。
// 生产环境由 svc 注入，logic 层通过包级 store 接口调用，无感知。
type mysqlStore struct {
	db *model.CourseModel
}

// InitCourseStore 把包级 store 从内存实现切换为 MySQL 实现（生产启动时调一次）。
func InitCourseStore(db *model.CourseModel) {
	if db != nil {
		Store = &mysqlStore{db: db}
	}
}

// 把 VideoRow 转成 course.Video（含私有字段，同包可访问）。
func videoFromRow(r *model.VideoRow) *Video {
	return &Video{
		Id:         int(r.Id),
		Name:       r.Name,
		Url:        r.Url,
		Size:       r.Size,
		UploadedAt: r.UploadedAt.Format(time.RFC3339),
		Faststarted: r.Faststarted,
		CourseId:   int(r.CourseId),
		ObjectName: r.ObjectName,
	}
}

// 把 ResourceRow 转成 course.Resource。
func resourceFromRow(r *model.ResourceRow) *Resource {
	return &Resource{
		Id:         int(r.Id),
		Name:       r.Name,
		Url:        r.Url,
		ObjectName: r.ObjectName,
		Size:       r.Size,
		Type:       r.Type,
		UploadedAt: r.UploadedAt.Format(time.RFC3339),
	}
}

// 把 CourseRow 转成 course.Course，并联查视频和资源填进去。
func (m *mysqlStore) courseFromRow(r *model.CourseRow, withChildren bool) (*Course, error) {
	c := &Course{
		Id:            int(r.Id),
		Name:          r.Name,
		Intro:         r.Intro,
		Teacher:       r.Teacher,
		CreatedBy:     r.CreatedBy,
		CoverUrl:      r.CoverUrl,
		CoverThumbUrl: r.CoverThumbUrl,
		Videos:        []*Video{},
		Resources:     []*Resource{},
	}
	if !withChildren {
		return c, nil
	}
	videos, err := m.db.ListVideos(r.Id)
	if err != nil {
		return nil, err
	}
	for _, v := range videos {
		c.Videos = append(c.Videos, videoFromRow(v))
	}
	resources, err := m.db.ListResources(r.Id)
	if err != nil {
		return nil, err
	}
	for _, r := range resources {
		c.Resources = append(c.Resources, resourceFromRow(r))
	}
	return c, nil
}

func (m *mysqlStore) AddCourse(name, intro, teacher, createdBy string) (*Course, error) {
	if name == "" {
		return nil, errors.New("课程名称不能为空")
	}
	id, err := m.db.InsertCourse(name, intro, teacher, createdBy)
	if err != nil {
		return nil, err
	}
	return &Course{Id: int(id), Name: name, Intro: intro, Teacher: teacher, CreatedBy: createdBy,
		Videos: []*Video{}, Resources: []*Resource{}}, nil
}

func (m *mysqlStore) GetCourse(id int) (*Course, error) {
	row, err := m.db.FindCourseById(int64(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("课程不存在")
		}
		return nil, err
	}
	return m.courseFromRow(row, true)
}

func (m *mysqlStore) SetCover(id int, coverUrl, coverThumbUrl string) error {
	return m.db.UpdateCourseCover(int64(id), coverUrl, coverThumbUrl)
}

func (m *mysqlStore) AddVideos(id int, videos []*Video) error {
	rows := make([]*model.VideoRow, 0, len(videos))
	for _, v := range videos {
		rows = append(rows, &model.VideoRow{
			Name: v.Name, Url: v.Url, ObjectName: v.ObjectName, Size: v.Size,
			Faststarted: v.Faststarted,
		})
	}
	if err := m.db.InsertVideos(int64(id), rows); err != nil {
		return err
	}
	// 回填自增 id 回原 Video 对象，调用方需要（faststart 异步流程会用到 v.Id）
	for i, v := range videos {
		v.Id = int(rows[i].Id)
	}
	return nil
}

func (m *mysqlStore) UpdateCourse(id int, name, intro, teacher string) (*Course, error) {
	if name == "" {
		return nil, errors.New("课程名称不能为空")
	}
	if err := m.db.UpdateCourse(int64(id), name, intro, teacher); err != nil {
		return nil, err
	}
	return m.GetCourse(id)
}

func (m *mysqlStore) DeleteCourse(id int) error {
	return m.db.DeleteCourse(int64(id))
}

func (m *mysqlStore) List() []*Course {
	rows, err := m.db.ListCourses()
	if err != nil {
		return []*Course{}
	}
	result := make([]*Course, 0, len(rows))
	for _, r := range rows {
		c, err := m.courseFromRow(r, true)
		if err != nil {
			continue
		}
		result = append(result, c)
	}
	return result
}

func (m *mysqlStore) FindVideo(courseId, videoId int) (*Video, error) {
	row, err := m.db.FindVideo(int64(courseId), int64(videoId))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("视频不存在")
		}
		return nil, err
	}
	return videoFromRow(row), nil
}

func (m *mysqlStore) AddResource(id int, res *Resource) error {
	row := &model.ResourceRow{
		Name: res.Name, Url: res.Url, ObjectName: res.ObjectName, Type: res.Type, Size: res.Size,
	}
	if err := m.db.InsertResource(int64(id), row); err != nil {
		return err
	}
	res.Id = int(row.Id)
	return nil
}

func (m *mysqlStore) ListResources(id int) ([]*Resource, error) {
	rows, err := m.db.ListResources(int64(id))
	if err != nil {
		return nil, err
	}
	result := make([]*Resource, 0, len(rows))
	for _, r := range rows {
		result = append(result, resourceFromRow(r))
	}
	return result, nil
}

func (m *mysqlStore) DeleteResource(id, resourceId int) error {
	return m.db.DeleteResource(int64(id), int64(resourceId))
}

func (m *mysqlStore) MarkVideoFaststart(courseId, videoId int, newUrl string) error {
	return m.db.MarkVideoFaststart(int64(courseId), int64(videoId), newUrl)
}
