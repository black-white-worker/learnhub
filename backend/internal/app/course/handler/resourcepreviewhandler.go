package handler

// 讲义在线预览。
//
// 设计要点（对标 Canvas / Moodle 的资源预览）：
//  1. PDF 直接返回，浏览器内置阅读器打开；
//  2. docx / pptx 用 LibreOffice 转成 PDF，转换结果按资源 id 缓存到 MinIO，
//     第二次访问直接命中缓存，不再占用 CPU；
//  3. 转 PDF 是重操作：全局并发上限 + 单次超时，避免一个循环把服务器打满；
//  4. 每个转换进程使用独立的 LibreOffice profile 目录，
//     否则并发调用会抢同一个 profile 锁而失败；
//  5. 只有通过课程媒体鉴权的用户能触发，未登录直接 401。

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
)

const (
	previewConvertTimeout = 90 * time.Second
	// previewMaxConcurrent 同时进行的 LibreOffice 转换数。
	// 与容器 CPU 核数相关，2 是 2 核机器上的稳妥值。
	previewMaxConcurrent = 2
	previewQueueTimeout  = 30 * time.Second
	previewCacheTTL      = 24 * time.Hour
)

// previewSem 全局转换信号量：超时拿不到就返回 503，而不是无限排队。
var previewSem = make(chan struct{}, previewMaxConcurrent)

// ResourcePreviewHandler 资源在线预览：GET /api/course/resource/preview/:courseId/:resourceId
func ResourcePreviewHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CourseId   int `path:"courseId"`
			ResourceId int `path:"resourceId"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			courseError(r, w, course.ErrBadRequest("参数错误："+err.Error()))
			return
		}
		if req.CourseId <= 0 || req.ResourceId <= 0 {
			courseError(r, w, course.ErrBadRequest("参数错误"))
			return
		}
		// 1. 鉴权：与视频、资源列表同一套规则
		if !guardCourseMedia(w, r, svcCtx, req.CourseId) {
			return
		}

		// 2. 找到资源
		c, err := course.Store.GetCourse(req.CourseId)
		if err != nil {
			courseError(r, w, course.ErrNotFound("课程不存在"))
			return
		}
		var res *course.Resource
		for _, x := range c.Resources {
			if x.Id == req.ResourceId {
				res = x
				break
			}
		}
		if res == nil {
			courseError(r, w, course.ErrNotFound("资源不存在"))
			return
		}

		// 3. PDF：直接 302 到（必要时预签名的）直链
		// 历史数据的 object_name 可能为空，从 URL 反解兜底（私有桶签名需要它）
		objectName := res.ObjectName
		if objectName == "" {
			objectName = course.ObjectNameFromURL(res.Url, svcCtx.Config.Minio.Bucket)
		}
		if strings.EqualFold(res.Type, "pdf") {
			http.Redirect(w, r, course.MediaURL(svcCtx, objectName, res.Url), http.StatusFound)
			return
		}

		if svcCtx.Oss == nil {
			courseError(r, w, fmt.Errorf("对象存储未初始化"))
			return
		}

		// 4. 命中预览缓存：直接流式返回，不再调 LibreOffice
		previewName := course.PreviewObjectName(req.CourseId, req.ResourceId)
		if obj, ok := openIfExists(r.Context(), svcCtx, previewName); ok {
			defer obj.Close()
			writePDFHeaders(w)
			if _, err := io.Copy(w, obj); err != nil {
				// 响应体已经开始写出，无法再改状态码，只能记录
				logx.WithContext(r.Context()).Errorf("预览缓存写出失败 %s: %v", previewName, err)
			}
			return
		}

		// 5. 未命中：转换（并发受限）
		select {
		case previewSem <- struct{}{}:
			defer func() { <-previewSem }()
		case <-time.After(previewQueueTimeout):
			courseError(r, w, &course.BizError{Code: http.StatusServiceUnavailable, Msg: "预览服务繁忙，请稍后再试"})
			return
		}

		pdfBytes, err := convertToPDF(r.Context(), svcCtx, res, objectName)
		if err != nil {
			courseError(r, w, &course.BizError{Code: http.StatusBadGateway, Msg: "文档转 PDF 失败：" + err.Error()})
			return
		}

		// 6. 缓存转换结果（失败不影响本次预览）
		if _, err := svcCtx.Oss.PutObject(r.Context(), previewName, bytes.NewReader(pdfBytes),
			int64(len(pdfBytes)), "application/pdf"); err != nil {
			logx.WithContext(r.Context()).Errorf("预览缓存写入失败 %s: %v", previewName, err)
		}

		writePDFHeaders(w)
		_, _ = w.Write(pdfBytes)
	}
}

// writePDFHeaders 预览响应头：内联打开 + 允许浏览器短期缓存。
func writePDFHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="preview.pdf"`)
	w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d", int(previewCacheTTL.Seconds())))
}

// openIfExists 打开对象；对象不存在时返回 ok=false（而不是把错误抛给调用方）。
func openIfExists(ctx context.Context, svcCtx *svc.ServiceContext, objectName string) (io.ReadCloser, bool) {
	infos, err := svcCtx.Oss.ListObjects(ctx, objectName)
	if err != nil {
		return nil, false
	}
	found := false
	for _, info := range infos {
		if info.Name == objectName {
			found = true
			break
		}
	}
	if !found {
		return nil, false
	}
	obj, err := svcCtx.Oss.Download(ctx, objectName)
	if err != nil {
		return nil, false
	}
	return obj, true
}

// convertToPDF 下载源文件 → LibreOffice 转 PDF → 读回内存。
//
// 资源文件大小受上传上限约束，读进内存是可接受的；
// 真要支持超大文件，应改成流式上传到 MinIO。
func convertToPDF(ctx context.Context, svcCtx *svc.ServiceContext, res *course.Resource, objectName string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "preview-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "src"+filepath.Ext(res.Name))
	obj, err := svcCtx.Oss.Download(ctx, objectName)
	if err != nil {
		return nil, fmt.Errorf("读取源文件失败: %w", err)
	}
	defer obj.Close()

	f, err := os.Create(srcPath)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(f, obj); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	execCtx, cancel := context.WithTimeout(ctx, previewConvertTimeout)
	defer cancel()

	// 独立 profile：LibreOffice 并发实例默认会抢同一个用户配置目录，
	// 第二个进程直接失败。每次转换给一个临时 profile 是官方推荐做法。
	// file:/// 前缀 + 去掉路径开头的斜杠：Linux 得到 file:///tmp/...，Windows 得到 file:///C:/...
	profile := filepath.Join(tmpDir, "lo-profile-"+uuid.NewString())
	profileURL := "file:///" + strings.TrimPrefix(filepath.ToSlash(profile), "/")
	cmd := exec.CommandContext(execCtx, "libreoffice",
		"-env:UserInstallation="+profileURL,
		"--headless", "--norestore", "--convert-to", "pdf",
		"--outdir", tmpDir, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("转换超时")
		}
		return nil, fmt.Errorf("%v, %s", err, strings.TrimSpace(string(out)))
	}

	converted := filepath.Join(tmpDir, strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))+".pdf")
	pdf, err := os.ReadFile(converted)
	if err != nil {
		return nil, fmt.Errorf("转换结果读取失败: %w", err)
	}
	if len(pdf) == 0 {
		return nil, fmt.Errorf("转换结果为空")
	}
	return pdf, nil
}
