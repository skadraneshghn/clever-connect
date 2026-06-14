import json
import time
import requests
import websocket

def main():
    # Try logging in with salman / 136517 on port 8888
    login_url = "http://127.0.0.1:8888/api/auth/login"
    credentials = {"username": "salman", "password": "136517"}
    try:
        resp = requests.post(login_url, json=credentials)
    except Exception as e:
        print(f"Connection error: {e}")
        return

    if not resp.ok:
        print(f"Failed to log in: {resp.text}")
        return
    token = resp.json()["token"]

    ws_url = f"ws://127.0.0.1:8888/ws?token={token}"
    ws = websocket.create_connection(ws_url)

    dns_start_msg = {
        "type": "dns:start",
        "data": {
            "concurrency_limit": 50,
            "qps_limit": 0,
            "timeout_ms": 3000,
            "attempts": 2,
            "cache_busting": True,
            "reference_domain": "google.com",
            "selected_protocols": ["udp", "tcp"],
            "custom_resolvers": []
        }
    }

    print("Sending dns:start...")
    ws.send(json.dumps(dns_start_msg))

    start_time = time.time()
    finished = False

    while time.time() - start_time < 15 and not finished:
        msg_raw = ws.recv()
        msg = json.loads(msg_raw)
        print(f"\n[RAW MESSAGE] Type: {msg.get('type')}, Event: {msg.get('event')}")
        if msg.get("event") == "dns.finished":
            print(f"Finished stats: {msg.get('stats')}")
            finished = True
        if msg.get("event") == "dns.candidate":
            data = msg.get('data')
            print(f"Candidates count: {len(data) if isinstance(data, list) else 1}")
            print(f"Sample candidate: {data[0] if isinstance(data, list) and len(data) > 0 else data}")

    ws.close()

if __name__ == "__main__":
    main()
