# The host agent

## The agent

`cmd/agent` runs on each host, built into `adrianozdp/platform-agent`. It needs the
Docker socket, the deployctl root and host networking (deployctl talks to
Prometheus and Traefik on 127.0.0.1):

```
docker run -d --name platform-agent --network host --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock -v /opt/plateful:/opt/plateful \
  -e AGENT_PLATFORM_URL=https://platform.kalz.one -e AGENT_TOKEN=... \
  adrianozdp/platform-agent:<sha>
```

| Variable | Default | Meaning |
|---|---|---|
| `AGENT_PLATFORM_URL` | | control plane URL |
| `AGENT_TOKEN` | | the control plane's `PLATFORM_AGENT_TOKEN` |
| `AGENT_NAME` | host name | how the agent shows on the Agents page |
| `AGENT_ROOT` | `/opt/plateful` | deployctl root |
| `AGENT_DEPLOYCTL` | `/opt/plateful/bin/deployctl` | deployctl binary |
| `AGENT_POLL` | `5s` | how often it asks for work |
| `AGENT_DRIFT_EVERY` | `60s` | how often it compares what runs with what should |

Each service runs as the deployctl service `<service>-<environment>`. A service
with a public host gets a `Host()` rule on Traefik's TLS entrypoint; one without
runs as a worker and needs a command. Replicas and CPU limits are not applied by
this runtime yet.
