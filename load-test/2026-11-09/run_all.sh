#!/bin/bash
# 学习平台 loopback 压测编排（分块串行，每块控制在 8 分钟以内）
# 用法：bash run_all.sh A|B|C|D|all
# 目录约定：D=/tmp/lpperf-20261109，脚本和 tokens.json 都在里面
D=/tmp/lpperf-20261109
LOG=$D/orch.log

# 压测容器：--network host 直接打 127.0.0.1:8888，--memory 128m 防止压测进程本身吃爆内存
L() {
  echo "[$(date '+%T')] $*" >> $LOG
  docker run --rm --memory 128m --memory-swap 192m --network host -v $D:/w node:24-alpine \
    node /w/perfload.js --tokens=/w/tokens.json --min-free-mb=45 --out=/w/raw "$@" >> $LOG 2>&1
  echo "  -> exit=$?" >> $LOG
  sleep 15
}

CHUNK="${1:-A}"
echo "=== 块 $CHUNK 开始 $(date '+%F %T') ===" >> $LOG

case "$CHUNK" in
  A)
    # 公开只读接口：课程列表（带缓存/DB 查询）、课程详情
    L P1a course_list --mode=simple --label=course_list --path='/api/course/list?page=1&pageSize=20' --ladder=8,16,32,64,128 --secs=30 --step-gap=15
    L P1b course_detail --mode=simple --label=course_detail --path='/api/course/detail/5' --ladder=8,16,32,64,128 --secs=30 --step-gap=15
    ;;
  B)
    # 登录接口（bcrypt CPU 密集，小并发阶梯）
    L P2a login --mode=login --label=login --ladder=4,8,16 --secs=30 --step-gap=15
    ;;
  C)
    # 鉴权中间件开销：带 token 打需要登录的空列表接口（Redis 会话查询 + 角色缓存 + 空 SQL）
    L P3a user_info --mode=simple --label=user_info --path='/api/user/info' --auth --ladder=8,16,32,64 --secs=30 --step-gap=15
    L P3b class_my --mode=simple --label=class_my --path='/api/class/my' --auth --ladder=8,16,32,64 --secs=30 --step-gap=15
    L P3c notifications --mode=simple --label=notifications --path='/api/notifications?page=1&pageSize=20' --auth --ladder=8,16,32,64 --secs=30 --step-gap=15
    L P3d my_exams --mode=simple --label=my_exams --path='/api/my/exams' --auth --ladder=8,16,32,64 --secs=30 --step-gap=15
    L P3e my_assignments --mode=simple --label=my_assignments --path='/api/my/assignments' --auth --ladder=8,16,32,64 --secs=30 --step-gap=15
    L P3f class_all --mode=simple --label=class_all --path='/api/class/all' --auth --ladder=8,16,32,64 --secs=30 --step-gap=15
    ;;
  D)
    # 写接口（小并发，避免污染生产数据太多）
    L P4a note_add --mode=note-add --label=note_add --ladder=2,4,8 --secs=30 --step-gap=15
    L P4b progress_save --mode=progress --label=progress_save --ladder=2,4,8 --secs=30 --step-gap=15
    ;;
  all)
    echo "[$(date '+%T')] bootstrap 探针账号" >> $LOG
    docker run --rm --network host -v $D:/w node:24-alpine node /w/bootstrap.js /w/tokens.json lpperf >> $LOG 2>&1
    docker run --rm -v $D:/w node:24-alpine node -e 'console.log("tokens="+require("/w/tokens.json").length)' >> $LOG 2>&1
    sleep 10
    "$0" A
    "$0" B
    "$0" C
    "$0" D
    ;;
esac
echo "=== 块 $CHUNK 结束 $(date '+%F %T') ===" >> $LOG
