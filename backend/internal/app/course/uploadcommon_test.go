package course

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// 图片魔数校验用例：覆盖 JPEG / PNG / GIF 与非法文件。
func TestCheckImageMagic(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{"png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}, true},
		{"gif", []byte{'G', 'I', 'F', '8', '9', 'a', 0, 0, 0, 0, 0, 0}, true},
		{"exe", []byte{'M', 'Z', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, false},
		{"empty", nil, false},
		{"too_short", []byte{0xFF, 0xD8}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckImageMagic(tc.data); got != tc.want {
				t.Fatalf("CheckImageMagic(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// 视频魔数校验用例：覆盖 MP4 / WebM / AVI 与非法文件。
func TestCheckVideoMagic(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"mp4", []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, true},
		{"webm", []byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{"avi", []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'A', 'V', 'I', ' '}, true},
		{"txt", []byte("hello world!!"), false},
		{"short", []byte{0, 0, 0, 0}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckVideoMagic(tc.data); got != tc.want {
				t.Fatalf("CheckVideoMagic(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// MIME 映射用例。
func TestContentTypeByExt(t *testing.T) {
	cases := []struct {
		ext  string
		want string
	}{
		{".jpg", "image/jpeg"},
		{".PNG", "image/png"},
		{".mp4", "video/mp4"},
		{".mov", "video/quicktime"},
		{".webm", "video/webm"},
		{".avi", "video/x-msvideo"},
		{".mkv", "video/x-matroska"},
		{".zip", "application/octet-stream"},
	}
	for _, tc := range cases {
		if got := ContentTypeByExt(tc.ext); got != tc.want {
			t.Fatalf("ContentTypeByExt(%s) = %s, want %s", tc.ext, got, tc.want)
		}
	}
}

// 单文件硬上限 800MB：未配置时取默认值，配置显式给值时以配置为准。
// 前端 videoUpload.js 的 MAX_FILE_BYTES 同样是 800MB，改一边必须改另一边。
func TestEffectiveMaxVideoBytes(t *testing.T) {
	ctx := newTestSvcCtx()
	ctx.Config.Upload.MaxVideoBytes = 0
	if got := EffectiveMaxVideoBytes(ctx); got != 800<<20 {
		t.Fatalf("默认单文件上限 = %d 字节，期望 %d（800MB）", got, int64(800<<20))
	}
	ctx.Config.Upload.MaxVideoBytes = 300 << 20
	if got := EffectiveMaxVideoBytes(ctx); got != 300<<20 {
		t.Fatalf("配置了 300MB 却返回 %d，配置没生效", got)
	}
}

// 部署用的 yaml 必须和代码默认值一致：只改代码不改配置，
// 线上会继续按旧上限放行/拒绝，而且这种漂移在本地跑测试时看不出来。
func TestUploadYamlVideoLimitMatchesDefault(t *testing.T) {
	raw, err := os.ReadFile("../../../etc/learningplatform-api.yaml")
	if err != nil {
		t.Fatalf("读不到部署配置: %v", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "MaxVideoBytes:") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "MaxVideoBytes:"))
		if len(fields) == 0 {
			t.Fatal("MaxVideoBytes 没有值")
		}
		n, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			t.Fatalf("MaxVideoBytes 不是纯数字（环境变量占位符会让上限失效）: %v", err)
		}
		if n != defaultMaxVideoBytes {
			t.Fatalf("yaml 上限 %d 与代码默认值 %d 不一致", n, defaultMaxVideoBytes)
		}
		return
	}
	t.Fatal("部署配置里没有 MaxVideoBytes")
}
