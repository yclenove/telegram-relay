#!/usr/bin/env python3
"""
轻量压测脚本：并发调用 /api/v2/notify 验证入站吞吐与排队稳定性。
"""
import concurrent.futures
import json
import os
import time
import urllib.request


def send_once(idx: int) -> int:
    url = os.environ["RELAY_URL"]
    token = os.environ["AUTH_TOKEN"]
    payload = {
        "title": f"load-test-{idx}",
        "message": "stress test message",
        "level": "warning",
        "source": "loadtest",
        "event_id": f"evt-load-{int(time.time() * 1000)}-{idx}",
        "labels": {"batch": "loadtest"},
    }
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        method="POST",
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
        },
    )
    with urllib.request.urlopen(req, timeout=8) as resp:
        return resp.status


def main():
    workers = int(os.getenv("WORKERS", "20"))
    total = int(os.getenv("TOTAL", "200"))
    success = 0
    start = time.time()
    with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
        futures = [pool.submit(send_once, i) for i in range(total)]
        for fut in concurrent.futures.as_completed(futures):
            try:
                if fut.result() == 200:
                    success += 1
            except Exception:
                pass
    elapsed = time.time() - start
    print(f"total={total} success={success} failed={total - success} elapsed={elapsed:.2f}s qps={total/elapsed:.2f}")


if __name__ == "__main__":
    main()
