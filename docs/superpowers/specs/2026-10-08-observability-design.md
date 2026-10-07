# Platform observability — design

Date: 2026-10-08
Status: self-approved (user asleep, asked Claude to decide), open for review in the PR
Sub-project 4 of 5. Builds on core, releases and deploys.

## 1. Goal

Each service's environment tab answers "is it healthy, and what is it saying?"
without leaving the platform, by **embedding** Grafana rather than rebuilding it
(user decision, 2026-10-07): metrics from the existing Prometheus, logs in Loki,
traces in Tempo.

## 2. Pieces

| Piece | Owner | What |
|---|---|---|
| Loki | infra | single-binary Loki, filesystem storage, 7-day retention |
| Log shipping | infra | Grafana Alloy discovers Docker containers and ships their logs to Loki with `container`, `service` and `colour` labels |
| Grafana behind the platform | infra + platform | Grafana serves from `/grafana` on the platform host and trusts the platform's auth-proxy headers; `grafana.<domain>` redirects there |
| Dashboards as code | platform | one dashboard per service, uid `platform-<service>`, written through Grafana's HTTP API |
| Embedding | platform | the environment tab shows four metric panels and the logs panel for that environment |

### Deviation: Alloy instead of the otel-collector for logs

The agreed plan fed Loki from the existing otel-collector. Its `filelog` receiver
reads Docker's JSON files but only knows the container id from the path, not the
container's name or labels, so logs could not be told apart per service and
colour without extra receivers. Grafana Alloy's Docker discovery gives the name
and labels directly and is the standard way to ship Docker logs to Loki. The
otel-collector keeps doing traces and span metrics.

## 3. Single sign-on into Grafana

The control plane reverse-proxies `/grafana/*` to Grafana for signed-in users. It
removes any `X-WEBAUTH-*` header the browser sent and sets:

- `X-WEBAUTH-USER`: the GitHub login,
- `X-WEBAUTH-NAME`: the display name,
- `X-WEBAUTH-ROLE`: `Admin` for platform admins, `Viewer` for everyone else.

Grafana runs with `auth.proxy` enabled for those headers, `serve_from_sub_path`,
`root_url = https://platform.<domain>/grafana/` and `allow_embedding`. Grafana's
port is bound to 127.0.0.1 and its old public route becomes a redirect, so the only
way to reach it with headers is the control plane; Traefik also strips
`X-WEBAUTH-*` on every router. The proxy is guarded by the session but not by the
platform's CSRF token: Grafana's own requests could not carry it, and the
`SameSite=Lax` session cookie is not sent on cross-site POSTs.

deployctl's Grafana annotations move to `http://127.0.0.1:3000/grafana`.

## 4. Dashboards as code

`app/observe` builds a dashboard per service: requests per second, 5xx ratio and
p95 latency from Traefik (`service=~"<name>-(blue|green)@file"`), memory and CPU
from cAdvisor (`name=~"<name>-(blue|green)"`), and a logs panel from Loki
(`{container=~"<name>-(blue|green)"}`), where `<name>` is `<service>-${env}` and
`env` is a dashboard variable. It upserts them into a `Platform` folder:

- at start-up, for every service,
- on `service.created` and `service.updated` events.

Writing a dashboard is idempotent (`overwrite: true`); a failure is logged and the
next event or restart retries. The platform's Grafana token is a service-account
token with the Editor role.

## 5. Embedding

The environment tab gets an **Observability** card with `d-solo` iframes of the
four metric panels and the logs panel for that environment, in the page's theme
(both themes are rendered; CSS shows the matching one, and hidden iframes are lazy
so they never load), plus links to the full dashboard and to Explore.

Without `PLATFORM_GRAFANA_URL` the card explains that observability is not
connected, and nothing is proxied or written.

## 6. Configuration

| Key | Meaning |
|---|---|
| `PLATFORM_GRAFANA_URL` | Grafana inside the network, `http://grafana:3000/grafana` |
| `PLATFORM_GRAFANA_TOKEN` | service-account token for dashboard writes |

## 7. Testing

- Dashboard builder: queries carry the workload names and the env variable;
  datasource uids are `prometheus` and `loki`.
- Grafana client against a fake Grafana: folder ensured once, dashboards upserted
  with the token.
- Proxy: anonymous visitors go to /login; spoofed `X-WEBAUTH-USER` from the browser
  is replaced; admins get `Admin`, others `Viewer`; paths keep the `/grafana` prefix.
- Integration: creating a service writes its dashboard to the fake Grafana; the
  environment tab embeds the panels.
