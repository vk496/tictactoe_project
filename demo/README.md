# Demo (not part of the case-study review)

This directory holds things built *around* the backend to make it pleasant to
show — a browser client and an architecture deck. None of it is required to
build, run, or review the backend itself (that's the repo root).

- **`web/`** — a dependency-free static client (vanilla JS over the Connect JSON
  protocol). Lets you play across two browser tabs.
- **`slides/`** — a [Slidev](https://sli.dev) deck (Markdown + Mermaid) explaining
  the architecture.
- **`traefik/`** — the edge router's config (`dynamic.yml`). A [Traefik](https://traefik.io)
  v3 proxy on port 80 puts everything on one origin, which is what removes CORS.

## Run it

The root `docker compose up` already includes all of this. The game is behind the
Traefik edge on port 8000; the slides and the two dashboards each get their own
port:

| URL                              | What                                    |
|----------------------------------|-----------------------------------------|
| http://localhost:8000            | the game UI (and, same-origin, the API) |
| http://localhost:8100            | the architecture deck                   |
| http://localhost:8999            | the RabbitMQ dashboard (`admin` / `admin`)|
| http://localhost:8090/dashboard/ | the Traefik dashboard                   |

Open http://localhost:8000 in **two tabs**, pick two names, set a board size, and
play. The worker route the client follows is relative, so everything stays on the
page's origin and there is no CORS. The **Dashboards** panel links to the slides,
the API reference, RabbitMQ and Traefik.

`docs.html` renders the OpenAPI spec (generated from the protos and served by the
gateway at `/openapi.yaml`) with [Scalar](https://github.com/scalar/scalar). The
Scalar viewer script loads from a CDN — the only external runtime dependency in the
demo; the spec itself is self-contained.

## Work on the slides locally

```sh
cd demo/slides && npm install && npm run dev   # http://localhost:3030
```

The deck is served at an origin root (its own port in Compose, `--base /`). Slidev's
client router prepends the base to every in-deck path, so a `/slides/`-style subpath
breaks navigation — hence the dedicated port rather than a path behind the edge.
