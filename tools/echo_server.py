"""本地回显服务器：把收到的请求头与请求体原样吐回。

用途：验证 llm-api-enhance 的 request_headers.set 是否真的注入了
期望的头（尤其是 ${session_id} 是否按对话稳定）。

    python tools/echo_server.py 8899
"""

import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer


class EchoHandler(BaseHTTPRequestHandler):
    def _handle(self):
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b""
        try:
            body = json.loads(raw.decode("utf-8")) if raw else None
        except Exception:
            body = raw.decode("utf-8", "replace")

        # HTTP 头名大小写不敏感，这里统一转小写，避免校验脚本误判。
        # 同时附带一个 usage 块，使响应形似 OpenAI 返回，
        # 从而能一并验证代理的 token 用量提取链路。
        payload = {
            "received_headers": {k.lower(): v for k, v in self.headers.items()},
            "received_body": body,
            "usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
        }
        data = json.dumps(payload, ensure_ascii=False, indent=2).encode("utf-8")

        self.send_response(200)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    do_POST = _handle
    do_GET = _handle

    def log_message(self, fmt, *args):  # 静音默认日志
        sys.stderr.write("[echo] " + (fmt % args) + "\n")


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8899
    print(f"echo server listening on 127.0.0.1:{port}", flush=True)
    HTTPServer(("127.0.0.1", port), EchoHandler).serve_forever()
