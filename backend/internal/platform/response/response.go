// Package response 统一 HTTP 响应封装。
//
// 设计说明：
// 项目历史接口（course/user/class 大部分）直接用 httpx.WriteJson 返回裸 data，
// 前端已经按这个格式解析，不能一刀切全改。本包只做两件事：
//  1. 新接口统一走 OK / Error，返回 {code, message, data} 三层结构；
//  2. 错误响应（不管新旧接口）统一由 WriteError 输出三层结构，
//     通过 codedError 鸭子类型识别 *class.BizError，避免循环 import。
package response

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Envelope 统一响应体。
// Code: 0=成功；非 0=业务错误码（默认取 HTTP 状态码）。
type Envelope struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// codedError 业务错误接口：实现了 StatusCode() int 的 error 都被识别为业务错误。
// *class.BizError 加一个 StatusCode() 方法即可，无需反向依赖。
type codedError interface {
	error
	StatusCode() int
}

// OK 成功响应：HTTP 200，code=0，data 原样返回。
func OK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "ok", Data: data})
}

// Created 资源创建成功：HTTP 201。
func Created(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusCreated, Envelope{Code: 0, Message: "created", Data: data})
}

// Error 统一错误响应。
//   - codedError（如 *class.BizError）：用其 StatusCode() 作为 HTTP 状态码和业务 code；
//   - 其他 error：HTTP 500，message 用 err.Error()（生产环境可改成"服务器内部错误"）。
func Error(w http.ResponseWriter, err error) {
	var ce codedError
	if errors.As(err, &ce) {
		writeJSON(w, ce.StatusCode(), Envelope{Code: ce.StatusCode(), Message: ce.Error()})
		return
	}
	writeJSON(w, http.StatusInternalServerError, Envelope{
		Code:    http.StatusInternalServerError,
		Message: err.Error(),
	})
}

// WriteError 兼容旧调用：class.WriteBizError 直接委托到这里，避免前端拿到裸 {"message":...}。
func WriteError(w http.ResponseWriter, err error) {
	Error(w, err)
}

func writeJSON(w http.ResponseWriter, status int, body Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
