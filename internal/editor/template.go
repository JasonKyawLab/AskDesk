package editor

// pageTemplate is the FAQ editor page. Data is editor.pageData. All fields
// render through html/template, which auto-escapes to prevent stored XSS.
const pageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>AskDesk — FAQ editor</title>
<style>
  :root { color-scheme: light dark; }
  * { box-sizing: border-box; }
  body { font-family: system-ui, sans-serif; margin: 0; padding: 1rem;
         max-width: 640px; margin-inline: auto; line-height: 1.5; }
  h1 { font-size: 1.25rem; }
  h2 { font-size: 1rem; margin-top: 1.5rem; }
  label { display: block; font-size: .85rem; margin-top: .6rem; opacity: .8; }
  input, textarea { width: 100%; padding: .55rem; font: inherit;
                    border: 1px solid #8886; border-radius: 8px; background: transparent; color: inherit; }
  textarea { min-height: 4rem; resize: vertical; }
  button { font: inherit; padding: .5rem .9rem; border: 0; border-radius: 8px; cursor: pointer; }
  .primary { background: #2563eb; color: #fff; margin-top: .8rem; }
  .faq { border: 1px solid #8883; border-radius: 10px; padding: .75rem; margin-top: .6rem; }
  .faq .q { font-weight: 600; }
  .faq .a { opacity: .85; margin-top: .25rem; }
  .faq form { display: inline; }
  .del { background: #dc262622; color: #dc2626; margin-top: .5rem; font-size: .8rem; }
  .cat { display: inline-block; font-size: .7rem; opacity: .7; border: 1px solid #8886;
         border-radius: 999px; padding: 0 .5rem; margin-top: .4rem; }
  .empty { opacity: .6; margin-top: .6rem; }
</style>
</head>
<body>
  <h1>Support admin</h1>

  <h2>📥 Pending questions</h2>
  {{if not .Pending}}<p class="empty">No pending questions. 🎉</p>{{end}}
  {{range .Pending}}
    <div class="faq">
      <div class="q">#{{.ID}} — {{if .UserName}}{{.UserName}}{{else}}customer{{end}} <span class="cat">{{.Channel.Label}}</span> <span class="cat">{{.Ago}}</span></div>
      <div class="a">{{.Question}}</div>
      <form method="post" action="/edit/reply">
        <input type="hidden" name="id" value="{{.ID}}">
        <label>Your reply
          <textarea name="message" required></textarea>
        </label>
        <button class="primary" type="submit">Send reply</button>
      </form>
      <form method="post" action="/edit/dismiss" onsubmit="return confirm('Dismiss without replying?')">
        <input type="hidden" name="id" value="{{.ID}}">
        <button class="del" type="submit">Dismiss</button>
      </form>
    </div>
  {{end}}

  <h2>Business settings</h2>
  <form method="post" action="/edit/settings">
    <label>Shop name
      <input name="display_name" value="{{.Settings.DisplayName}}" placeholder="e.g. MiniPOS" autocomplete="off">
    </label>
    <label>Welcome message <small>({name} = shop name)</small>
      <textarea name="welcome_message" placeholder="👋 Welcome to {name} support! Pick a topic below, or just type your question.">{{.Settings.WelcomeMessage}}</textarea>
    </label>
    <label>Busy / fallback message
      <textarea name="fallback_message" placeholder="Sorry, I'm a bit busy right now — leave your message and our team will follow up.">{{.Settings.FallbackMessage}}</textarea>
    </label>
    <label>“Ask a question” prompt
      <textarea name="ask_prompt" placeholder="💬 Type your question below — I'll answer right away…">{{.Settings.AskPrompt}}</textarea>
    </label>
    <label>Max questions per user / minute <small>(0 = default 10)</small>
      <input name="ask_rate_per_min" type="number" min="0" value="{{if .Settings.AskRatePerMin}}{{.Settings.AskRatePerMin}}{{end}}" placeholder="10">
    </label>
    <label>Max questions total / minute <small>(lower this if you peak out; 0 = default 60)</small>
      <input name="ask_global_per_min" type="number" min="0" value="{{if .Settings.AskGlobalPerMin}}{{.Settings.AskGlobalPerMin}}{{end}}" placeholder="60">
    </label>
    <button class="primary" type="submit">Save settings</button>
  </form>

  <h2>Add a FAQ</h2>
  <form method="post" action="/edit/faqs">
    <label>Question
      <input name="question" required autocomplete="off">
    </label>
    <label>Answer
      <textarea name="answer" required></textarea>
    </label>
    <label>Category (optional)
      <input name="category" autocomplete="off">
    </label>
    <button class="primary" type="submit">Add FAQ</button>
  </form>

  <h2>Existing FAQs</h2>
  {{if not .FAQs}}<p class="empty">No FAQs yet.</p>{{end}}
  {{range .FAQs}}
    <div class="faq">
      <form method="post" action="/edit/faqs/update">
        <input type="hidden" name="id" value="{{.ID}}">
        <label>Question
          <input name="question" value="{{.Question}}" required autocomplete="off">
        </label>
        <label>Answer
          <textarea name="answer" required>{{.Answer}}</textarea>
        </label>
        <label>Category
          <input name="category" value="{{.Category}}" autocomplete="off">
        </label>
        <button class="primary" type="submit">Save changes</button>
      </form>
      <form method="post" action="/edit/faqs/delete" onsubmit="return confirm('Delete this FAQ?')">
        <input type="hidden" name="id" value="{{.ID}}">
        <button class="del" type="submit">Delete</button>
      </form>
    </div>
  {{end}}
</body>
</html>`
