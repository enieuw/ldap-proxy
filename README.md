# ldap-proxy

LDAP-proxy cobbled together to offload traffic going to a downstream LDAP server. This proxy will cache incoming requests using a hash of the content as the hash key. If a cache miss happens during the transmission the proxy will automatically replay all packages since opening the connection with the client to the downstream server.

This proxy has the following three env vars:

| Env var | Default |
| ----- | --- |
|LISTEN_INTERFACE| :389 |
|TARGET_SERVER | 127.0.0.1:389 |
|CACHE_DURATION_MINUTES | 15 |
