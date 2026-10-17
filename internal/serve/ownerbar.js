// The owner-only glim bar. Wrapped by ownerbar.go as function(D){...}(data);
// D is {title, url, dash, exp, locked, qr?: {size, path}, api?, tok?, listen?};
// api and tok, present when comments are on, are the comment endpoint and the
// token sent to it (the sandboxed page sends no cookies); listen, present
// when the server makes audio versions, is the Listen endpoint, which takes
// the same token. It renders a fixed, closed shadow DOM
// overlay, never touches page layout, and swallows every error. On narrow
// screens the bar docks to the bottom edge and pads itself by the safe-area
// insets; on wider ones it floats as a rounded bar.
try {
  var doc = document, root = doc.documentElement;
  if (!root || window.__glimBar) return;
  window.__glimBar = true;
  // A random tag name: a page can neither already contain it nor register it as
  // its own custom element, either of which would hide or break the bar.
  var host = doc.createElement("glim-bar-" + Math.random().toString(36).slice(2, 10));
  host.setAttribute("data-glim-bar", "");
  // Inline !important beats any !important rule the page's own CSS sets on
  // elements, so a hostile stylesheet cannot hide or move the bar.
  var force = {
    all: "initial", display: "block", position: "fixed", left: "0", right: "0", bottom: "0",
    "z-index": "2147483647", "pointer-events": "none", margin: "0", padding: "0",
    border: "0", opacity: "1", visibility: "visible", transform: "none", filter: "none",
    width: "auto", height: "auto"
  };
  for (var k in force) host.style.setProperty(k, force[k], "important");
  var sh = host.attachShadow({ mode: "closed" });
  // Interaction with the bar is not the page's business: a deck that turns
  // the slide on any click, or a page with keyboard shortcuts, must not react
  // to the bar's buttons or to typing a comment.
  var mine = ["click", "dblclick", "auxclick", "pointerdown", "pointerup", "mousedown", "mouseup",
    "touchstart", "touchend", "keydown", "keyup", "keypress", "input"];
  for (var mi = 0; mi < mine.length; mi++) host.addEventListener(mine[mi], function (e) { e.stopPropagation(); });
  // iOS reports safe-area insets only to pages whose viewport tag opts in with
  // viewport-fit=cover, so add it to an existing tag; without it the docked
  // bar would sit under the home indicator.
  var vp = doc.querySelector('meta[name="viewport"]');
  if (vp && !/viewport-fit/i.test(vp.content)) {
    vp.content = vp.content ? vp.content + ", viewport-fit=cover" : "viewport-fit=cover";
  }
  var css =
    ":host{all:initial}" +
    "*{box-sizing:border-box}" +
    ".w{position:relative;display:flex;justify-content:center;pointer-events:none;" +
    "padding:0 8px calc(8px + env(safe-area-inset-bottom,0px));" +
    "font:13px/1 -apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;" +
    "--bg:rgba(255,255,255,.94);--sol:#fff;--fg:#1c1b19;--mut:#6b675f;--bd:rgba(0,0,0,.14);--hv:rgba(0,0,0,.07);--ac:#c2410c;--mk:#6366f1}" +
    "@media(prefers-color-scheme:dark){.w{--bg:rgba(28,27,25,.94);--sol:#1c1b19;--fg:#f1efe9;--mut:#a8a398;--bd:rgba(255,255,255,.18);--hv:rgba(255,255,255,.1);--ac:#fb923c;--mk:#818cf8}}" +
    ".bar,.pill{pointer-events:auto;background:var(--bg);color:var(--fg);border:1px solid var(--bd);" +
    "box-shadow:0 2px 12px rgba(0,0,0,.22);-webkit-backdrop-filter:blur(8px);backdrop-filter:blur(8px)}" +
    ".bar{display:flex;align-items:center;gap:4px;width:100%;max-width:640px;height:36px;padding:0 4px 0 10px;border-radius:18px}" +
    ".w.c{justify-content:flex-end}.w.c .bar,.w:not(.c) .pill{display:none}" +
    ".mark{flex:none;display:flex;color:var(--mk)}" +
    ".t{flex:1 1 0;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-weight:600;margin-left:2px}" +
    ".m{flex:none;display:flex;align-items:center;gap:4px;color:var(--mut);font-size:12px;white-space:nowrap}" +
    "button,a{all:unset;box-sizing:border-box;flex:none;display:flex;align-items:center;gap:5px;height:28px;min-width:28px;" +
    "padding:0 7px;border-radius:14px;cursor:pointer;color:var(--fg);font:inherit;font-size:12px;justify-content:center}" +
    "button:hover,a:hover{background:var(--hv)}" +
    "button:focus-visible,a:focus-visible{outline:2px solid var(--ac);outline-offset:-2px}" +
    ".lb{display:none}@media(min-width:520px){.lb{display:inline}}" +
    ".ok .lb{display:inline;color:var(--ac)}" +
    ".pill{display:flex;align-items:center;gap:6px;height:32px;padding:0 12px 0 10px;border-radius:16px;font-weight:600}" +
    ".man{pointer-events:auto;position:absolute;left:8px;right:8px;bottom:100%;" +
    "max-width:640px;margin:0 auto;padding:6px;border-radius:10px;background:var(--bg);border:1px solid var(--bd);" +
    "box-shadow:0 2px 12px rgba(0,0,0,.22)}" +
    ".sp{display:flex;flex-direction:column;align-items:center;gap:8px;padding:10px}" +
    ".qr{display:block;width:176px;height:176px;border-radius:6px}" +
    ".sp .u{max-width:100%;color:var(--mut);font:12px/1.4 monospace;overflow-wrap:anywhere;text-align:center;" +
    "user-select:text;-webkit-user-select:text}" +
    ".sp button{border:1px solid var(--bd)}" +
    // Comments: the badge, the pins on the page, the element highlight while
    // picking, and the comment list and composer panels.
    ".bd{min-width:16px;height:16px;padding:0 4px;border-radius:8px;background:var(--mk);color:#fff;" +
    "font-size:10px;font-weight:700;display:inline-flex;align-items:center;justify-content:center}" +
    ".bd[hidden]{display:none}" +
    ".pins{position:fixed;left:0;top:0;width:0;height:0;pointer-events:none;--mk:#6366f1;--ac:#c2410c}" +
    "@media(prefers-color-scheme:dark){.pins{--mk:#818cf8;--ac:#fb923c}}" +
    ".pins.off{display:none}" +
    ".pin{all:unset;box-sizing:border-box;position:fixed;display:flex;align-items:center;justify-content:center;" +
    "width:22px;height:22px;margin:-22px 0 0 0;border-radius:11px 11px 11px 2px;background:var(--mk);color:#fff;" +
    "font:700 11px/1 -apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;" +
    "box-shadow:0 1px 4px rgba(0,0,0,.35);pointer-events:auto;cursor:pointer}" +
    ".pin:focus-visible{outline:2px solid var(--ac);outline-offset:1px}" +
    ".pin.new{background:var(--ac)}" +
    ".hl{position:fixed;border:2px solid var(--mk);background:rgba(99,102,241,.12);border-radius:3px;pointer-events:none}" +
    ".cp{display:flex;flex-direction:column;gap:8px;max-height:min(60vh,480px);overflow:auto;padding:10px 12px;" +
    "font-size:13px;line-height:1.4;color:var(--fg)}" +
    ".hd{display:flex;align-items:center;gap:8px;font-weight:600}.hd .sp1{flex:1}" +
    ".cp button{border:1px solid var(--bd);height:26px}" +
    ".cp button.pr{background:var(--mk);border-color:var(--mk);color:#fff}" +
    ".cp button[disabled]{opacity:.55;cursor:default}" +
    ".it{display:flex;flex-direction:column;gap:4px;padding:8px 0;border-top:1px solid var(--bd)}" +
    ".it.r{opacity:.62}.it.on{background:var(--hv);margin:0 -12px;padding:8px 12px}" +
    ".ih{display:flex;align-items:center;gap:6px;color:var(--mut);font-size:12px;min-width:0}" +
    ".ih .q{flex:1 1 0;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}" +
    ".ih .n{flex:none;width:18px;height:18px;border-radius:9px 9px 9px 2px;background:var(--mk);color:#fff;" +
    "font-size:10px;font-weight:700;display:flex;align-items:center;justify-content:center}" +
    ".tx{white-space:pre-wrap;overflow-wrap:anywhere;user-select:text;-webkit-user-select:text}" +
    ".rp{border-left:3px solid var(--mk);padding:2px 0 2px 8px;white-space:pre-wrap;overflow-wrap:anywhere;" +
    "user-select:text;-webkit-user-select:text}" +
    ".rp b{display:block;color:var(--mut);font-size:11px;font-weight:600}" +
    ".mt{color:var(--mut);font-size:11px}" +
    ".ac{display:flex;gap:6px;flex-wrap:wrap}" +
    ".er{color:var(--ac);font-size:12px}" +
    ".mu{color:var(--mut);font-size:12px}" +
    ".cp textarea{all:unset;box-sizing:border-box;display:block;width:100%;min-height:72px;padding:8px;" +
    "border:1px solid var(--bd);border-radius:8px;color:var(--fg);font:13px/1.4 inherit;white-space:pre-wrap;" +
    "user-select:text;-webkit-user-select:text;cursor:text}" +
    ".cp textarea:focus-visible{outline:2px solid var(--ac);outline-offset:-1px}" +
    // Listen: the player, the summary and the transcript.
    // Solid, unlike the other panels: long text over the page's own is hard to read.
    ".man.ls{background:var(--sol)}" +
    ".cp audio{display:block;width:100%;height:36px}" +
    ".sum{white-space:pre-wrap;overflow-wrap:anywhere;user-select:text;-webkit-user-select:text}" +
    ".cp details{border-top:1px solid var(--bd);padding-top:6px}" +
    ".cp summary{cursor:pointer;color:var(--mut);font-size:12px;padding:2px 0}" +
    // No scroll of its own: the panel scrolls, and one scroll is enough.
    ".trn{margin-top:6px;white-space:pre-wrap;overflow-wrap:anywhere;" +
    "color:var(--mut);font-size:12px;user-select:text;-webkit-user-select:text}" +
    ".rt button{min-width:40px}.rt button.on{background:var(--hv);font-weight:700}" +
    ".man input{all:unset;box-sizing:border-box;display:block;width:100%;padding:6px 8px;color:var(--fg);font:12px monospace;" +
    "user-select:text;-webkit-user-select:text}" +
    // Narrow screens: a flat strip flush with the bottom edge, padded by the
    // safe-area insets so it fills the home-indicator area instead of
    // floating over it.
    "@media(max-width:640px){" +
    ".w{padding:0}" +
    ".w.c{padding:0 calc(8px + env(safe-area-inset-right,0px)) calc(8px + env(safe-area-inset-bottom,0px)) 0}" +
    ".bar{max-width:none;height:calc(40px + env(safe-area-inset-bottom,0px));border-radius:0;border-width:1px 0 0;" +
    "box-shadow:0 -1px 8px rgba(0,0,0,.14);" +
    "padding:0 max(2px,env(safe-area-inset-right,0px)) env(safe-area-inset-bottom,0px) max(12px,env(safe-area-inset-left,0px))}" +
    ".man{left:0;right:0;border-radius:0;border-width:1px 0 0}}";
  var st = doc.createElement("style");
  st.textContent = css;
  sh.appendChild(st);

  var NS = "http://www.w3.org/2000/svg";
  function icon(paths, size) {
    var s = doc.createElementNS(NS, "svg");
    s.setAttribute("viewBox", "0 0 24 24");
    s.setAttribute("width", size || 15);
    s.setAttribute("height", size || 15);
    s.setAttribute("fill", "none");
    s.setAttribute("stroke", "currentColor");
    s.setAttribute("stroke-width", "2");
    s.setAttribute("stroke-linecap", "round");
    s.setAttribute("stroke-linejoin", "round");
    s.setAttribute("aria-hidden", "true");
    for (var i = 0; i < paths.length; i++) {
      var p = doc.createElementNS(NS, "path");
      p.setAttribute("d", paths[i]);
      s.appendChild(p);
    }
    return s;
  }
  function el(tag, cls, text) {
    var e = doc.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  // The glim eye from the app icon, in the bar's accent colour.
  function mark(size) {
    var s = icon(["M12 7.5c-3.4 0-6.2 2.2-7.1 4.5.9 2.3 3.7 4.5 7.1 4.5s6.2-2.2 7.1-4.5c-.9-2.3-3.7-4.5-7.1-4.5Z"], size);
    var c = doc.createElementNS(NS, "circle");
    c.setAttribute("cx", "12");
    c.setAttribute("cy", "12");
    c.setAttribute("r", "2.4");
    c.setAttribute("fill", "currentColor");
    s.appendChild(c);
    return s;
  }
  function btn(tag, label) {
    var b = el(tag);
    b.setAttribute("aria-label", label);
    b.title = label;
    return b;
  }

  var w = el("div", "w");
  var bar = el("div", "bar");
  var mk = el("span", "mark");
  mk.appendChild(mark(16));
  var title = el("span", "t", D.title);
  title.title = D.title;
  var meta = el("span", "m");
  if (D.locked) {
    var lk = el("span");
    lk.title = "Password-protected";
    lk.setAttribute("aria-label", "Password-protected");
    lk.appendChild(icon(["M5 11h14v10H5z", "M8 11V7a4 4 0 0 1 8 0v4"], 13));
    lk.style.display = "flex";
    meta.appendChild(lk);
  }
  var left = el("span");
  meta.appendChild(left);
  function fmt() {
    if (!D.exp) return "Pinned";
    var s = Math.floor((D.exp - Date.now()) / 1000);
    if (s <= 0) return "Expired";
    var d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
    if (d > 0) return d + "d " + h + "h left";
    if (h > 0) return h + "h " + m + "m left";
    if (m > 0) return m + "m left";
    return "<1m left";
  }
  function tick() { left.textContent = fmt(); }
  tick();
  if (D.exp) setInterval(tick, 15000);

  var copy = btn("button", "Copy link");
  copy.appendChild(icon(["M9 9h11v11H9z", "M5 15V5h10"], 15));
  var cl = el("span", "lb", "Copy link");
  copy.appendChild(cl);
  var share = btn("button", "Share link");
  share.setAttribute("aria-expanded", "false");
  share.appendChild(icon(["M4 12v7a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-7", "M16 6l-4-4-4 4", "M12 2v13"], 15));
  share.appendChild(el("span", "lb", "Share"));
  var cmt = null, badge = null;
  if (D.api && D.tok) {
    cmt = btn("button", "Comments");
    cmt.setAttribute("aria-expanded", "false");
    cmt.appendChild(icon(["M4 5h16v11H9l-5 4z"], 15));
    cmt.appendChild(el("span", "lb", "Comment"));
    badge = el("span", "bd");
    badge.hidden = true;
    cmt.appendChild(badge);
  }
  var lsn = null;
  if (D.listen && D.tok) {
    lsn = btn("button", "Listen");
    lsn.setAttribute("aria-expanded", "false");
    lsn.appendChild(icon(["M4 14v-2a8 8 0 0 1 16 0v2", "M4 14h3v6H5a1 1 0 0 1-1-1z", "M20 14h-3v6h2a1 1 0 0 0 1-1z"], 15));
    lsn.appendChild(el("span", "lb", "Listen"));
  }
  var dash = btn("a", "Back to dashboard");
  dash.href = D.dash;
  dash.target = "_self";
  dash.appendChild(icon(["M3 12l9-8 9 8", "M5 10v10h14V10"], 15));
  dash.appendChild(el("span", "lb", "Dashboard"));
  var col = btn("button", "Collapse glim bar");
  col.appendChild(icon(["M6 9l6 6 6-6"], 15));
  bar.appendChild(mk);
  bar.appendChild(title);
  bar.appendChild(meta);
  bar.appendChild(copy);
  bar.appendChild(share);
  if (cmt) bar.appendChild(cmt);
  if (lsn) bar.appendChild(lsn);
  bar.appendChild(dash);
  bar.appendChild(col);

  var pill = btn("button", "Show glim bar");
  pill.className = "pill";
  pill.appendChild(mark(14));
  pill.appendChild(el("span", "", "glim"));

  // One panel above the bar at a time: the hand-copy link, the share panel or
  // a comments panel. Closing it also ends comment picking.
  var man = null, opener = null;
  function closeManual() {
    if (lsn && player.parentNode && player.parentNode !== hold) hold.appendChild(player);
    if (man) { man.remove(); man = null; }
    share.setAttribute("aria-expanded", "false");
    if (cmt) cmt.setAttribute("aria-expanded", "false");
    if (lsn) lsn.setAttribute("aria-expanded", "false");
    stopListenPoll();
    stopPick();
    showHL(null);
    draft = null;
    placePins();
  }
  function manual() {
    closeManual();
    man = el("div", "man");
    var inp = doc.createElement("input");
    inp.readOnly = true;
    inp.value = D.url;
    inp.setAttribute("aria-label", "Preview link");
    man.appendChild(inp);
    w.appendChild(man);
    inp.focus();
    inp.select();
    return inp;
  }
  var timer;
  function done(ok) {
    clearTimeout(timer);
    if (!ok) { manual(); return; }
    closeManual();
    cl.textContent = "Copied";
    copy.className = "ok";
    timer = setTimeout(function () { cl.textContent = "Copy link"; copy.className = ""; }, 1500);
  }
  function legacy() {
    var ta = doc.createElement("textarea");
    ta.value = D.url;
    ta.setAttribute("readonly", "");
    ta.style.cssText = "position:absolute;left:0;bottom:0;width:1px;height:1px;opacity:0";
    sh.appendChild(ta);
    var ok = false;
    try { ta.select(); ta.setSelectionRange(0, ta.value.length); ok = doc.execCommand("copy"); } catch (e) {}
    ta.remove();
    return ok;
  }
  copy.addEventListener("click", function () {
    var tried = false;
    function fallback() { if (tried) return; tried = true; done(legacy()); }
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(D.url).then(function () { tried = true; done(true); }, fallback);
        return;
      }
    } catch (e) {}
    fallback();
  });

  // The share panel: the link as a QR code to scan from another device, and
  // the system share sheet where the browser offers one.
  function qrSVG() {
    var q = D.qr, s = doc.createElementNS(NS, "svg");
    s.setAttribute("class", "qr");
    s.setAttribute("viewBox", "0 0 " + q.size + " " + q.size);
    s.setAttribute("shape-rendering", "crispEdges");
    s.setAttribute("role", "img");
    s.setAttribute("aria-label", "QR code of the link");
    var bg = doc.createElementNS(NS, "rect");
    bg.setAttribute("width", q.size);
    bg.setAttribute("height", q.size);
    bg.setAttribute("fill", "#fff");
    var p = doc.createElementNS(NS, "path");
    p.setAttribute("d", q.path);
    p.setAttribute("fill", "#000");
    s.appendChild(bg);
    s.appendChild(p);
    return s;
  }
  function openShare() {
    closeManual();
    man = el("div", "man sp");
    man.setAttribute("role", "group");
    man.setAttribute("aria-label", "Share link");
    if (D.qr) man.appendChild(qrSVG());
    man.appendChild(el("span", "u", D.url));
    if (navigator.share) {
      var native = el("button", "", "Share…");
      native.addEventListener("click", function () {
        try {
          navigator.share({ title: D.title, url: D.url }).then(closeManual, function () {});
        } catch (e) {}
      });
      man.appendChild(native);
    }
    w.appendChild(man);
    share.setAttribute("aria-expanded", "true");
    opener = share;
  }
  share.addEventListener("click", function () {
    if (share.getAttribute("aria-expanded") === "true") closeManual();
    else openShare();
  });
  host.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && man) {
      closeManual();
      if (opener) opener.focus();
    }
  });

  // --- comments ---
  // The owner leaves comments on the page from here; an agent reads them
  // over MCP (the comments tool) and answers with resolve_comment. Open
  // comments pinned to an element show as numbered pins on the page.
  var comments = [], loadErr = "", picking = false, draft = null, showResolved = false, focusId = 0;
  var pins = el("div", "pins"), hl = el("div", "hl");
  hl.hidden = true;
  pins.appendChild(hl);

  function call(method, path, body) { return request(D.api + path, method, body); }
  function request(url, method, body) {
    var opts = { method: method, headers: { "X-Glim-Bar": D.tok }, credentials: "omit", mode: "cors", cache: "no-store" };
    if (body) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }
    return fetch(url, opts).then(function (r) {
      if (r.status === 204) return null;
      return r.json().then(function (j) {
        if (!r.ok) throw new Error((j && j.error) || "Request failed (" + r.status + ").");
        return j;
      }, function () { throw new Error("Request failed (" + r.status + ")."); });
    }, function () { throw new Error("Can't reach glim. Check your connection."); });
  }
  function unresolved() {
    var out = [];
    for (var i = 0; i < comments.length; i++) if (!comments[i].resolved) out.push(comments[i]);
    return out;
  }
  function updateBadge() {
    if (!badge) return;
    var n = unresolved().length;
    badge.textContent = n > 99 ? "99+" : String(n);
    badge.hidden = n === 0;
    cmt.setAttribute("aria-label", n ? "Comments (" + n + " open)" : "Comments");
    cmt.title = cmt.getAttribute("aria-label");
  }
  function load() {
    return call("GET", "").then(function (j) {
      comments = (j && j.comments) || [];
      loadErr = "";
    }, function (e) { loadErr = e.message; }).then(function () {
      updateBadge();
      placePins();
    });
  }

  // Where a comment points: its element (found again by selector) and the
  // spot inside it, as viewport coordinates; null when it can't be shown.
  function target(c) {
    if (!c.selector) return null;
    var t = null;
    try { t = doc.querySelector(c.selector); } catch (e) {}
    if (!t || t === host) return null;
    var r = t.getBoundingClientRect();
    if (!r.width && !r.height) return null;
    return { el: t, x: r.left + c.x * r.width, y: r.top + c.y * r.height, r: r };
  }
  var pinEls = [];
  function placePins() {
    for (var i = 0; i < pinEls.length; i++) pinEls[i].remove();
    pinEls = [];
    var list = unresolved().slice();
    if (draft && draft.selector) list.push({ id: 0, selector: draft.selector, x: draft.x, y: draft.y, draft: true });
    for (var j = 0; j < list.length; j++) {
      var c = list[j], t = target(c);
      if (!t || t.y < 0 || t.y > innerHeight + 22 || t.x < -22 || t.x > innerWidth) continue;
      var p = el("button", c.draft ? "pin new" : "pin", c.draft ? "+" : String(j + 1));
      p.style.left = Math.round(t.x) + "px";
      p.style.top = Math.round(t.y) + "px";
      p.setAttribute("aria-label", c.draft ? "New comment" : "Comment " + (j + 1));
      if (!c.draft) (function (id) {
        p.addEventListener("click", function (e) { e.stopPropagation(); focusId = id; openList(); });
      })(c.id);
      pins.appendChild(p);
      pinEls.push(p);
    }
    if (draft) showHL(draft.el);
  }
  var raf = 0;
  function schedule() {
    if (raf) return;
    raf = requestAnimationFrame(function () { raf = 0; placePins(); });
  }

  function showHL(t) {
    if (!t || !t.getBoundingClientRect) { hl.hidden = true; return; }
    var r = t.getBoundingClientRect();
    hl.style.left = r.left - 2 + "px";
    hl.style.top = r.top - 2 + "px";
    hl.style.width = r.width + 4 + "px";
    hl.style.height = r.height + 4 + "px";
    hl.hidden = false;
  }

  function cssEsc(s) {
    if (window.CSS && CSS.escape) return CSS.escape(s);
    return s.replace(/[^a-zA-Z0-9_-]/g, function (ch) { return "\\" + ch; });
  }
  // A selector that finds el again: the nearest ancestor with a unique id,
  // then tag:nth-of-type steps down to el.
  function selectorOf(t) {
    var parts = [];
    for (var n = t; n && n.nodeType === 1 && n !== root; n = n.parentElement) {
      if (n.id) {
        var idSel = "#" + cssEsc(n.id);
        try {
          if (doc.querySelectorAll(idSel).length === 1) { parts.unshift(idSel); break; }
        } catch (e) {}
      }
      var tag = n.localName, i = 1, same = false, s = n;
      while ((s = s.previousElementSibling)) if (s.localName === tag) { i++; same = true; }
      for (s = n.nextElementSibling; s && !same; s = s.nextElementSibling) if (s.localName === tag) same = true;
      parts.unshift(same ? cssEsc(tag) + ":nth-of-type(" + i + ")" : cssEsc(tag));
    }
    var sel = parts.join(" > ");
    return sel.length > 1000 ? "" : sel;
  }
  function textOf(t) {
    var s = "";
    try { s = t.innerText || t.textContent || ""; } catch (e) {}
    if (!s.trim()) s = (t.getAttribute && (t.getAttribute("alt") || t.getAttribute("aria-label") || t.getAttribute("title"))) || "";
    return s.replace(/\s+/g, " ").trim().slice(0, 200);
  }

  function inBar(e) {
    var path = e.composedPath ? e.composedPath() : [];
    for (var i = 0; i < path.length; i++) if (path[i] === host) return true;
    return false;
  }
  function onPickMove(e) {
    if (inBar(e)) { showHL(null); return; }
    showHL(e.target);
  }
  // While picking, the page sees none of the input that makes the pick. Mouse
  // buttons also lose their default (focus, selection); touch and keys keep
  // theirs, so the page still scrolls and the browser still makes the click.
  function swallow(e) {
    if (inBar(e)) return;
    e.stopPropagation();
    if (/^(mousedown|mouseup|pointerup|auxclick|dblclick)$/.test(e.type)) e.preventDefault();
  }
  function onPickClick(e) {
    if (inBar(e)) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    var t = e.target;
    if (!t || t.nodeType !== 1 || t === root || t === doc.body) { compose(null); return; }
    var r = t.getBoundingClientRect();
    compose({
      el: t,
      selector: selectorOf(t),
      quote: textOf(t),
      x: r.width ? Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)) : 0,
      y: r.height ? Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)) : 0
    });
  }
  function onPickKey(e) {
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); openList(); }
  }
  var pickEvents = ["pointerdown", "mousedown", "mouseup", "pointerup", "auxclick", "dblclick",
    "touchstart", "touchend", "contextmenu", "keydown", "keyup", "keypress"];
  function startPick() {
    if (picking) return;
    picking = true;
    addEventListener("pointermove", onPickMove, true);
    addEventListener("click", onPickClick, true);
    addEventListener("keydown", onPickKey, true);
    for (var i = 0; i < pickEvents.length; i++) addEventListener(pickEvents[i], swallow, true);
    root.style.setProperty("cursor", "crosshair", "important");
  }
  function stopPick() {
    if (!picking) return;
    picking = false;
    removeEventListener("pointermove", onPickMove, true);
    removeEventListener("click", onPickClick, true);
    removeEventListener("keydown", onPickKey, true);
    for (var i = 0; i < pickEvents.length; i++) removeEventListener(pickEvents[i], swallow, true);
    root.style.removeProperty("cursor");
  }

  function panel(label, from) {
    closeManual();
    from = from || cmt;
    man = el("div", "man cp");
    man.setAttribute("role", "group");
    man.setAttribute("aria-label", label);
    w.appendChild(man);
    from.setAttribute("aria-expanded", "true");
    opener = from;
    return man;
  }
  function button(text, cls, fn) {
    var b = el("button", cls || "", text);
    b.addEventListener("click", function (e) { e.stopPropagation(); fn(b); });
    return b;
  }
  function ago(iso) {
    var s = Math.max(0, Math.floor((Date.now() - Date.parse(iso)) / 1000));
    if (s < 60) return "just now";
    if (s < 3600) return Math.floor(s / 60) + "m ago";
    if (s < 86400) return Math.floor(s / 3600) + "h ago";
    return Math.floor(s / 86400) + "d ago";
  }

  // Pick mode: the next click on the page chooses what to comment on.
  function pick() {
    var p = panel("Add a comment");
    var hd = el("div", "hd");
    hd.appendChild(el("span", "sp1", "Click the part of the page to comment on"));
    p.appendChild(hd);
    var ac = el("div", "ac");
    ac.appendChild(button("Whole page", "", function () { compose(null); }));
    ac.appendChild(button("Cancel", "", function () { openList(); }));
    p.appendChild(ac);
    startPick();
  }

  function compose(anchor) {
    var p = panel("New comment");
    draft = anchor ? { el: anchor.el, selector: anchor.selector, x: anchor.x, y: anchor.y } : null;
    placePins();
    var hd = el("div", "ih");
    hd.appendChild(el("span", "q", anchor ? (anchor.quote ? "On “" + anchor.quote + "”" : "On <" + anchor.el.localName + ">") : "On the whole page"));
    p.appendChild(hd);
    var ta = doc.createElement("textarea");
    ta.setAttribute("aria-label", "Comment");
    ta.placeholder = "What should change?";
    ta.maxLength = 4000;
    p.appendChild(ta);
    var err = el("div", "er");
    err.hidden = true;
    p.appendChild(err);
    var ac = el("div", "ac");
    var post = button("Comment", "pr", submit);
    ac.appendChild(post);
    ac.appendChild(button("Cancel", "", function () { openList(); }));
    p.appendChild(ac);
    function submit() {
      if (!ta.value.trim()) { ta.focus(); return; }
      post.disabled = true;
      err.hidden = true;
      call("POST", "", {
        selector: anchor ? anchor.selector : "", quote: anchor ? anchor.quote : "",
        x: anchor ? anchor.x : 0, y: anchor ? anchor.y : 0, body: ta.value
      }).then(function (c) {
        comments.push(c);
        focusId = c.id;
        updateBadge();
        openList(true);
      }, function (e) {
        post.disabled = false;
        err.textContent = e.message;
        err.hidden = false;
      });
    }
    ta.addEventListener("keydown", function (e) {
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); submit(); }
    });
    ta.focus();
  }

  function item(c, n) {
    var it = el("div", c.resolved ? "it r" : "it");
    if (c.id === focusId) it.className += " on";
    var ih = el("div", "ih");
    if (n) ih.appendChild(el("span", "n", String(n)));
    var q = el("span", "q", c.selector ? (c.quote ? "“" + c.quote + "”" : c.selector) : "Whole page");
    q.title = c.selector || "";
    ih.appendChild(q);
    if (c.selector) {
      ih.appendChild(button("Show", "", function () {
        var t = target(c);
        if (t) t.el.scrollIntoView({ block: "center" });
        placePins();
      }));
    }
    it.appendChild(ih);
    it.appendChild(el("div", "tx", c.body));
    var mt = (c.author ? c.author + " · " : "") + ago(c.created);
    if (c.stale) mt += " · on an earlier version";
    if (c.resolved) mt += " · resolved";
    it.appendChild(el("div", "mt", mt));
    if (c.reply) {
      var rp = el("div", "rp");
      rp.appendChild(el("b", "", "Reply"));
      rp.appendChild(doc.createTextNode(c.reply));
      it.appendChild(rp);
    }
    var ac = el("div", "ac");
    ac.appendChild(button(c.resolved ? "Reopen" : "Resolve", "", function (b) {
      b.disabled = true;
      call("PATCH", "/" + c.id, { resolved: !c.resolved }).then(function (u) {
        for (var i = 0; i < comments.length; i++) if (comments[i].id === u.id) comments[i] = u;
        updateBadge();
        openList(true);
      }, function (e) { b.disabled = false; b.textContent = e.message; });
    }));
    ac.appendChild(button("Delete", "", function (b) {
      if (b.textContent !== "Delete?") { b.textContent = "Delete?"; return; }
      b.disabled = true;
      call("DELETE", "/" + c.id).then(function () {
        comments = comments.filter(function (x) { return x.id !== c.id; });
        updateBadge();
        openList(true);
      }, function (e) { b.disabled = false; b.textContent = e.message; });
    }));
    it.appendChild(ac);
    return it;
  }

  // The comment list; fresh fetches it again first, so replies an agent left
  // since the page loaded show up.
  function openList(cached) {
    var p = panel("Comments");
    var hd = el("div", "hd");
    hd.appendChild(el("span", "sp1", "Comments"));
    hd.appendChild(button("Add", "pr", pick));
    p.appendChild(hd);
    var body = el("div");
    p.appendChild(body);
    function render() {
      body.textContent = "";
      if (loadErr) {
        body.appendChild(el("div", "er", loadErr));
        body.appendChild(button("Retry", "", function () { openList(); }));
        return;
      }
      var o = unresolved(), done = [];
      for (var i = 0; i < comments.length; i++) if (comments[i].resolved) done.push(comments[i]);
      if (!o.length && !done.length) {
        body.appendChild(el("p", "mu", "No comments yet. Add one, then ask your agent to read them: it has a comments tool."));
      }
      for (var j = 0; j < o.length; j++) body.appendChild(item(o[j], j + 1));
      if (done.length) {
        body.appendChild(button((showResolved ? "Hide" : "Show") + " resolved (" + done.length + ")", "", function () {
          showResolved = !showResolved;
          render();
        }));
        if (showResolved) for (var k = 0; k < done.length; k++) body.appendChild(item(done[k], 0));
      }
      var on = focusId && body.querySelector(".on");
      if (on) on.scrollIntoView({ block: "nearest" });
      focusId = 0;
      placePins();
    }
    if (cached) { render(); return; }
    body.appendChild(el("p", "mu", "Loading…"));
    load().then(function () { if (man === p) render(); });
  }

  if (cmt) {
    cmt.addEventListener("click", function () {
      if (cmt.getAttribute("aria-expanded") === "true") closeManual();
      else openList();
    });
    sh.appendChild(pins);
    addEventListener("scroll", schedule, true);
    addEventListener("resize", schedule);
    load();
    doc.addEventListener("visibilitychange", function () { if (doc.visibilityState === "visible") load(); });
  }

  // --- listen ---
  // The audio version: a model describes the page (its text, pictures and
  // design) and the server reads that aloud. The player lives outside the
  // panel, so closing the panel doesn't stop playback.
  var lstat = null, lpoll = 0, audioFor = "", hold = el("div");
  hold.hidden = true;
  var player = doc.createElement("audio");
  player.controls = true;
  player.preload = "auto";
  player.setAttribute("aria-label", "Audio version of the page");
  hold.appendChild(player);
  var speaking = false;
  function stopListenPoll() { clearTimeout(lpoll); lpoll = 0; }
  function lcall(method, path, body) { return request(D.listen + path, method, body); }
  var stepText = { queued: "Waiting for another audio version to finish…", page: "Reading the page…",
    script: "Writing the narration…", audio: "Recording the audio…" };
  function mins(sec) {
    var m = Math.floor(sec / 60), s = Math.round(sec % 60);
    if (s === 60) { m++; s = 0; }
    return m + ":" + (s < 10 ? "0" : "") + s;
  }
  function playing(on) {
    if (!lsn) return;
    lsn.className = on ? "ok" : "";
  }
  player.addEventListener("play", function () { playing(true); });
  player.addEventListener("pause", function () { playing(false); });
  player.addEventListener("ended", function () { playing(false); });
  function mediaSession() {
    try {
      if (navigator.mediaSession && window.MediaMetadata) {
        navigator.mediaSession.metadata = new MediaMetadata({ title: D.title, artist: "glim" });
      }
    } catch (e) {}
  }
  function loadAudio(r, err) {
    var key = r.version + r.created;
    if (audioFor === key) return;
    audioFor = key;
    // The query changes with each version, so a new one never comes from the cache.
    fetch(D.listen + "/audio?v=" + encodeURIComponent(r.created), { headers: { "X-Glim-Bar": D.tok }, credentials: "omit", mode: "cors" }).then(function (res) {
      if (!res.ok) throw new Error("The audio couldn't be loaded (" + res.status + ").");
      return res.blob();
    }).then(function (b) {
      if (audioFor !== key) return;
      if (player.src) URL.revokeObjectURL(player.src);
      player.src = URL.createObjectURL(b);
      mediaSession();
    }, function (e) {
      audioFor = "";
      err.textContent = e.message;
      err.hidden = false;
    });
  }
  function speak(r, b) {
    var ss = window.speechSynthesis;
    if (!ss) return;
    if (speaking) { ss.cancel(); speaking = false; b.textContent = "Read aloud"; playing(false); return; }
    var u = new SpeechSynthesisUtterance(r.narration);
    if (r.language) u.lang = r.language;
    u.onend = u.onerror = function () { speaking = false; b.textContent = "Read aloud"; playing(false); };
    speaking = true;
    b.textContent = "Stop";
    playing(true);
    ss.speak(u);
  }
  function start(again) {
    lstat = { state: "working", step: "queued", result: lstat && lstat.result, speech: lstat && lstat.speech };
    renderListen();
    lcall("POST", "", again ? { again: true } : null).then(function (st) {
      lstat = st;
      renderListen();
    }, function (e) {
      lstat = { state: "failed", error: e.message, result: lstat && lstat.result };
      renderListen();
    });
  }
  var lpanel = null;
  function renderListen() {
    if (!lpanel || man !== lpanel) return;
    var p = lpanel;
    p.textContent = "";
    var hd = el("div", "hd");
    hd.appendChild(el("span", "sp1", "Listen"));
    p.appendChild(hd);
    var st = lstat;
    if (!st) { p.appendChild(el("p", "mu", "Loading…")); return; }
    var r = st.result;
    if (st.state === "working") {
      var t = stepText[st.step] || "Working…";
      // done is left out of the JSON while it is 0.
      if (st.step === "audio" && st.total > 1) t = "Recording the audio, part " + Math.min((st.done || 0) + 1, st.total) + " of " + st.total + "…";
      p.appendChild(el("p", "mu", t));
      p.appendChild(el("p", "mu", "You can close this panel; it keeps going."));
      stopListenPoll();
      lpoll = setTimeout(refreshListen, 2000);
      return;
    }
    if (st.state === "failed") {
      p.appendChild(el("div", "er", st.error || "It didn't work."));
      var ac0 = el("div", "ac");
      ac0.appendChild(button("Try again", "pr", function () { start(true); }));
      p.appendChild(ac0);
      if (!r) return;
    }
    if (!r) {
      p.appendChild(el("p", "mu", "Get a spoken walkthrough of this page: a model describes its text, pictures and design, and it's read aloud."));
      var ac1 = el("div", "ac");
      ac1.appendChild(button("Make the audio version", "pr", function () { start(false); }));
      p.appendChild(ac1);
      return;
    }
    if (st.stale) {
      var ac2 = el("div", "ac");
      ac2.appendChild(el("span", "mu", "The page changed since this was made."));
      ac2.appendChild(button("Update", "pr", function () { start(true); }));
      p.appendChild(ac2);
    }
    var err = el("div", "er");
    err.hidden = true;
    if (r.audio) {
      p.appendChild(player);
      p.appendChild(err);
      loadAudio(r, err);
      var rt = el("div", "ac rt");
      [1, 1.25, 1.5, 2].forEach(function (v) {
        var b = button(v + "×", player.playbackRate === v ? "on" : "", function () {
          player.playbackRate = v;
          renderListen();
        });
        b.setAttribute("aria-label", "Speed " + v + "×");
        rt.appendChild(b);
      });
      p.appendChild(rt);
    } else if (window.speechSynthesis) {
      var ac3 = el("div", "ac");
      ac3.appendChild(button(speaking ? "Stop" : "Read aloud", "pr", function (b) { speak(r, b); }));
      p.appendChild(ac3);
    }
    if (r.truncated) p.appendChild(el("div", "mu", "The model stopped before the end of the page, so the audio ends early."));
    p.appendChild(el("div", "sum", r.summary));
    var meta = [];
    if (r.seconds) meta.push(mins(r.seconds));
    meta.push(ago(r.created));
    if (r.model) meta.push(r.model);
    if (!r.rendered) meta.push("read from the HTML, without screenshots");
    p.appendChild(el("div", "mt", meta.join(" · ")));
    var det = doc.createElement("details");
    det.appendChild(el("summary", "", "Transcript"));
    det.appendChild(el("div", "trn", r.narration));
    p.appendChild(det);
    var ac4 = el("div", "ac");
    if (!st.stale) ac4.appendChild(button("Make it again", "", function () { start(true); }));
    p.appendChild(ac4);
  }
  function refreshListen() {
    lcall("GET", "").then(function (st) { lstat = st; }, function (e) {
      lstat = { state: "failed", error: e.message, result: lstat && lstat.result };
    }).then(renderListen);
  }
  function openListen() {
    lpanel = panel("Listen", lsn);
    lpanel.className += " ls";
    renderListen();
    refreshListen();
  }
  if (lsn) {
    sh.appendChild(hold);
    lsn.addEventListener("click", function () {
      if (lsn.getAttribute("aria-expanded") === "true") closeManual();
      else openListen();
    });
  }

  function set(collapsed) {
    w.className = collapsed ? "w c" : "w";
    pins.className = collapsed ? "pins off" : "pins";
    host.setAttribute("data-state", collapsed ? "collapsed" : "expanded");
    if (collapsed) closeManual();
  }
  col.addEventListener("click", function () { set(true); });
  pill.addEventListener("click", function () { set(false); });
  w.appendChild(bar);
  w.appendChild(pill);
  sh.appendChild(w);
  set(false);
  root.appendChild(host);
} catch (e) {}
