package editor

// pageTemplate is the FAQ editor page. Data is the []store.FAQ list. All fields
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
  <h1>FAQ editor</h1>

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
  {{if not .}}<p class="empty">No FAQs yet.</p>{{end}}
  {{range .}}
    <div class="faq">
      <div class="q">{{.Question}}</div>
      <div class="a">{{.Answer}}</div>
      {{if .Category}}<span class="cat">{{.Category}}</span>{{end}}
      <div>
        <form method="post" action="/edit/faqs/delete" onsubmit="return confirm('Delete this FAQ?')">
          <input type="hidden" name="id" value="{{.ID}}">
          <button class="del" type="submit">Delete</button>
        </form>
      </div>
    </div>
  {{end}}
</body>
</html>`
