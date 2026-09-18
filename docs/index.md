---
layout: home

hero:
  name: glim
  text: HTML previews, one short link
  tagline: Publish a self-contained HTML file or directory from the command line — or straight from a coding agent — and get a short, readable link to open on any device.
  image:
    src: /icon.svg
    alt: glim
  actions:
    - theme: brand
      text: Get started
      link: /getting-started
    - theme: alt
      text: CLI reference
      link: /cli

features:
  - title: One command, one link
    details: glim report.html --title "Diff review" copies your page into a readable slug and prints its URL. Previews auto-expire; nothing to clean up.
  - title: Runs its own server
    details: No web server to configure. glim serves previews itself — a loopback link on your laptop, or a public URL when placed behind a reverse proxy.
  - title: Agents use it directly
    details: An MCP present tool plus a steering rule wires glim into Claude, Codex, and Cursor, so a coding agent shows you a live preview instead of pasting raw HTML.
---
