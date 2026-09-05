/* AskDesk embeddable chat widget — self-contained, no dependencies.
 * Embed with:
 *   <script src="https://YOUR-HOST/widget.js"
 *           data-key="PUBLIC_API_KEY"
 *           data-api="https://YOUR-HOST"   (optional; defaults to this script's origin)
 *           data-color="#0D7A5F"           (optional; header colour)
 *           data-accent="#0D7A5F"          (optional; buttons/bubbles/launcher — defaults to data-color)
 *           data-bg="#F7F7F5"              (optional; chat background)
 *           data-logo="https://…/logo.png" (optional; header logo)
 *           data-position="right"          (optional; left | right)
 *           data-telegram="https://t.me/…" (optional; "continue on Telegram" link)
 *   ></script>
 */
(function () {
  "use strict";
  var script = document.currentScript;
  if (!script) return;
  var API = (script.getAttribute("data-api") || new URL(script.src).origin).replace(/\/+$/, "");
  var KEY = script.getAttribute("data-key") || "";
  var COLOR = script.getAttribute("data-color") || "#0D7A5F"; // header
  var ACCENT = script.getAttribute("data-accent") || COLOR;   // launcher/buttons/user bubbles
  var BG = script.getAttribute("data-bg") || "#F7F7F5";       // chat background
  var TELEGRAM = script.getAttribute("data-telegram") || ""; // optional t.me/... handoff link
  var LOGO = script.getAttribute("data-logo") || "";         // optional header logo URL
  var POS = script.getAttribute("data-position") === "left" ? "left" : "right";
  var LOGO_SVG = '<svg role="img" aria-label="AskDesk" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1543.259098 225.797903"><g transform="translate(-114.740902,553.000000) scale(0.100000,-0.100000)" fill="currentColor" stroke="none"><path d="M2574 5408 c-128 -204 -610 -921 -1079 -1608 -275 -403 -352 -519 -347 -524 11 -12 667 6 678 17 34 35 465 667 865 1269 128 192 236 345 240 341 15 -17 432 -1043 426 -1049 -2 -3 -161 -5 -353 -5 -354 0 -458 -4 -448 -20 2 -5 71 -64 152 -131 81 -68 225 -189 320 -270 l173 -148 516 0 c487 0 515 1 508 18 -4 9 -60 154 -125 322 -64 168 -152 395 -194 504 -43 110 -144 367 -226 570 -139 347 -309 779 -323 819 -6 16 -32 17 -356 17 l-350 0 -77 -122z"/><path d="M6570 5503 c0 -35 -181 -1612 -223 -1951 -18 -144 -31 -265 -28 -267 2 -3 129 -5 282 -5 l277 0 5 23 c3 12 20 153 37 312 17 160 34 309 38 332 l7 41 65 -81 c36 -45 160 -205 274 -354 l209 -273 343 0 c189 0 344 2 344 6 0 3 -35 49 -78 102 -44 53 -158 196 -255 317 -96 121 -209 261 -251 312 l-76 91 395 400 c217 220 395 403 395 406 0 3 -152 6 -339 6 l-339 0 -308 -322 c-170 -178 -319 -334 -331 -347 -29 -30 -30 -44 7 252 16 133 38 328 50 432 11 105 29 264 40 355 11 91 20 180 20 198 l0 32 -280 0 c-260 0 -280 -1 -280 -17z"/><path d="M8590 5508 c0 -7 -11 -112 -25 -233 -14 -121 -25 -226 -25 -232 0 -10 111 -13 520 -13 335 0 538 -4 572 -11 90 -18 211 -83 278 -148 109 -107 157 -246 147 -421 -11 -178 -76 -319 -207 -451 -69 -70 -101 -94 -171 -128 -145 -70 -209 -81 -461 -81 -206 0 -218 1 -218 19 0 10 16 151 35 312 31 255 85 729 85 744 0 3 -135 5 -300 5 -216 0 -300 -3 -300 -11 0 -17 -63 -554 -155 -1327 -14 -117 -25 -219 -25 -228 0 -14 65 -15 618 -11 689 4 681 3 894 80 482 174 788 642 788 1205 0 234 -61 422 -192 597 -72 95 -162 166 -293 230 -218 107 -285 115 -997 115 -448 0 -568 -3 -568 -12z"/><path d="M14816 5483 c-3 -21 -10 -81 -16 -133 -5 -52 -19 -174 -30 -270 -11 -96 -25 -213 -30 -260 -5 -47 -17 -143 -25 -215 -9 -71 -29 -256 -46 -410 -16 -154 -45 -417 -64 -583 -19 -167 -35 -310 -35 -318 0 -12 43 -14 283 -12 l282 3 7 80 c10 106 46 428 60 537 l11 87 116 -147 c64 -81 166 -212 227 -292 60 -80 132 -173 159 -207 l48 -63 344 0 c188 0 343 3 343 8 0 9 -403 514 -552 692 l-105 125 198 205 c109 113 286 293 393 400 108 107 196 198 196 202 0 5 -154 8 -342 7 l-343 0 -204 -217 c-376 -399 -451 -474 -451 -453 0 11 13 134 30 273 16 139 45 397 65 573 19 176 38 344 42 373 l6 52 -281 0 -280 0 -6 -37z"/><path d="M11455 4925 c-345 -65 -595 -310 -684 -667 -63 -254 -41 -488 63 -665 49 -83 158 -183 248 -227 156 -76 245 -86 785 -86 238 0 433 1 434 3 0 1 13 99 27 217 l27 215 -450 5 c-433 5 -452 6 -495 26 -73 35 -114 85 -121 149 l-4 30 615 5 615 5 23 125 c26 146 33 375 13 455 -47 190 -201 342 -406 401 -81 23 -582 30 -690 9z m478 -441 c52 -27 76 -55 92 -108 8 -27 15 -56 15 -63 0 -10 -68 -13 -355 -13 -241 0 -355 3 -355 10 0 39 73 130 125 157 53 28 77 31 263 32 150 1 189 -2 215 -15z"/><path d="M4926 4904 c-111 -22 -209 -74 -285 -152 -119 -121 -164 -239 -165 -432 -1 -97 3 -126 23 -185 31 -91 107 -170 196 -206 59 -24 66 -24 470 -29 226 -3 418 -9 428 -14 45 -22 49 -83 8 -128 l-29 -33 -563 -5 -564 -5 -22 -195 c-12 -107 -22 -205 -22 -218 l-1 -23 648 3 c710 4 693 3 820 67 121 61 222 183 270 328 22 67 26 96 26 208 l1 130 -38 77 c-30 62 -48 86 -90 117 -118 88 -138 91 -614 91 -377 0 -383 0 -404 21 -17 17 -20 29 -16 62 3 22 15 49 27 61 20 21 28 21 573 26 l552 5 26 215 c14 118 24 218 22 223 -2 4 -275 7 -606 6 -467 -1 -617 -4 -671 -15z"/><path d="M13215 4913 c-224 -30 -411 -194 -470 -413 -9 -30 -18 -95 -21 -145 -14 -204 58 -345 214 -418 l67 -32 416 -5 c392 -5 418 -6 438 -24 23 -21 28 -74 10 -108 -25 -47 -42 -48 -624 -48 l-545 0 -1 -22 c0 -13 -10 -110 -23 -217 -17 -145 -19 -194 -10 -198 6 -2 297 -3 645 -1 612 4 636 5 701 25 232 73 378 255 409 508 30 242 -95 435 -306 474 -36 7 -208 11 -441 11 l-383 0 -20 26 c-12 15 -21 35 -21 46 0 31 27 73 55 86 19 9 167 12 561 12 419 0 536 3 539 13 8 22 53 422 48 430 -4 7 -1182 8 -1238 0z"/></g></svg>';
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
.adk-btn{position:fixed;bottom:20px;width:56px;height:56px;border-radius:50%;background:$A;color:#fff;border:0;cursor:pointer;box-shadow:0 6px 20px rgba(0,0,0,.25);z-index:2147483000;display:flex;align-items:center;justify-content:center;transition:transform .15s}\
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
.adk-body{flex:1;overflow-y:auto;background:$BG;padding:14px}\
.adk-msg{max-width:82%;padding:9px 12px;border-radius:14px;font-size:14px;line-height:1.5;margin-bottom:10px;white-space:pre-wrap;word-wrap:break-word}\
.adk-bot{background:#fff;border:1px solid #ececec;color:#1a1a1a;border-bottom-left-radius:4px}\
.adk-me{background:$A;color:#fff;margin-left:auto;border-bottom-right-radius:4px}\
.adk-chips{display:flex;flex-wrap:wrap;gap:7px;margin-bottom:10px}\
.adk-chip{background:#fff;border:1px solid #ddd;color:#1a1a1a;border-radius:999px;padding:7px 12px;font-size:13px;cursor:pointer}\
.adk-chip:hover{border-color:$A}\
.adk-form{background:#fff;border:1px solid #ececec;border-radius:12px;padding:12px;margin-bottom:10px}\
.adk-form p{margin:0 0 8px;font-size:13px;color:#444}\
.adk-form input{width:100%;box-sizing:border-box;padding:9px;font:inherit;font-size:14px;border:1px solid #ccc;border-radius:8px;margin-bottom:8px}\
.adk-form button{background:$A;color:#fff;border:0;border-radius:8px;padding:9px 14px;font-size:14px;cursor:pointer;width:100%}\
.adk-tg{display:block;text-align:center;margin-top:8px;font-size:13px;color:$A;text-decoration:none}\
.adk-foot{flex:none;text-align:center;padding:7px;font-size:11px;color:#9a9a9a;border-top:1px solid #eee;background:#fff}\
.adk-foot a{color:#9a9a9a;text-decoration:none}\
.adk-brand{display:inline-flex;align-items:center;vertical-align:middle;margin-left:2px}\
.adk-brand svg{height:12px;width:auto;display:block}\
.adk-in{flex:none;display:flex;gap:8px;padding:10px;border-top:1px solid #eee;background:#fff}\
.adk-in input{flex:1;border:1px solid #ddd;border-radius:10px;padding:9px 11px;font:inherit;font-size:14px}\
.adk-in button{background:$A;color:#fff;border:0;border-radius:10px;padding:0 14px;font-size:14px;cursor:pointer}\
.adk-in button:disabled{opacity:.5}\
".replace(/\$C/g, COLOR).replace(/\$A/g, ACCENT).replace(/\$BG/g, BG);
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
      LOGO_SVG + '</a>';
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
