#!/usr/bin/env python3
import sys, json

def send(obj):
    sys.stdout.write(json.dumps(obj) + "\n"); sys.stdout.flush()

for line in sys.stdin:
    line = line.strip()
    if not line: continue
    req = json.loads(line)
    method, rid = req.get("method"), req.get("id")
    if method == "initialize":
        send({"jsonrpc":"2.0","id":rid,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"mock","version":"1.0"}}})
    elif method == "notifications/initialized":
        pass  # notification, no reply
    elif method == "tools/list":
        send({"jsonrpc":"2.0","id":rid,"result":{"tools":[
            {"name":"sales","description":"weekly sales","inputSchema":{"type":"object"}}
        ]}})
    elif method == "tools/call":
        name = req["params"]["name"]
        if name == "sales":
            send({"jsonrpc":"2.0","id":rid,"result":{"content":[{"type":"text","text":"wk1 100\nwk2 140\nwk3 190"}],"isError":False}})
        else:
            send({"jsonrpc":"2.0","id":rid,"result":{"content":[{"type":"text","text":"unknown tool"}],"isError":True}})
    else:
        send({"jsonrpc":"2.0","id":rid,"error":{"code":-32601,"message":"method not found"}})
