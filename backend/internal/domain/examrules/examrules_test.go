package examrules

import (
	"errors"
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02 15:04:05"} {
		if v, err := time.Parse(layout, s); err == nil {
			return v
		}
	}
	t.Fatalf("解析时间失败: %s", s)
	return time.Time{}
}

func baseWindow(t *testing.T) Window {
	return Window{
		StartAt:     mustTime(t, "2026-05-01 09:00"),
		EndAt:       mustTime(t, "2026-05-01 11:00"),
		DurationMin: 60,
	}
}

// Deadline 必须基于"个人开考时间"，而不是考试开始时间。
func TestDeadline_UsesPersonalStart(t *testing.T) {
	w := baseWindow(t)
	// 9:00 进场 → 10:00 截止（时长 60 分钟）
	if got := w.Deadline(mustTime(t, "2026-05-01 09:00")); !got.Equal(mustTime(t, "2026-05-01 10:00")) {
		t.Fatalf("9:00 进场应在 10:00 截止，实际 %v", got)
	}
	// 9:40 进场（迟到 40 分钟）→ 11:00 整场结束，取较早者 10:40
	if got := w.Deadline(mustTime(t, "2026-05-01 09:40")); !got.Equal(mustTime(t, "2026-05-01 10:40")) {
		t.Fatalf("9:40 进场应在 10:40 截止，实际 %v", got)
	}
	// 10:30 进场 → 个人 11:30，但整场 11:00 结束，取 11:00
	if got := w.Deadline(mustTime(t, "2026-05-01 10:30")); !got.Equal(mustTime(t, "2026-05-01 11:00")) {
		t.Fatalf("10:30 进场应在 11:00 截止，实际 %v", got)
	}
}

func TestDeadline_NoDurationFallsBackToEnd(t *testing.T) {
	w := baseWindow(t)
	w.DurationMin = 0
	if got := w.Deadline(mustTime(t, "2026-05-01 09:10")); !got.Equal(w.EndAt) {
		t.Fatalf("没配时长应只受整场结束限制，实际 %v", got)
	}
}

func TestCheckStart(t *testing.T) {
	w := baseWindow(t)
	if err := w.CheckStart(mustTime(t, "2026-05-01 08:59")); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("未开考应拒绝，实际 %v", err)
	}
	if err := w.CheckStart(mustTime(t, "2026-05-01 09:30")); err != nil {
		t.Fatalf("开考中应允许，实际 %v", err)
	}
	if err := w.CheckStart(mustTime(t, "2026-05-01 11:01")); !errors.Is(err, ErrEnded) {
		t.Fatalf("结束后不允许开考，实际 %v", err)
	}
}

func TestCheckSubmit_WithinPersonalDeadline(t *testing.T) {
	w := baseWindow(t)
	started := mustTime(t, "2026-05-01 09:00")
	if err := w.CheckSubmit(mustTime(t, "2026-05-01 09:59"), started); err != nil {
		t.Fatalf("时限内应允许交卷，实际 %v", err)
	}
}

func TestCheckSubmit_PersonalTimeoutBeforeExamEnd(t *testing.T) {
	w := baseWindow(t)
	started := mustTime(t, "2026-05-01 09:00") // 10:00 个人截止
	// 10:30 交卷：整场没结束，但个人时限已过
	err := w.CheckSubmit(mustTime(t, "2026-05-01 10:30"), started)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("个人超时应返回 ErrExpired，实际 %v", err)
	}
}

func TestCheckSubmit_AllowLate(t *testing.T) {
	w := baseWindow(t)
	w.AllowLate = true
	w.LateDeadline = mustTime(t, "2026-05-01 12:00")
	started := mustTime(t, "2026-05-01 09:00")
	if err := w.CheckSubmit(mustTime(t, "2026-05-01 11:30"), started); err != nil {
		t.Fatalf("补交窗口内应允许，实际 %v", err)
	}
	if err := w.CheckSubmit(mustTime(t, "2026-05-01 12:01"), started); !errors.Is(err, ErrEnded) {
		t.Fatalf("补交截止后应拒绝，实际 %v", err)
	}
}

// allow_late 但没配 late_deadline：只给 24 小时兜底，不能永久可交。
func TestCheckSubmit_AllowLateWithoutDeadline(t *testing.T) {
	w := baseWindow(t)
	w.AllowLate = true
	started := mustTime(t, "2026-05-01 09:00")
	if err := w.CheckSubmit(mustTime(t, "2026-05-02 10:00"), started); err != nil {
		t.Fatalf("24 小时内应允许补交，实际 %v", err)
	}
	if err := w.CheckSubmit(mustTime(t, "2026-05-02 11:30"), started); err == nil {
		t.Fatal("超过 24 小时兜底后应拒绝")
	}
}

// 答案只在整场结束后可见：考试期间先交卷的学生也看不到。
func TestRevealAnswers(t *testing.T) {
	w := baseWindow(t)
	if w.RevealAnswers(mustTime(t, "2026-05-01 09:30")) {
		t.Fatal("考试进行中不应下发标准答案")
	}
	if !w.RevealAnswers(mustTime(t, "2026-05-01 11:00:01")) {
		t.Fatal("考试结束后应允许查看答案")
	}
}

func TestAutoSubmitDue(t *testing.T) {
	w := baseWindow(t)
	started := mustTime(t, "2026-05-01 09:00")
	if w.AutoSubmitDue(mustTime(t, "2026-05-01 09:59"), started) {
		t.Fatal("时限内不应触发自动交卷")
	}
	if !w.AutoSubmitDue(mustTime(t, "2026-05-01 10:00:01"), started) {
		t.Fatal("超时后应触发自动交卷")
	}
	if w.AutoSubmitDue(mustTime(t, "2026-05-01 10:00:01"), time.Time{}) {
		t.Fatal("没有开考记录时不应自动交卷")
	}
}
