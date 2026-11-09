#!/usr/bin/env python3
import json, os, time
root = os.environ.get("CLOUDSLASH_ROOT", os.path.expanduser("~"))
cutoff = time.time() - 90 * 86400
res = []
for d, dirs, files in os.walk(root):
    dirs[:] = [x for x in dirs if not x.startswith(".")]
    for f in files:
        p = os.path.join(d, f)
        try:
            st = os.stat(p)
        except OSError:
            continue
        if st.st_size > 5_000_000 and st.st_mtime < cutoff:
            res.append({"id": p, "type": "file", "service": "storage", "region": "local",
                        "cost_monthly": round(st.st_size / 1e9 * 0.023, 4), "tags": {},
                        "properties": {"size_bytes": st.st_size}})
print(json.dumps({"provider": "local", "account": "localhost", "resources": res}))
