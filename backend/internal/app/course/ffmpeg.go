package course

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"learningplatform/internal/svc"

	"github.com/google/uuid"
)

// ===== FFmpeg faststart 转码封装（来自「课程观看」分支）=====
//
// 参考 S:\videonest 的做法：把 MP4 的 moov 原子（索引信息）通过
//   -movflags +faststart
// 从文件尾部挪到文件头部。这样浏览器不用等整个文件下载完才起播，
// 可以边下边播、拖动进度条 seek 时也能快速定位。
//
// 教学版只做"重封装 + H.264/AAC 统一"，不做多档清晰度（videonest 的 480/720/1080
// 是生产级复杂度，这里不需要）。

// ffmpegTimeout 单次转码超时（教学视频不大，给 10 分钟兜底）。
const ffmpegTimeout = 10 * time.Minute

// ffmpegPath 取 ffmpeg 可执行文件路径。
// 优先环境变量 FFMPEG_PATH（容器里可指向安装路径），默认直接用 PATH 里的 ffmpeg。
func ffmpegPath() string {
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		return p
	}
	return "ffmpeg"
}

// buildFaststartArgs 拼装 ffmpeg 命令行参数（纯函数，便于单测）。
// 关键参数与 videonest 对齐：
//   -c:v libx264 -pix_fmt yuv420p   统一 H.264 编码，浏览器都能播
//   -c:a aac                        音频统一 AAC
//   -movflags +faststart            moov 前置，边下边播 / 可拖动
func buildFaststartArgs(src, dst string) []string {
	return []string{
		"-y", // 覆盖已存在的输出
		"-i", src,
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-preset", "veryfast", // 教学演示用快预设，不等精细压缩
		"-c:a", "aac",
		"-movflags", "+faststart",
		dst,
	}
}

// runFfmstart 执行 ffmpeg 命令并等待完成，超时强杀。
// 返回的 error 含退出码和尾部日志，便于排查。
func runFfmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, ffmpegPath(), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return errors.New("ffmpeg 处理超时")
		}
		tail := string(output)
		if len(tail) > 800 {
			tail = tail[len(tail)-800:]
		}
		return fmt.Errorf("ffmpeg 失败: %v; 输出尾部: %s", err, tail)
	}
	return nil
}

// ProcessVideoFaststart 把指定视频从 MinIO 拉下来，跑 faststart 转码后重新上传，
// 返回 faststart 版本的新 MinIO URL。任何一步失败都返回 error，调用方应忽略、
// 继续用原始 URL 播放（MinIO 本身支持 range 请求，浏览器多数仍能拖动）。
//
// video.objectName 为源对象在桶内的路径。
func ProcessVideoFaststart(ctx context.Context, svcCtx *svc.ServiceContext, video *Video) (string, error) {
	if svcCtx.Oss == nil || video == nil || video.objectName == "" {
		return "", errors.New("缺少对象存储或源对象名")
	}

	// 1. 下载源视频到临时文件
	rc, err := svcCtx.Oss.Download(ctx, video.objectName)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	tmpDir, err := os.MkdirTemp("", "lp-faststart-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "source"+filepath.Ext(video.objectName))
	dstPath := filepath.Join(tmpDir, "faststart.mp4")

	f, err := os.Create(srcPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, rc); err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	// 2. 跑 ffmpeg
	execCtx, cancel := context.WithTimeout(ctx, ffmpegTimeout)
	defer cancel()
	if err := runFfmpeg(execCtx, buildFaststartArgs(srcPath, dstPath)); err != nil {
		return "", err
	}

	// 3. 上传 faststart 版本回 MinIO（新对象，不动原文件）
	dstObject := fmt.Sprintf("course/%d/videos/faststart/%s.mp4", video.courseId, uuid.NewString())
	out, err := os.Open(dstPath)
	if err != nil {
		return "", err
	}
	defer out.Close()
	stat, _ := out.Stat()
	url, err := svcCtx.Oss.PutObject(ctx, dstObject, out, stat.Size(), "video/mp4")
	if err != nil {
		return "", err
	}
	return url, nil
}
