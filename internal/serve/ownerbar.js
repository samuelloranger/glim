// The owner-only glim bar. Wrapped by ownerbar.go as function(D){...}(data);
// D is {title, url, dash, exp, locked, qr?: {size, path}}. It renders a fixed, closed shadow DOM
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
    "--bg:rgba(255,255,255,.94);--fg:#1c1b19;--mut:#6b675f;--bd:rgba(0,0,0,.14);--hv:rgba(0,0,0,.07);--ac:#c2410c;--mk:#6366f1}" +
    "@media(prefers-color-scheme:dark){.w{--bg:rgba(28,27,25,.94);--fg:#f1efe9;--mut:#a8a398;--bd:rgba(255,255,255,.18);--hv:rgba(255,255,255,.1);--ac:#fb923c;--mk:#818cf8}}" +
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
  bar.appendChild(dash);
  bar.appendChild(col);

  var pill = btn("button", "Show glim bar");
  pill.className = "pill";
  pill.appendChild(mark(14));
  pill.appendChild(el("span", "", "glim"));

  // One panel above the bar at a time: the hand-copy link or the share panel.
  var man = null;
  function closeManual() {
    if (man) { man.remove(); man = null; }
    share.setAttribute("aria-expanded", "false");
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
  }
  share.addEventListener("click", function () {
    if (share.getAttribute("aria-expanded") === "true") closeManual();
    else openShare();
  });
  host.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && man) { closeManual(); share.focus(); }
  });

  function set(collapsed) {
    w.className = collapsed ? "w c" : "w";
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
