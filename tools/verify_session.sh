#!/usr/bin/env bash
# 端到端自检：验证三件事
#   1. ${session_id} 按对话稳定注入上游请求头
#   2. 不同工具的访问密钥映射到不同标签
#   3. 转录按工具归因，且能提取 token 用量
#
# 前置：tools/echo_server.py 已监听 8899，代理以 config.echo-test.yaml 监听 8898。
set -u

WORKBUDDY_KEY="bench-workbuddy-01"
QODER_KEY="bench-qoder-01"
HOST="http://127.0.0.1:8898"

call() {  # call <access_key> <json body>
  curl -s --noproxy '*' --max-time 20 -X POST "$HOST/$1/proxy/echo/v1/chat/completions" \
    -H "Content-Type: application/json" -d "$2"
}

session_of() { python -c "import sys,json;print(json.load(sys.stdin)['received_headers'].get('x-opencode-session'))"; }

echo "### 对话A 第1轮（WorkBuddy，首条 user = 什么是闭包）"
A1=$(call "$WORKBUDDY_KEY" '{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"什么是闭包"}]}')
echo "$A1" | python -c "
import sys,json
h=json.load(sys.stdin)['received_headers']
print('  --- 上游实际收到的请求头 ---')
for k,v in sorted(h.items()): print(f'    {k}: {v}')
"

echo "### 对话A 第2轮（WorkBuddy，历史变长，首条 user 不变）"
A2=$(call "$WORKBUDDY_KEY" '{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"什么是闭包"},{"role":"assistant","content":"闭包是函数与其词法作用域的组合"},{"role":"user","content":"举个例子"}]}')
echo "  session = $(echo "$A2" | session_of)"

echo "### 对话B（换成 Qoder 的密钥，不同问题）"
B1=$(call "$QODER_KEY" '{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"解释一下事件循环"}]}')
echo "  session = $(echo "$B1" | session_of)"

echo "### 对话A 变体（WorkBuddy，带 system 前缀，应被跳过）"
A3=$(call "$WORKBUDDY_KEY" '{"model":"deepseek-v4.1-flash","messages":[{"role":"system","content":"你是助手"},{"role":"user","content":"什么是闭包"}]}')
echo "  session = $(echo "$A3" | session_of)"

echo "### 无效密钥应被拒绝"
curl -s --noproxy '*' --max-time 10 -o /dev/null -w "  HTTP:%{http_code}\n" \
  -X POST "$HOST/wrong-key-000000/proxy/echo/v1/chat/completions" \
  -H "Content-Type: application/json" -d '{"model":"m","messages":[]}'

echo
echo "=== 断言 ==="
python - "$A1" "$A2" "$B1" "$A3" <<'PY'
import json, sys
def sid(raw):
    return json.loads(raw)['received_headers'].get('x-opencode-session')
a1, a2, b1, a3 = (sid(x) for x in sys.argv[1:5])
print(f"  A1={a1}\n  A2={a2}\n  B1={b1}\n  A3={a3}")
checks = [
    ("同一对话多次请求会话标识稳定", a1 == a2),
    ("不同对话会话标识互相区分",     a1 != b1),
    ("跳过 system 消息取首条 user",  a1 == a3),
    ("会话标识带 sess- 前缀",        bool(a1) and a1.startswith("sess-")),
]
for label, ok in checks:
    print(f"  [{'OK' if ok else '!!'}] {label}")
print("  RESULT:", "PASS" if all(ok for _, ok in checks) else "FAIL")
PY

echo
echo "=== 转录按工具汇总 ==="
python "$(dirname "$0")/summarize_transcript.py" "$(dirname "$0")/../data/echo-transcripts.jsonl"
