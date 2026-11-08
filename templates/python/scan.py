#!/usr/bin/env python3
# Minimal CloudSlash script plugin: print discovered resources as JSON.
import json
print(json.dumps({"resources": [{"id": "demo-1", "type": "demo.disk", "cost_monthly": 12.5, "tags": {"orphaned": "true"}}]}))
