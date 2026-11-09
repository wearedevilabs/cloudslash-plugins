# CloudSlash Plugins
Write a plugin in any language in 5 minutes:

    ./new-plugin.sh mycloud python     # scaffolds + installs ~/.cloudslash/plugins/cloudslash-plugin-mycloud
    # edit scan.py: print {"provider","account","resources":[{id,type,service,region,cost_monthly,tags,properties}]}
    cloudslash scan --allow-unsigned

The script runtime (runtime/script) runs scan.py / scan.sh / scan.js / scan next to it, passes scan config as CLOUDSLASH_<KEY> env vars, and converts JSON output into CloudSlash resources. See examples/stale-files for a real working scanner. Compiled Go plugins live in plugins/.
