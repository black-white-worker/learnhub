package course

import "testing"

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
