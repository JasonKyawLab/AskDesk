/* AskDesk embeddable chat widget — self-contained, no dependencies.
 * Embed with:
 *   <script src="https://YOUR-HOST/widget.js"
 *           data-key="PUBLIC_API_KEY"
 *           data-api="https://YOUR-HOST"     (optional; defaults to this script's origin)
 *           data-color="#0D7A5F"></script>   (optional accent colour)
 */
(function () {
  "use strict";
  var script = document.currentScript;
  if (!script) return;
  var API = (script.getAttribute("data-api") || new URL(script.src).origin).replace(/\/+$/, "");
  var KEY = script.getAttribute("data-key") || "";
  var COLOR = script.getAttribute("data-color") || "#0D7A5F";
  var TELEGRAM = script.getAttribute("data-telegram") || ""; // optional t.me/... handoff link
  var LOGO = script.getAttribute("data-logo") || "";         // optional header logo URL
  var POS = script.getAttribute("data-position") === "left" ? "left" : "right";
  if (!KEY) { console.error("[AskDesk] widget: missing data-key"); return; }

  function store(k, v) { try { if (v === undefined) return localStorage.getItem(k); localStorage.setItem(k, v); } catch (e) { return null; } }
  var session = store("askdesk_session");
  if (!session) { session = (window.crypto && crypto.randomUUID) ? crypto.randomUUID() : String(Math.random()).slice(2) + Date.now(); store("askdesk_session", session); }
  var contactDone = store("askdesk_lead_" + KEY) === "1";

  var cfg = { business_name: "Support", welcome: "", require_contact: false, source_url: "", categories: [] };
  var faqs = [];        // [{name, faqs:[{id,question,answer}]}]
  var lastReplyId = 0, open = false, sending = false, gating = false, pollTimer = null;

  function api(path, opts) {
    opts = opts || {};
    opts.headers = Object.assign({ "Content-Type": "application/json", "X-API-Key": KEY }, opts.headers || {});
    return fetch(API + path, opts).then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); });
  }
  function el(tag, cls, txt) { var e = document.createElement(tag); if (cls) e.className = cls; if (txt != null) e.textContent = txt; return e; }

  // ---- styles ----
  var css = "\
.adk-btn{position:fixed;bottom:20px;width:56px;height:56px;border-radius:50%;background:$C;color:#fff;border:0;cursor:pointer;box-shadow:0 6px 20px rgba(0,0,0,.25);z-index:2147483000;display:flex;align-items:center;justify-content:center;transition:transform .15s}\
.adk-btn:hover{transform:scale(1.05)}\
.adk-btn.adk-right{right:20px}.adk-btn.adk-left{left:20px}\
.adk-panel{position:fixed;bottom:88px;width:370px;max-width:calc(100vw - 32px);height:560px;max-height:calc(100vh - 120px);background:#fff;border-radius:16px;box-shadow:0 12px 48px rgba(0,0,0,.22);z-index:2147483000;display:none;flex-direction:column;overflow:hidden;font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif}\
.adk-panel.adk-right{right:20px}.adk-panel.adk-left{left:20px}\
.adk-panel.adk-open{display:flex}\
.adk-hd{background:$C;color:#fff;padding:14px 16px;display:flex;align-items:center;justify-content:space-between;flex:none}\
.adk-hdl{display:flex;align-items:center;gap:9px;min-width:0}\
.adk-logo{width:26px;height:26px;border-radius:6px;object-fit:cover;background:#fff;flex:none}\
.adk-hd b{font-size:15px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}\
.adk-x{background:none;border:0;color:#fff;opacity:.85;font-size:20px;cursor:pointer;line-height:1}\
.adk-body{flex:1;overflow-y:auto;background:#F7F7F5;padding:14px}\
.adk-msg{max-width:82%;padding:9px 12px;border-radius:14px;font-size:14px;line-height:1.5;margin-bottom:10px;white-space:pre-wrap;word-wrap:break-word}\
.adk-bot{background:#fff;border:1px solid #ececec;color:#1a1a1a;border-bottom-left-radius:4px}\
.adk-me{background:$C;color:#fff;margin-left:auto;border-bottom-right-radius:4px}\
.adk-chips{display:flex;flex-wrap:wrap;gap:7px;margin-bottom:10px}\
.adk-chip{background:#fff;border:1px solid #ddd;color:#1a1a1a;border-radius:999px;padding:7px 12px;font-size:13px;cursor:pointer}\
.adk-chip:hover{border-color:$C}\
.adk-form{background:#fff;border:1px solid #ececec;border-radius:12px;padding:12px;margin-bottom:10px}\
.adk-form p{margin:0 0 8px;font-size:13px;color:#444}\
.adk-form input{width:100%;box-sizing:border-box;padding:9px;font:inherit;font-size:14px;border:1px solid #ccc;border-radius:8px;margin-bottom:8px}\
.adk-form button{background:$C;color:#fff;border:0;border-radius:8px;padding:9px 14px;font-size:14px;cursor:pointer;width:100%}\
.adk-tg{display:block;text-align:center;margin-top:8px;font-size:13px;color:$C;text-decoration:none}\
.adk-foot{flex:none;text-align:center;padding:7px;font-size:11px;color:#9a9a9a;border-top:1px solid #eee;background:#fff}\
.adk-foot a{color:#9a9a9a;text-decoration:none}\
.adk-brand{display:inline-flex;align-items:center;vertical-align:middle;margin-left:2px}\
.adk-brand svg{height:12px;width:auto;display:block}\
.adk-in{flex:none;display:flex;gap:8px;padding:10px;border-top:1px solid #eee;background:#fff}\
.adk-in input{flex:1;border:1px solid #ddd;border-radius:10px;padding:9px 11px;font:inherit;font-size:14px}\
.adk-in button{background:$C;color:#fff;border:0;border-radius:10px;padding:0 14px;font-size:14px;cursor:pointer}\
.adk-in button:disabled{opacity:.5}\
".replace(/\$C/g, COLOR);
  var s = document.createElement("style"); s.textContent = css; document.head.appendChild(s);

  // ---- DOM ----
  var btn = el("button", "adk-btn adk-" + POS); btn.setAttribute("aria-label", "Open chat");
  btn.innerHTML = '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>';
  var panel = el("div", "adk-panel adk-" + POS);
  var head = el("div", "adk-hd");
  var hdl = el("div", "adk-hdl");
  if (LOGO) { var logo = el("img", "adk-logo"); logo.src = LOGO; logo.alt = ""; hdl.appendChild(logo); }
  var title = el("b", null, "Support"); hdl.appendChild(title);
  var xbtn = el("button", "adk-x", "×");
  head.appendChild(hdl); head.appendChild(xbtn);
  var body = el("div", "adk-body");
  var foot = el("div", "adk-foot");
  var form = el("form", "adk-in");
  var input = el("input"); input.type = "text"; input.placeholder = "Type your question…";
  var send = el("button", null, "Send"); send.type = "submit";
  form.appendChild(input); form.appendChild(send);
  panel.appendChild(head); panel.appendChild(body); panel.appendChild(foot); panel.appendChild(form);
  document.body.appendChild(btn); document.body.appendChild(panel);

  function addMsg(text, who) { var m = el("div", "adk-msg " + (who === "me" ? "adk-me" : "adk-bot"), text); body.appendChild(m); body.scrollTop = body.scrollHeight; return m; }

  function showMenu() {
    if (cfg.welcome) addMsg(cfg.welcome, "bot");
    if (cfg.categories && cfg.categories.length) {
      var wrap = el("div", "adk-chips");
      cfg.categories.forEach(function (c) {
        var chip = el("button", "adk-chip", c); chip.type = "button";
        chip.onclick = function () { showCategory(c); };
        wrap.appendChild(chip);
      });
      body.appendChild(wrap); body.scrollTop = body.scrollHeight;
    }
  }
  function showCategory(name) {
    addMsg(name, "me");
    var cat = faqs.filter(function (c) { return c.name === name; })[0];
    if (!cat || !cat.faqs.length) { addMsg("No questions here yet.", "bot"); return; }
    var wrap = el("div", "adk-chips");
    cat.faqs.forEach(function (f) {
      var chip = el("button", "adk-chip", f.question); chip.type = "button";
      chip.onclick = function () { addMsg(f.question, "me"); addMsg(f.answer, "bot"); };
      wrap.appendChild(chip);
    });
    body.appendChild(wrap); body.scrollTop = body.scrollHeight;
  }

  // Contact-gate: before the first AI/human answer, collect email/phone.
  function askContact(then) {
    gating = true;
    var f = el("div", "adk-form");
    f.appendChild(el("p", null, "To reply to you personally, please share your contact:"));
    var email = el("input"); email.type = "email"; email.placeholder = "Your email";
    var phone = el("input"); phone.type = "text"; phone.placeholder = "Phone (optional)";
    var ok = el("button", null, "Continue"); ok.type = "button";
    f.appendChild(email); f.appendChild(phone); f.appendChild(ok);
    if (TELEGRAM) { var tg = el("a", "adk-tg", "→ Continue on Telegram instead"); tg.href = TELEGRAM; tg.target = "_blank"; f.appendChild(tg); }
    body.appendChild(f); body.scrollTop = body.scrollHeight; email.focus();
    ok.onclick = function () {
      if (!email.value.trim() && !phone.value.trim()) { email.focus(); return; }
      api("/api/v1/lead", { method: "POST", body: JSON.stringify({ session_id: session, email: email.value.trim(), phone: phone.value.trim() }) })
        .then(function () { contactDone = true; store("askdesk_lead_" + KEY, "1"); f.remove(); gating = false; then(); })
        .catch(function () { contactDone = true; f.remove(); gating = false; then(); });
    };
  }

  function ask(text) {
    var pending = addMsg("…", "bot"); sending = true; send.disabled = true;
    api("/api/v1/ask", { method: "POST", body: JSON.stringify({ message: text, session_id: session }) })
      .then(function (d) { pending.textContent = d.answer || ""; })
      .catch(function () { pending.textContent = "Sorry, something went wrong. Please try again."; })
      .then(function () { sending = false; send.disabled = false; });
  }

  form.onsubmit = function (e) {
    e.preventDefault();
    var text = input.value.trim();
    if (!text || sending || gating) return;
    input.value = ""; addMsg(text, "me");
    if (cfg.require_contact && !contactDone) { askContact(function () { ask(text); }); }
    else { ask(text); }
  };

  // Poll for admin (human) replies to this session.
  function poll() {
    api("/api/v1/replies?session_id=" + encodeURIComponent(session) + "&since=" + lastReplyId)
      .then(function (d) {
        (d.replies || []).forEach(function (r) { if (r.id > lastReplyId) lastReplyId = r.id; addMsg(r.message, "bot"); });
      }).catch(function () {});
  }

  function toggle(v) {
    open = v == null ? !open : v;
    panel.classList.toggle("adk-open", open);
    if (open) { input.focus(); if (!pollTimer) pollTimer = setInterval(poll, 5000); }
    else if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  }
  btn.onclick = function () { toggle(); };
  xbtn.onclick = function () { toggle(false); };

  // AskDesk brand wordmark (self-contained SVG; links to source for AGPL).
  function brandHTML(url) {
    return 'Powered by <a class="adk-brand" href="' + (url || "https://github.com/JasonKyawLab/AskDesk") +
      '" target="_blank" rel="noopener" aria-label="AskDesk — source">' +
      '<svg viewBox="0 0 108 20" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="AskDesk">' +
      '<text x="3" y="16" font-family="Arial Black,Arial,Helvetica,sans-serif" font-weight="900" ' +
      'font-size="18" letter-spacing="-.5" transform="skewX(-11)" fill="currentColor">AskDesk</text></svg></a>';
  }

  // ---- boot ----
  Promise.all([api("/api/v1/config"), api("/api/v1/faqs").catch(function () { return { categories: [] }; })])
    .then(function (res) {
      cfg = Object.assign(cfg, res[0]);
      faqs = (res[1].categories) || [];
      title.textContent = cfg.business_name || "Support";
      foot.innerHTML = brandHTML(cfg.source_url);
      showMenu();
    })
    .catch(function () { foot.innerHTML = brandHTML(""); addMsg("Chat is unavailable right now.", "bot"); });
})();
