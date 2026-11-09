#!/usr/bin/env bash
# Usage: ./new-plugin.sh <name> [python|bash|node]  — scaffolds a script plugin and installs it locally.
set -euo pipefail
name=${1:?name}; lang=${2:-python}
dest="$HOME/.cloudslash/plugins/cloudslash-plugin-$name"; mkdir -p "$dest"
here="$(cd "$(dirname "$0")" && pwd)"
(cd "$here/runtime/script" && go build -o "$dest/cloudslash-plugin-$name" .)
printf "name: cloudslash-plugin-%s\nversion: 0.1.0\ndescription: My %s plugin\nprovider: %s\ncapabilities: [discovery]\n" "$name" "$name" "$name" > "$dest/manifest.yaml"
case $lang in
 python) printf "#!/usr/bin/env python3\nimport json\nprint(json.dumps({\"provider\":\"%s\",\"account\":\"me\",\"resources\":[{\"id\":\"example-1\",\"type\":\"disk\",\"service\":\"storage\",\"region\":\"us-east-1\",\"cost_monthly\":12.5}]}))\n" "$name" > "$dest/scan.py";;
 bash) printf "#!/usr/bin/env bash\necho \x27{\"provider\":\"%s\",\"account\":\"me\",\"resources\":[{\"id\":\"example-1\",\"type\":\"disk\",\"cost_monthly\":12.5}]}\x27\n" "$name" > "$dest/scan.sh";;
 node) printf "console.log(JSON.stringify({provider:\"%s\",account:\"me\",resources:[{id:\"example-1\",type:\"disk\",cost_monthly:12.5}]}))\n" "$name" > "$dest/scan.js";;
esac
echo "Created $dest — edit its scan script, then run: cloudslash scan --allow-unsigned"
