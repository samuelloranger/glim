# Getting started

glim publishes a self-contained HTML file or directory and gives you a short,
readable link to open in a browser. It runs its own preview server, so there is
nothing else to install.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/samuelloranger/glim/main/install.sh | sh
```

This drops the `glim` binary in `~/.local/bin`. Make sure that directory is on
your `PATH`. To build from source instead:

```sh
git clone https://github.com/samuelloranger/glim
cd glim && go build -o ~/.local/bin/glim .
```

## Your first preview

Start the server, then publish a page:

```sh
glim serve &
glim report.html --title "Diff review"
# → http://127.0.0.1:8787/diff-review-a1b2/
```

Open the printed link. That's it.

The link's readable part comes from `--title` (or the filename); the short random
suffix keeps names unique. Previews expire after their TTL (6h by default) and are
pruned automatically.

## A public link

Set a domain and place glim behind a reverse proxy, and the same command prints a
public URL:

```sh
glim config --domain https://glim.example.com
glim report.html --title "Diff review"
# → https://glim.example.com/diff-review-a1b2/
```

See [Serving & reverse proxy](/serving) for the proxy setup, and
[Configuration](/config) for all settings.

## One-off, no proxy

`--local` starts the built-in server automatically and returns a loopback link,
handy on a laptop with nothing else running:

```sh
glim report.html --title "Quick look" --local
```

## Use it from a coding agent

```sh
glim install claude    # or codex, cursor
```

This registers glim's MCP server and adds a steering rule so the agent shows you
previews through glim. See [Agent integration](/agents).
