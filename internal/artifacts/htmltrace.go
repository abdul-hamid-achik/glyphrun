package artifacts

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// RenderTraceHTML builds a self-contained frame scrubber for a run.
func RenderTraceHTML(runID, specName string, frames []terminal.Frame, finalText string) string {
	payload, _ := json.Marshal(frames)
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>`)
	b.WriteString(html.EscapeString(runID))
	b.WriteString(`</title><style>
body{font:14px/1.4 ui-monospace,Menlo,monospace;background:#111;color:#e8e8e8;margin:0}
header{padding:12px 16px;background:#1c1c1c;border-bottom:1px solid #333}
pre{white-space:pre-wrap;padding:16px;margin:0;min-height:40vh}
nav{padding:8px 16px;background:#1c1c1c;position:sticky;bottom:0;display:flex;gap:8px;align-items:center}
input[type=range]{flex:1}
</style></head><body><header>`)
	b.WriteString(html.EscapeString(specName + " · " + runID + " · " + strconv.Itoa(len(frames)) + " frames"))
	b.WriteString(`</header><pre id="screen">`)
	b.WriteString(html.EscapeString(finalText))
	b.WriteString(`</pre><nav><button id="prev">prev</button><input id="scrub" type="range" min="0" max="`)
	if len(frames) == 0 {
		b.WriteString("0")
	} else {
		b.WriteString(strconv.Itoa(len(frames) - 1))
	}
	b.WriteString(`" value="0"><button id="next">next</button><span id="meta"></span></nav>
<script>
const frames = `)
	b.Write(payload)
	b.WriteString(`;
const screen = document.getElementById('screen');
const scrub = document.getElementById('scrub');
const meta = document.getElementById('meta');
function show(i){
  if(!frames.length) return;
  i = Math.max(0, Math.min(frames.length-1, i));
  scrub.value = i;
  const f = frames[i];
  screen.textContent = (f.screen && f.screen.text) ? f.screen.text : '';
  meta.textContent = '#' + (f.seq||i) + ' ' + (f.kind||'') + ' ' + (f.time||'');
}
document.getElementById('prev').onclick = () => show(+scrub.value-1);
document.getElementById('next').onclick = () => show(+scrub.value+1);
scrub.oninput = () => show(+scrub.value);
show(frames.length ? frames.length-1 : 0);
</script></body></html>`)
	return b.String()
}
