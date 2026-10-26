package class

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 测 BizError.StatusCode 方法存在且返回 Code 字段（response.codedError 鸭子类型契约）。
func TestBizError_StatusCode(t *testing.T) {
	be := &BizError{Code: http.StatusForbidden, Msg: "无权限"}
	if be.StatusCode() != http.StatusForbidden {
		t.Errorf("StatusCode 期望 403，实际 %d", be.StatusCode())
	}
	if be.Error() != "无权限" {
		t.Errorf("Error() 期望返回 Msg，实际 %q", be.Error())
	}
}

// 测 WriteBizError 输出三层结构 {code,message,data}。
func TestWriteBizError_Envelope(t *testing.T) {
	w := httptest.NewRecorder()
	WriteBizError(w, &BizError{Code: http.StatusBadRequest, Msg: "参数错"})

	if w.Code != http.StatusBadRequest {
		t.Errorf("HTTP 状态码期望 400，实际 %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{`"code":400`, `"message":"参数错"`} {
		if !strings.Contains(body, want) {
			t.Errorf("响应体期望包含 %s，实际 %s", want, body)
		}
	}
}

// 测非 BizError 的普通 error 走 500 分支。
func TestWriteBizError_PlainError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteBizError(w, errors.New("db down"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("普通 error 期望 500，实际 %d", w.Code)
	}
}
