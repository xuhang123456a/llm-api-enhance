#!/usr/bin/env python3
"""按客户端工具汇总转录记录，输出 token 与耗时对比表。

    python tools/summarize_transcript.py data/transcripts.jsonl

列含义：
  工具        访问密钥对应的标签
  请求数      该工具发出的请求条数
  对话数      去重后的 session 数量
  输入/输出   prompt / completion token 合计
  合计        total token 合计
  平均耗时    单次请求平均毫秒数
  失败        记录到错误的请求条数
"""

import collections
import json
import sys


def load(path):
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            line = line.strip()
            if not line:
                continue
            try:
                yield json.loads(line)
            except json.JSONDecodeError:
                continue


def main():
    path = sys.argv[1] if len(sys.argv) > 1 else "data/transcripts.jsonl"
    try:
        records = list(load(path))
    except FileNotFoundError:
        print(f"找不到转录文件：{path}", file=sys.stderr)
        return 1

    if not records:
        print(f"{path} 里没有任何记录。")
        return 0

    stats = collections.defaultdict(lambda: {
        "requests": 0, "sessions": set(), "prompt": 0, "completion": 0,
        "total": 0, "elapsed": 0, "errors": 0,
    })
    for record in records:
        row = stats[record.get("tool") or "(未标注)"]
        row["requests"] += 1
        if record.get("session_id"):
            row["sessions"].add(record["session_id"])
        usage = record.get("usage") or {}
        row["prompt"] += usage.get("prompt_tokens", 0)
        row["completion"] += usage.get("completion_tokens", 0)
        row["total"] += usage.get("total_tokens", 0)
        row["elapsed"] += record.get("elapsed_ms", 0)
        if record.get("error"):
            row["errors"] += 1

    header = ("工具", "请求数", "对话数", "输入", "输出", "合计", "平均耗时", "失败")
    rows = []
    for tool, row in sorted(stats.items(), key=lambda kv: -kv[1]["total"]):
        average = row["elapsed"] // row["requests"] if row["requests"] else 0
        rows.append((
            tool, str(row["requests"]), str(len(row["sessions"])),
            str(row["prompt"]), str(row["completion"]), str(row["total"]),
            f"{average}ms", str(row["errors"]),
        ))

    widths = [max(len(header[i]), *(len(r[i]) for r in rows)) for i in range(len(header))]
    line = "  ".join(h.ljust(widths[i]) for i, h in enumerate(header))
    print(line)
    print("-" * len(line))
    for row in rows:
        print("  ".join(cell.ljust(widths[i]) for i, cell in enumerate(row)))

    grand_total = sum(r["total"] for r in stats.values())
    print(f"\n总 token 用量：{grand_total}    总请求数：{len(records)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
