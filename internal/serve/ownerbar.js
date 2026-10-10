// The owner-only glim bar. Wrapped by ownerbar.go as function(D){...}(data);
// D is {title, url, dash, exp, locked}. It renders a fixed, closed shadow DOM
// overlay, never touches page layout, and swallows every error.
try {
  var doc = document, root = doc.documentElement;
  if (!root || doc.querySelector("glim-bar")) return;
  var host = doc.createElement("glim-bar");
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
  var css =
    ":host{all:initial}" +
    "*{box-sizing:border-box}" +
    ".w{position:relative;display:flex;justify-content:center;pointer-events:none;" +
    "padding:0 8px calc(8px + env(safe-area-inset-bottom,0px));" +
    "font:13px/1 -apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;" +
    "--bg:rgba(255,255,255,.94);--fg:#1c1b19;--mut:#6b675f;--bd:rgba(0,0,0,.14);--hv:rgba(0,0,0,.07);--ac:#c2410c}" +
    "@media(prefers-color-scheme:dark){.w{--bg:rgba(28,27,25,.94);--fg:#f1efe9;--mut:#a8a398;--bd:rgba(255,255,255,.18);--hv:rgba(255,255,255,.1);--ac:#fb923c}}" +
    ".bar,.pill{pointer-events:auto;background:var(--bg);color:var(--fg);border:1px solid var(--bd);" +
    "box-shadow:0 2px 12px rgba(0,0,0,.22);-webkit-backdrop-filter:blur(8px);backdrop-filter:blur(8px)}" +
    ".bar{display:flex;align-items:center;gap:4px;width:100%;max-width:640px;height:36px;padding:0 4px 0 10px;border-radius:18px}" +
    ".w.c{justify-content:flex-end}.w.c .bar,.w:not(.c) .pill{display:none}" +
    ".mark{flex:none;display:flex;color:var(--ac)}" +
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
    ".man input{all:unset;box-sizing:border-box;display:block;width:100%;padding:6px 8px;color:var(--fg);font:12px monospace;" +
    "user-select:text;-webkit-user-select:text}";
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
  var MARK = ["M12 3l1.9 5.1L19 10l-5.1 1.9L12 17l-1.9-5.1L5 10l5.1-1.9z", "M19 16l.8 2.2L22 19l-2.2.8L19 22l-.8-2.2L16 19l2.2-.8z"];
  function btn(tag, label) {
    var b = el(tag);
    b.setAttribute("aria-label", label);
    b.title = label;
    return b;
  }

  var w = el("div", "w");
  var bar = el("div", "bar");
  var mk = el("span", "mark");
  mk.appendChild(icon(MARK, 16));
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
  bar.appendChild(dash);
  bar.appendChild(col);

  var pill = btn("button", "Show glim bar");
  pill.className = "pill";
  pill.appendChild(icon(MARK, 14));
  pill.appendChild(el("span", "", "glim"));

  var man = null;
  function closeManual() { if (man) { man.remove(); man = null; } }
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
