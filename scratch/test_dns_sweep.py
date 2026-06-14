import json
import time
import requests
import websocket

def main():
    login_url = "http://localhost:8080/api/auth/login"
    credentials = {"username": "salman", "password": "136517"}
    resp = requests.post(login_url, json=credentials)
    if not resp.ok:
      print(f"Failed to log in: {resp.text}")
      return
    token = resp.json()["token"]

    ws_url = f"ws://localhost:8080/ws?token={token}"
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
            print(f"Candidates details: {msg.get('data')}")

    ws.close()

if __name__ == "__main__":
    main()
