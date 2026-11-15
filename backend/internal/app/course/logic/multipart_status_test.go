package logic

import "testing"

// parsePartNumber：断点续传把 MinIO 里的对象名翻译回片号。
// 报错方向必须是"忽略这片 → 前端重传"，绝不能把不相干的对象当成分片报上去 ——
// 少报一片只是多传几 MB，多报一片会合出一个字节错位的视频。
func TestParsePartNumber(t *testing.T) {
	base := "course/3/videos/abc.mp4"
	cases := []struct {
		name   string
		object string
		want   int
		wantOK bool
	}{
		{"正常第 1 片", base + ".part1", 1, true},
		{"两位数片号", base + ".part12", 12, true},
		{"最后一片", base + ".part1000", 1000, true},
		{"最终对象本身不算分片", base, 0, false},
		{"faststart 产物不算分片", "course/3/videos/faststart/x.mp4", 0, false},
		{"后缀不是数字", base + ".partX", 0, false},
		{"片号 0 非法", base + ".part0", 0, false},
		{"片号为负", base + ".part-1", 0, false},
		{"空后缀", base + ".part", 0, false},
	}
	for _, c := range cases {
		got, ok := parsePartNumber(c.object, base)
		if got != c.want || ok != c.wantOK {
			t.Errorf("%s: parsePartNumber(%q) = (%d,%v)，期望 (%d,%v)", c.name, c.object, got, ok, c.want, c.wantOK)
		}
	}
}
