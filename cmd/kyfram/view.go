package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	kyfram "github.com/codetesla51/kyfram"
)

// runView serves a live preview: scrub slider, play button, and hot
// reload for free — every request re-reads the script from disk, so
// saving the file updates the preview with no watcher and no restart.
func runView(args []string) {
	fs := flag.NewFlagSet("view", flag.ExitOnError)
	port := fs.Int("port", 8080, "preview server port")
	font := fs.String("font", "", "default typeface path")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "flags:", err)
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: kyfram view [--port n] [--font f] <script.lgv>")
		os.Exit(1)
	}
	script := fs.Arg(0)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, strings.Replace(viewPage, "{{SCRIPT}}", filepath.Base(script), 1))
	})
	http.HandleFunc("/font", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "font/ttf")
		w.Write(kyfram.DisplayFont())
	})
	http.HandleFunc("/meta", func(w http.ResponseWriter, r *http.Request) {
		tl, err := loadScript(script, *font)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"duration": tl.Duration, "fps": tl.FPS})
	})
	http.HandleFunc("/frame", func(w http.ResponseWriter, r *http.Request) {
		t, err := strconv.ParseFloat(r.URL.Query().Get("t"), 64)
		if err != nil {
			http.Error(w, "bad t", http.StatusBadRequest)
			return
		}
		tl, err := loadScript(script, *font)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if err := png.Encode(w, kyfram.RenderFrame(tl, t).Image()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	addr := "localhost:" + strconv.Itoa(*port)
	fmt.Println("view:", "http://"+addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintln(os.Stderr, "view:", err)
		os.Exit(1)
	}
}

// loadScript runs the file fresh: declarations in, Timeline out. Errors
// carry run/build context so the preview shows them instead of stale art.
func loadScript(path, font string) (kyfram.Timeline, error) {
	source, err := kyfram.LoadScript(path)
	if err != nil {
		return kyfram.Timeline{}, fmt.Errorf("load: %w", err)
	}
	vm := newVM()
	var b kyfram.Builder
	b.Bind(vm)
	if err := vm.Run(string(source)); err != nil {
		return kyfram.Timeline{}, fmt.Errorf("run: %w", err)
	}
	tl, err := b.Build()
	if err != nil {
		return kyfram.Timeline{}, fmt.Errorf("build: %w", err)
	}
	if font != "" {
		tl.Font = font
	}
	return tl, nil
}

const viewPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>kyfram preview — {{SCRIPT}}</title>
<style>
@font-face {
  font-family: "Archivo Black";
  src: url("/font") format("truetype");
}
* { box-sizing: border-box; }
body {
  background: #14141c;
  color: #ece8df;
  font-family: system-ui, -apple-system, sans-serif;
  margin: 0;
  padding: 32px 20px 64px;
}
main { max-width: 720px; margin: 0 auto; }
.brand {
  font-family: "Archivo Black", system-ui, sans-serif;
  font-size: 30px;
  margin: 0;
}
.script { color: #8a877f; font-size: 14px; margin: 6px 0 20px; }
.stage img {
  width: 100%;
  display: block;
  background: #000;
  border: 1px solid #2a2a35;
}
.time {
  font-family: "Archivo Black", system-ui, sans-serif;
  font-size: 54px;
  color: #e6c040;
  margin: 14px 0 2px;
}
.transport { display: flex; gap: 12px; align-items: center; margin-top: 8px; }
button {
  font: inherit;
  font-weight: 700;
  background: #e6c040;
  color: #16130a;
  border: 0;
  padding: 10px 24px;
  cursor: pointer;
}
button:focus-visible, input:focus-visible {
  outline: 2px solid #ece8df;
  outline-offset: 2px;
}
input[type="range"] { flex: 1; accent-color: #e6c040; }
.meta { color: #8a877f; font-size: 13px; margin-top: 10px; }
.error { color: #e64c40; white-space: pre-wrap; margin-top: 10px; }
.note { color: #8a877f; font-size: 13px; margin-top: 28px; }
@media (max-width: 560px) { .time { font-size: 40px; } }
</style>
</head>
<body>
<main>
  <h1 class="brand">kyfram</h1>
  <p class="script">{{SCRIPT}}</p>
  <div class="stage"><img id="f" alt="preview frame"></div>
  <div class="time" id="t">0.00</div>
  <div class="transport">
    <button id="p">Play</button>
    <input id="s" type="range" min="0" max="2" step="0.033" value="0" aria-label="scrub timeline">
  </div>
  <p class="meta" id="m"></p>
  <pre class="error" id="e"></pre>
  <p class="note">Saving the script refreshes this page. Space plays and pauses, arrow keys step one frame.</p>
</main>
<script>
let dur=2, fps=30, t=0, playing=false, timer=null;
const img=document.getElementById('f'), s=document.getElementById('s'),
      p=document.getElementById('p'), lbl=document.getElementById('t'),
      meta=document.getElementById('m'), err=document.getElementById('e');
fetch('/meta').then(r=>{if(!r.ok)throw new Error('meta');return r.json();}).then(m=>{
  dur=m.duration;fps=m.fps;s.max=dur;s.step=1/fps;
  meta.textContent=dur.toFixed(1)+'s at '+fps+'fps — '+Math.round(dur*fps)+' frames';
  show();
}).catch(async e=>{err.textContent='Could not load script.';});
async function show(){
  const r=await fetch('/frame?t='+t.toFixed(3));
  if(!r.ok){err.textContent=await r.text();return;}
  err.textContent='';
  img.src=URL.createObjectURL(await r.blob());
  lbl.textContent=t.toFixed(2); s.value=t;
}
function toggle(){
  playing=!playing; p.textContent=playing?'Pause':'Play';
  if(playing){timer=setInterval(()=>{t+=1/fps;if(t>dur)t=0;show();},1000/fps);}
  else clearInterval(timer);
}
p.onclick=toggle;
s.oninput=()=>{t=parseFloat(s.value);show();};
document.addEventListener('keydown',e=>{
  if(e.code==='Space'&&document.activeElement!==s){e.preventDefault();toggle();}
  if(e.code==='ArrowRight'){t=Math.min(dur,t+1/fps);show();}
  if(e.code==='ArrowLeft'){t=Math.max(0,t-1/fps);show();}
});
</script>
</body>
</html>`
