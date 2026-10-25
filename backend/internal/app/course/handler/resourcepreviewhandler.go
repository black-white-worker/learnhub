package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ResourcePreviewHandler 资源在线预览：
//   - PDF：直接 302 跳 MinIO 直链，浏览器内置 PDF 阅读器打开
//   - docx / pptx：从 MinIO 拉原文件到临时目录，用 LibreOffice headless 转 PDF，
//     再把 PDF 内容写回响应流（Content-Type: application/pdf）
//
// 这样浏览器新标签页里看到的永远是 PDF 预览，不会触发下载。
func ResourcePreviewHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 用 go-zero 的 httpx.Parse 把路径参数 :courseId/:resourceId 解析出来
		var req struct {
			CourseId   int `path:"courseId"`
			ResourceId int `path:"resourceId"`
		}
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("参数错误: %w", err))
			return
		}
		courseId := req.CourseId
		resourceId := req.ResourceId
		if courseId <= 0 || resourceId <= 0 {
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("参数错误"))
			return
		}

		// 1. 从 store 找到这条资源
		c, err := course.Store.GetCourse(courseId)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		var res *course.Resource
		for _, x := range c.Resources {
			if x.Id == resourceId {
				res = x
				break
			}
		}
		if res == nil {
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("资源不存在"))
			return
		}

		// 2. PDF 直接 302，走 MinIO 直链
		if strings.EqualFold(res.Type, "pdf") {
			http.Redirect(w, r, res.Url, http.StatusFound)
			return
		}

		// 3. 其他类型（docx/pptx）需要转 PDF。先确保 MinIO 可用
		if svcCtx.Oss == nil {
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("对象存储未初始化"))
			return
		}

		// 3.1 建临时目录，把原文件拉下来
		tmpDir, err := os.MkdirTemp("", "preview-*")
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		defer os.RemoveAll(tmpDir)

		srcPath := filepath.Join(tmpDir, "src"+filepath.Ext(res.Name))
		outPath := filepath.Join(tmpDir, "out.pdf")

		obj, err := svcCtx.Oss.Download(r.Context(), res.ObjectName)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("下载源文件失败: %w", err))
			return
		}
		f, err := os.Create(srcPath)
		if err != nil {
			obj.Close()
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if _, err := io.Copy(f, obj); err != nil {
			f.Close()
			obj.Close()
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		f.Close()
		obj.Close()

		// 3.2 LibreOffice headless 转 PDF。--headless 无 GUI，--convert-to pdf 指定输出格式
		cmd := exec.Command("libreoffice", "--headless", "--norestore", "--convert-to", "pdf",
			"--outdir", tmpDir, srcPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("文档转 PDF 失败: %v, %s", err, string(out)))
			return
		}
		// LibreOffice 输出文件名 = 原文件名去扩展名 + .pdf
		converted := filepath.Join(tmpDir, strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))+".pdf")
		if _, err := os.Stat(converted); err != nil {
			// 兜底：有些版本输出名不同
			_ = converted
			converted = outPath
		}

		// 3.3 把转好的 PDF 写回响应
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "inline; filename=\"preview.pdf\"")
		pdfData, err := os.ReadFile(converted)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		w.Write(pdfData)
	}
}
