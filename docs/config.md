# Configuration

glim reads `~/.glim/config.json`. Set it from the command line:

```sh
glim config --domain https://glim.example.com --bind 0.0.0.0 --port 8787
glim config                     # show the resolved config
```

## Keys

| key | default | meaning |
| --- | --- | --- |
| `domain` | _(none)_ | Public base URL for links. Unset → links use `http://127.0.0.1:PORT`. |
| `bind` | `127.0.0.1` | Address the server binds. Use `0.0.0.0` behind a reverse proxy. |
| `port` | `8787` | Server port. |
| `root` | `~/.glim/pub` | Where previews are stored and served from. |
| `ttl` | `6h` | Default lifetime for a preview. |
| `live_reload` | `true` | Refresh open preview tabs when the preview is republished under the same name. `false` stops glim injecting the reload script and serving `/_glim/live/<slug>`. Set with `glim config --live-reload=false`. |
| `header` | `true` | Show the owner-only glim bar on preview pages you open while signed in to the dashboard. `false` stops glim injecting it anywhere. Set with `glim config --header=false`. See [Dashboard](/dashboard#the-glim-bar). |
| `self_fetch` | `true` | Let a preview page `fetch`/XHR its own files by answering `Origin: null` with CORS headers. `false` sends no CORS headers and no preflight answer. Set with `glim config --self-fetch=false`. See [Serving](/serving). |

## Environment overrides

Every key can be overridden by an environment variable, which wins over the file:

`GLIM_DOMAIN`, `GLIM_BIND`, `GLIM_PORT`, `GLIM_ROOT`, `GLIM_TTL`, `GLIM_LIVE_RELOAD`, `GLIM_SELF_FETCH`, `GLIM_HEADER`.

`GLIM_SESSION_ID`, if set, is recorded on each preview as metadata.

## Listen

Listen makes an audio version of a preview on demand, from the glim bar or the
dashboard: a vision model looks at the page (its text, screenshots of it and
close-ups of its pictures) and writes a summary and a script that describes
everything on it, design included, and a speech endpoint reads the script
aloud. It is off until a model is configured.

Both endpoints speak the OpenAI API shape, so any hosted provider or local
server that does works: glim calls `<llm_url>/chat/completions` and
`<tts_url>/audio/speech`.

```sh
glim config --llm-url https://api.example.com/v1 --llm-model some-vision-model --llm-max-tokens 4096
glim config --tts-url http://127.0.0.1:8880/v1 --tts-model some-tts-model \
  --tts-voice voice-a --tts-voices fr=voice-b,es=voice-c
```

| key | default | meaning |
| --- | --- | --- |
| `listen.llm_url` | _(none)_ | Base URL of an OpenAI-compatible API, ending where `/chat/completions` goes (usually `/v1`). Listen is off without it. |
| `listen.llm_model` | _(none)_ | Model that describes the page. Use one that accepts images; a text-only model still works from the page text. |
| `listen.llm_max_tokens` | `4096` | The model's output limit, sent as `max_completion_tokens` (or `max_tokens` if the provider refuses that). The narration is a condensed reading of about a third of the page, and its word budget follows from this limit (at most 1500 words). Keep it within your model's limit; a reply that still hits it is kept up to its last full sentence and marked as cut off. |
| `listen.tts_url` | _(none)_ | Base URL of an OpenAI-compatible speech API. Without it glim writes the narration only, and the bar and the dashboard offer **Read aloud** with the device's own voice. |
| `listen.tts_model` | _(none)_ | Speech model. |
| `listen.tts_voice` | _(none)_ | Default voice. Omitted from the request when unset. |
| `listen.tts_voices` | _(none)_ | Voice per language, keyed by the part of the page's language code before any `-` (`fr` covers `fr-CA`). `--tts-voices -` clears it. |
| `listen.chrome` | _(search PATH)_ | Browser used to render the page for screenshots: a path or a command name. `off` never uses one. Without a browser, glim reads the page's HTML and its image files instead (scripts don't run). |
| `listen.instructions` | _(none)_ | Extra instructions appended to the prompt, such as `Keep it under five minutes.` |

API keys are never stored in the config file. Set `GLIM_LLM_API_KEY` and
`GLIM_TTS_API_KEY` in the environment of `glim serve` (for the systemd unit,
an `Environment=` line or an `EnvironmentFile=` readable only by you). Every
other key can also be set from the environment: `GLIM_LLM_URL`,
`GLIM_LLM_MODEL`, `GLIM_LLM_MAX_TOKENS`, `GLIM_TTS_URL`, `GLIM_TTS_MODEL`, `GLIM_TTS_VOICE`,
`GLIM_CHROME` and `GLIM_LISTEN_INSTRUCTIONS`.

`glim config` shows whether Listen is on, which endpoints it uses, whether each
key is set and which browser it found. `glim serve` logs the same at startup.

What gets sent: the page's visible text (up to 60 KB), up to five pictures
(screenshots from top to bottom, then close-ups of its largest images, charts
and canvases) and the page title. Locked previews are sent too: only you can
start a Listen, and it reads the files from disk.

A long script is read in chunks of at most 4000 characters, split at paragraph
and sentence ends, and joined into one MP3 with a header giving its true
length, so players show the right duration and seek correctly. One audio
version is made at a time; others wait their turn. The result lives in a
hidden folder inside the preview and goes away when the preview expires, is
removed or is republished (a republish marks the old one as changed, with an
**Update** button).

## How links are built

- With a `domain`, a link is `https://domain/<slug>-<id>/`.
- Without one, it is `http://127.0.0.1:<port>/<slug>-<id>/`.

The `<slug>` comes from the title or filename; the short `<id>` suffix keeps names
unique. The MCP `present` tool returns the same URL the CLI prints, so an agent
always hands you a link that resolves.

## Lifetime and cleanup

Each preview stores its expiry. Expired previews are removed on the next `glim`
command and by `glim gc`. Run `gc` from a periodic timer if the server is
long-lived and rarely invoked.
