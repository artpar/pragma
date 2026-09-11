#!/usr/bin/env python3
"""Local Brave-compatible Search API stub for the WEB-001 hermetic gate.

Serves GET /res/v1/web/search?q=<query>&count=<n> and returns one result
per query word with a STUB_BRAVE_RESULT marker, so the boot probe can verify
the tool path (headers, params, parsing, formatting) without spending the
real Brave quota. Requests are logged to requests.log.
"""
import json
import os
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

WORKDIR = sys.argv[1]
os.makedirs(WORKDIR, exist_ok=True)
lock = threading.Lock()

class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        pass

    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path != "/res/v1/web/search":
            self.send_response(404)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        params = parse_qs(parsed.query)
        query = (params.get("q") or [""])[0]
        token = self.headers.get("X-Subscription-Token", "")
        with lock:
            with open(os.path.join(WORKDIR, "requests.log"), "a") as f:
                f.write(json.dumps({
                    "q": query,
                    "count": (params.get("count") or [""])[0],
                    "token_present": bool(token),
                    "accept": self.headers.get("Accept", ""),
                }) + "\n")
        words = [w for w in query.split() if w][:5] or ["empty"]
        results = [
            {
                "title": "STUB_BRAVE_RESULT %d for %s" % (i + 1, words[0]),
                "url": "https://example.com/stub/%d" % (i + 1),
                "description": "Stubbed description %d about %s." % (i + 1, " ".join(words)),
            }
            for i in range(min(3, len(words) + 2))
        ]
        body = json.dumps({"web": {"results": results}}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

def main():
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    port = server.server_address[1]
    with open(os.path.join(WORKDIR, "port"), "w") as f:
        f.write(str(port))
    print("brave stub listening on 127.0.0.1:%d" % port, flush=True)
    server.serve_forever()

if __name__ == "__main__":
    main()
